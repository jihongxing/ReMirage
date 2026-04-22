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
	Status         string // ONLINE / DEGRADED / UNDER_ATTACK / DRAINING / DEAD / OFFLINE
	Version        string
	EBPFSupported  bool
	MaxConnections int32
	MaxSessions    int32
	ActiveSessions int32
	LastHeartbeat  time.Time
	ThreatLevel    int32     // Gateway 上报的威胁等级
	CPUUsage       float64   // CPU 使用率
	MemoryUsageMB  int32     // 内存使用量
	AttackStartAt  time.Time // 进入 UNDER_ATTACK 的时间
}

// Registry 拓扑索引管理器：内存索引 + DB 持久化 + Redis 缓存三写
type Registry struct {
	gateways      map[string]*GatewayInfo // gateway_id → info
	byCell        map[string][]string     // cell_id → []gateway_id
	mu            sync.RWMutex
	db            *sql.DB
	rdb           *goredis.Client
	ddosResponder *DDoSResponder // DDoS 响应协调器（可选注入）
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

// SetDDoSResponder 注入 DDoS 响应协调器
func (r *Registry) SetDDoSResponder(responder *DDoSResponder) {
	r.ddosResponder = responder
}

// Register 注册 Gateway（DB UPSERT + Redis 拓扑索引写入 + 内存索引更新）
func (r *Registry) Register(ctx context.Context, info *GatewayInfo) error {
	info.Status = "ONLINE"
	info.LastHeartbeat = time.Now()

	// 1. DB UPSERT（列名对齐 GORM Gateway model — ADR-001）
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO gateways (gateway_id, cell_id, ip_address, status, ebpf_loaded, last_heartbeat_at, updated_at, downlink_addr, version, max_sessions, active_sessions)
		VALUES ($1, $2, $3, 'ONLINE', $4, NOW(), NOW(), $5, $6, $7, $8)
		ON CONFLICT (gateway_id) DO UPDATE SET
			cell_id = EXCLUDED.cell_id,
			ip_address = EXCLUDED.ip_address,
			status = 'ONLINE',
			ebpf_loaded = EXCLUDED.ebpf_loaded,
			last_heartbeat_at = NOW(),
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

// GetAllOnlineCount 返回在线节点数量（实现 RegistryReader 接口）
func (r *Registry) GetAllOnlineCount() int {
	return len(r.GetAllOnline())
}

// GetByStatusCount 返回指定状态的节点数量（实现 RegistryReader 接口）
func (r *Registry) GetByStatusCount(status string) int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	count := 0
	for _, gw := range r.gateways {
		if gw.Status == status {
			count++
		}
	}
	return count
}

// UpdateHeartbeat 刷新 LastHeartbeat 和 ActiveSessions，并评估 DDoS 状态
func (r *Registry) UpdateHeartbeat(gatewayID string, activeSessions int32, stateHash string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	gw, ok := r.gateways[gatewayID]
	if !ok {
		return
	}

	gw.LastHeartbeat = time.Now()
	gw.ActiveSessions = activeSessions

	// 如果节点从 UNDER_ATTACK 恢复（威胁等级降低），自动恢复
	if gw.Status == "UNDER_ATTACK" && gw.ThreatLevel <= 1 {
		gw.Status = "ONLINE"
		gw.AttackStartAt = time.Time{}
		log.Printf("[Registry] Gateway %s 威胁等级降低，自动恢复为 ONLINE", gatewayID)
	} else if gw.Status == "DEAD" || gw.Status == "DRAINING" {
		// DEAD/DRAINING 节点收到心跳说明恢复了
		// DEAD 节点重新上线但 Phase 重置为 1（校准期），不立即分配用户
		prevStatus := gw.Status
		gw.Status = "ONLINE"
		gw.AttackStartAt = time.Time{}
		log.Printf("[Registry] Gateway %s 重新上线（从 %s 恢复），进入校准期", gatewayID, prevStatus)
	} else if gw.Status != "UNDER_ATTACK" {
		gw.Status = "ONLINE"
	}
}

// UpdateHeartbeatWithMetrics 带指标的心跳更新（7.5 - 接通真实数据）
func (r *Registry) UpdateHeartbeatWithMetrics(gatewayID string, activeSessions int32,
	threatLevel int32, cpuUsage float64, memoryMB int32) {
	r.mu.Lock()
	gw, ok := r.gateways[gatewayID]
	if !ok {
		r.mu.Unlock()
		return
	}

	gw.LastHeartbeat = time.Now()
	gw.ActiveSessions = activeSessions
	gw.ThreatLevel = threatLevel
	gw.CPUUsage = cpuUsage
	gw.MemoryUsageMB = memoryMB

	prevStatus := gw.Status

	// DDoS 状态裁决（7.5 - 基于真实指标）
	ddosState := r.evaluateDDoSState(gw)
	r.mu.Unlock()

	ctx := context.Background()

	if r.ddosResponder != nil {
		// 使用 DDoSResponder 处理状态变迁
		switch ddosState {
		case "UNDER_ATTACK":
			if prevStatus == "ONLINE" {
				r.ddosResponder.HandleResourcePressure(ctx, gatewayID)
			} else if prevStatus == "UNDER_ATTACK" {
				// 仍在攻击中，重置恢复计数器
				r.ddosResponder.ResetRecoveryCounter(gatewayID)
			}
		case "ONLINE":
			if prevStatus == "UNDER_ATTACK" {
				// 递增恢复计数器，连续 3 次正常后恢复
				if r.ddosResponder.IncrementRecoveryCounter(gatewayID) {
					r.ddosResponder.HandleRecovery(ctx, gatewayID)
				}
			}
		}
	} else {
		// 降级路径：直接标记状态
		switch ddosState {
		case "UNDER_ATTACK":
			if prevStatus == "ONLINE" {
				r.MarkUnderAttack(ctx, gatewayID)
			}
		case "ONLINE":
			if prevStatus == "UNDER_ATTACK" {
				r.RecoverFromAttack(ctx, gatewayID)
			}
		}
	}
}

// evaluateDDoSState 基于真实指标判定 DDoS 状态（7.5 - 替代占位逻辑）
// 必须在持有 r.mu 锁的情况下调用
func (r *Registry) evaluateDDoSState(gw *GatewayInfo) string {
	// 资源耗尽型攻击判定：
	// - 威胁等级 ≥ 3（Critical）
	// - 或 CPU > 90%
	// - 或会话数接近上限（> 90%）
	if gw.ThreatLevel >= 3 {
		return "UNDER_ATTACK"
	}
	if gw.CPUUsage > 90 {
		return "UNDER_ATTACK"
	}
	if gw.MaxSessions > 0 && float64(gw.ActiveSessions)/float64(gw.MaxSessions) > 0.9 {
		return "UNDER_ATTACK"
	}

	// 恢复判定：威胁等级 ≤ 1 且 CPU < 70%
	if gw.ThreatLevel <= 1 && gw.CPUUsage < 70 {
		return "ONLINE"
	}

	return gw.Status // 维持当前状态
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
		_, err := r.db.ExecContext(ctx, `UPDATE gateways SET status = 'OFFLINE', updated_at = NOW() WHERE gateway_id = $1`, gatewayID)
		if err != nil {
			log.Printf("[Registry] DB mark offline error for %s: %v", gatewayID, err)
		}
	}

	// Redis 标记下线
	r.rdb.HSet(ctx, fmt.Sprintf("topo:gw:%s", gatewayID), "status", "OFFLINE")
	r.rdb.SRem(ctx, fmt.Sprintf("topo:cell:%s:gateways", gw.CellID), gatewayID)

	log.Printf("[Registry] Gateway %s 标记下线", gatewayID)
}

