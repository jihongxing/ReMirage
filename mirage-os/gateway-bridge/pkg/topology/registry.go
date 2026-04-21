package topology

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"sync"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// GatewayInfo 表示一个 Gateway 的完整拓扑信息
type GatewayInfo struct {
	GatewayID      string
	CellID         string
	DownlinkAddr   string
	Status         string // ONLINE / DEGRADED / OFFLINE
	Version        string
	EBPFSupported  bool
	MaxConnections int32
	MaxSessions    int32
	ActiveSessions int32
	LastHeartbeat  time.Time
}

// Registry 拓扑索引管理器：内存索引 + DB 持久化 + Redis 缓存三写
type Registry struct {
	gateways map[string]*GatewayInfo // gateway_id → info
	byCell   map[string][]string     // cell_id → []gateway_id
	mu       sync.RWMutex
	db       *sql.DB
	rdb      *goredis.Client
}

// NewRegistry 创建拓扑索引管理器，启动时从 DB 加载已有数据
func NewRegistry(db *sql.DB, rdb *goredis.Client) *Registry {
	r := &Registry{
		gateways: make(map[string]*GatewayInfo),
		byCell:   make(map[string][]string),
		db:       db,
		rdb:      rdb,
	}
	r.loadFromDB()
	return r
}

// Register 注册 Gateway（DB UPSERT + Redis 拓扑索引写入 + 内存索引更新）
func (r *Registry) Register(ctx context.Context, info *GatewayInfo) error {
	info.Status = "ONLINE"
	info.LastHeartbeat = time.Now()

	// 1. DB UPSERT
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO gateways (id, cell_id, ip_address, status, ebpf_loaded, last_heartbeat, updated_at, downlink_addr, version, max_sessions, active_sessions)
		VALUES ($1, $2, $3, 'ONLINE', $4, NOW(), NOW(), $5, $6, $7, $8)
		ON CONFLICT (id) DO UPDATE SET
			cell_id = EXCLUDED.cell_id,
			ip_address = EXCLUDED.ip_address,
			status = 'ONLINE',
			ebpf_loaded = EXCLUDED.ebpf_loaded,
			last_heartbeat = NOW(),
			updated_at = NOW(),
			downlink_addr = EXCLUDED.downlink_addr,
			version = EXCLUDED.version,
			max_sessions = EXCLUDED.max_sessions,
			active_sessions = EXCLUDED.active_sessions
	`, info.GatewayID, info.CellID, info.DownlinkAddr, info.EBPFSupported,
		info.DownlinkAddr, info.Version, info.MaxSessions, info.ActiveSessions)
	if err != nil {
		return fmt.Errorf("db upsert: %w", err)
	}

	// 2. Redis 拓扑索引（pipeline 批量写入）
	pipe := r.rdb.Pipeline()
	gwKey := fmt.Sprintf("topo:gw:%s", info.GatewayID)
	pipe.HSet(ctx, gwKey, map[string]interface{}{
		"cell_id":       info.CellID,
		"downlink_addr": info.DownlinkAddr,
		"status":        "ONLINE",
		"version":       info.Version,
		"max_sessions":  info.MaxSessions,
	})
	pipe.Expire(ctx, gwKey, 10*time.Minute)
	pipe.SAdd(ctx, fmt.Sprintf("topo:cell:%s:gateways", info.CellID), info.GatewayID)
	pipe.Set(ctx, fmt.Sprintf("gateway:%s:addr", info.GatewayID), info.DownlinkAddr, 10*time.Minute)
	pipe.Set(ctx, fmt.Sprintf("gateway:%s:status", info.GatewayID), "ONLINE", 60*time.Second)
	if _, err := pipe.Exec(ctx); err != nil {
		log.Printf("[Registry] Redis pipeline error: %v", err)
	}

	// 3. 内存索引更新
	r.mu.Lock()
	defer r.mu.Unlock()
	// Cell 变更时从旧 Cell 移除
	if old, ok := r.gateways[info.GatewayID]; ok && old.CellID != info.CellID {
		r.removeCellIndex(old.CellID, info.GatewayID)
		// 同时从 Redis 旧 Cell 集合中移除
		r.rdb.SRem(ctx, fmt.Sprintf("topo:cell:%s:gateways", old.CellID), info.GatewayID)
	}
	r.gateways[info.GatewayID] = info
	r.byCell[info.CellID] = appendUnique(r.byCell[info.CellID], info.GatewayID)

	log.Printf("[Registry] Gateway %s 注册成功 (cell=%s, addr=%s)", info.GatewayID, info.CellID, info.DownlinkAddr)
	return nil
}

// GetGatewaysByCell 查询指定 Cell 下所有在线 Gateway
func (r *Registry) GetGatewaysByCell(cellID string) []*GatewayInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []*GatewayInfo
	for _, gwID := range r.byCell[cellID] {
		if gw, ok := r.gateways[gwID]; ok && gw.Status == "ONLINE" {
			result = append(result, gw)
		}
	}
	return result
}

// GetAllOnline 查询所有在线 Gateway
func (r *Registry) GetAllOnline() []*GatewayInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []*GatewayInfo
	for _, gw := range r.gateways {
		if gw.Status == "ONLINE" {
			result = append(result, gw)
		}
	}
	return result
}

// UpdateHeartbeat 刷新 LastHeartbeat 和 ActiveSessions
func (r *Registry) UpdateHeartbeat(gatewayID string, activeSessions int32, stateHash string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if gw, ok := r.gateways[gatewayID]; ok {
		gw.LastHeartbeat = time.Now()
		gw.ActiveSessions = activeSessions
		gw.Status = "ONLINE"
	}
}

// MarkOffline 标记 Gateway 下线（DB + Redis + 内存三处）
func (r *Registry) MarkOffline(ctx context.Context, gatewayID string) {
	r.mu.Lock()
	gw, ok := r.gateways[gatewayID]
	if ok {
		gw.Status = "OFFLINE"
	}
	r.mu.Unlock()

	if !ok {
		return
	}

	// DB 标记下线
	if r.db != nil {
		_, err := r.db.ExecContext(ctx, `UPDATE gateways SET status = 'OFFLINE', updated_at = NOW() WHERE id = $1`, gatewayID)
		if err != nil {
			log.Printf("[Registry] DB mark offline error for %s: %v", gatewayID, err)
		}
	}

	// Redis 标记下线
	r.rdb.HSet(ctx, fmt.Sprintf("topo:gw:%s", gatewayID), "status", "OFFLINE")
	r.rdb.SRem(ctx, fmt.Sprintf("topo:cell:%s:gateways", gw.CellID), gatewayID)

	log.Printf("[Registry] Gateway %s 标记下线", gatewayID)
}

// StartTimeoutChecker 启动心跳超时检查协程（每 60 秒检查一次）
func (r *Registry) StartTimeoutChecker(ctx context.Context, timeout time.Duration) {
	go func() {
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				r.checkTimeouts(ctx, timeout)
			}
		}
	}()
}

func (r *Registry) checkTimeouts(ctx context.Context, timeout time.Duration) {
	r.mu.RLock()
	var expired []string
	for gwID, gw := range r.gateways {
		if gw.Status == "ONLINE" && time.Since(gw.LastHeartbeat) > timeout {
			expired = append(expired, gwID)
		}
	}
	r.mu.RUnlock()

	for _, gwID := range expired {
		log.Printf("[Registry] Gateway %s 心跳超时，自动标记下线", gwID)
		r.MarkOffline(ctx, gwID)
	}
}

// loadFromDB 启动时从 DB 加载已有 Gateway 信息到内存索引
func (r *Registry) loadFromDB() {
	rows, err := r.db.Query(`
		SELECT id, COALESCE(cell_id, ''), COALESCE(downlink_addr, COALESCE(ip_address, '')),
		       COALESCE(status, 'OFFLINE'), COALESCE(version, ''),
		       COALESCE(ebpf_loaded, false), COALESCE(max_sessions, 0),
		       COALESCE(active_sessions, 0), COALESCE(last_heartbeat, NOW())
		FROM gateways
	`)
	if err != nil {
		log.Printf("[Registry] loadFromDB query error: %v", err)
		return
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var gw GatewayInfo
		if err := rows.Scan(
			&gw.GatewayID, &gw.CellID, &gw.DownlinkAddr,
			&gw.Status, &gw.Version,
			&gw.EBPFSupported, &gw.MaxSessions,
			&gw.ActiveSessions, &gw.LastHeartbeat,
		); err != nil {
			log.Printf("[Registry] loadFromDB scan error: %v", err)
			continue
		}
		r.gateways[gw.GatewayID] = &gw
		if gw.CellID != "" {
			r.byCell[gw.CellID] = appendUnique(r.byCell[gw.CellID], gw.GatewayID)
		}
		count++
	}
	log.Printf("[Registry] 从 DB 加载 %d 个 Gateway", count)
}

// removeCellIndex 从 byCell 索引中移除指定 gateway
func (r *Registry) removeCellIndex(cellID, gatewayID string) {
	ids := r.byCell[cellID]
	for i, id := range ids {
		if id == gatewayID {
			r.byCell[cellID] = append(ids[:i], ids[i+1:]...)
			return
		}
	}
}

// appendUnique 向 slice 中追加不重复的元素
func appendUnique(slice []string, item string) []string {
	for _, s := range slice {
		if s == item {
			return slice
		}
	}
	return append(slice, item)
}