// MarkUnderAttack 标记 Gateway 受攻击（7.6 - 停止分配新用户，老会话保留）
func (r *Registry) MarkUnderAttack(ctx context.Context, gatewayID string) {
	r.mu.Lock()
	gw, ok := r.gateways[gatewayID]
	if ok && gw.Status == "ONLINE" {
		gw.Status = "UNDER_ATTACK"
		gw.AttackStartAt = time.Now()
	}
	r.mu.Unlock()

	if !ok {
		return
	}

	r.updateStatus(ctx, gatewayID, "UNDER_ATTACK")
	log.Printf("[Registry] ⚠️ Gateway %s 标记为 UNDER_ATTACK（停止接纳新用户）", gatewayID)
}

// MarkDraining 标记 Gateway 排空中（软防 → 准备下线）
func (r *Registry) MarkDraining(ctx context.Context, gatewayID string) {
	r.mu.Lock()
	gw, ok := r.gateways[gatewayID]
	if ok {
		gw.Status = "DRAINING"
	}
	r.mu.Unlock()

	if !ok {
		return
	}

	r.updateStatus(ctx, gatewayID, "DRAINING")
	log.Printf("[Registry] Gateway %s 标记为 DRAINING（排空中）", gatewayID)
}

// MarkDead 标记 Gateway 死亡（7.6 - 体积型攻击，不再假设可排空）
func (r *Registry) MarkDead(ctx context.Context, gatewayID string) {
	r.mu.Lock()
	gw, ok := r.gateways[gatewayID]
	if ok {
		gw.Status = "DEAD"
	}
	r.mu.Unlock()

	if !ok {
		return
	}

	// 从分配池完全移除
	r.updateStatus(ctx, gatewayID, "DEAD")
	r.rdb.SRem(ctx, fmt.Sprintf("topo:cell:%s:gateways", gw.CellID), gatewayID)

	log.Printf("[Registry] 🔴 Gateway %s 标记为 DEAD（已从分配池移除）", gatewayID)
}

// RecoverFromAttack 攻击结束后恢复节点（软防场景）
func (r *Registry) RecoverFromAttack(ctx context.Context, gatewayID string) {
	r.mu.Lock()
	gw, ok := r.gateways[gatewayID]
	if ok && (gw.Status == "UNDER_ATTACK" || gw.Status == "DRAINING") {
		gw.Status = "ONLINE"
		gw.AttackStartAt = time.Time{}
	}
	r.mu.Unlock()

	if !ok {
		return
	}

	r.updateStatus(ctx, gatewayID, "ONLINE")
	r.rdb.SAdd(ctx, fmt.Sprintf("topo:cell:%s:gateways", gw.CellID), gatewayID)
	log.Printf("[Registry] ✅ Gateway %s 恢复为 ONLINE", gatewayID)
}

// IsAssignable 判断 Gateway 是否可分配新用户
func (r *Registry) IsAssignable(gatewayID string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	gw, ok := r.gateways[gatewayID]
	if !ok {
		return false
	}
	// 只有 ONLINE 状态可以接纳新用户
	return gw.Status == "ONLINE"
}

// updateStatus 统一更新 DB + Redis 状态
func (r *Registry) updateStatus(ctx context.Context, gatewayID, status string) {
	if r.db != nil {
		_, err := r.db.ExecContext(ctx,
			`UPDATE gateways SET status = $1, updated_at = NOW() WHERE gateway_id = $2`,
			status, gatewayID)
		if err != nil {
			log.Printf("[Registry] DB update status error for %s: %v", gatewayID, err)
		}
	}
	r.rdb.HSet(ctx, fmt.Sprintf("topo:gw:%s", gatewayID), "status", status)
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
	var dead []string
	deadThreshold := timeout * 3 // 3 倍超时判定为 DEAD（体积型攻击 / 物理失联）
	for gwID, gw := range r.gateways {
		if gw.Status == "OFFLINE" || gw.Status == "DEAD" {
			continue
		}
		elapsed := time.Since(gw.LastHeartbeat)
		if elapsed > deadThreshold {
			dead = append(dead, gwID)
		} else if elapsed > timeout {
			expired = append(expired, gwID)
		}
	}
	r.mu.RUnlock()

	for _, gwID := range dead {
		log.Printf("[Registry] 🔴 Gateway %s 心跳超时 >3x，判定为 DEAD", gwID)
		if r.ddosResponder != nil {
			r.ddosResponder.HandleNodeDeath(ctx, gwID)
		} else {
			r.MarkDead(ctx, gwID)
		}
	}
	for _, gwID := range expired {
		log.Printf("[Registry] Gateway %s 心跳超时，标记下线", gwID)
		r.MarkOffline(ctx, gwID)
	}
}

// loadFromDB 启动时从 DB 加载已有 Gateway 信息到内存索引
func (r *Registry) loadFromDB() {
	rows, err := r.db.Query(`
		SELECT gateway_id, COALESCE(cell_id, ''), COALESCE(downlink_addr, COALESCE(ip_address, '')),
		       COALESCE(status, 'OFFLINE'), COALESCE(version, ''),
		       COALESCE(ebpf_loaded, false), COALESCE(max_sessions, 0),
		       COALESCE(active_sessions, 0), COALESCE(last_heartbeat_at, NOW())
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
