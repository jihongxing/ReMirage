// Package rest - 内部 REST API
// 供 NestJS api-server 调用的 HTTP 接口
// 监听在 gateway-bridge 的 :7000 端口
package rest

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"

	goredis "github.com/redis/go-redis/v9"

	"mirage-os/gateway-bridge/pkg/dispatch"
	"mirage-os/gateway-bridge/pkg/quota"
)

// Handler 内部 REST 处理器
type Handler struct {
	enforcer   *quota.Enforcer
	dispatcher *dispatch.StrategyDispatcher
	db         *sql.DB
	rdb        *goredis.Client
}

// NewHandler 创建处理器
func NewHandler(enforcer *quota.Enforcer, dispatcher *dispatch.StrategyDispatcher, db *sql.DB, rdb *goredis.Client) *Handler {
	return &Handler{enforcer: enforcer, dispatcher: dispatcher, db: db, rdb: rdb}
}

// RegisterRoutes 注册路由到 mux
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/internal/gateways", h.handleGateways)
	mux.HandleFunc("/internal/quota/", h.handleQuota)
	mux.HandleFunc("/internal/strategy/push", h.handleStrategyPush)
	mux.HandleFunc("/internal/health", h.handleHealth)
}

// handleGateways 查询所有在线 Gateway
func (h *Handler) handleGateways(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	rows, err := h.db.Query(`
		SELECT id, cell_id, ip_address, status, last_heartbeat, ebpf_loaded,
		       threat_level, active_connections, memory_usage_mb
		FROM gateways
		ORDER BY last_heartbeat DESC
	`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type GatewayInfo struct {
		ID                string  `json:"id"`
		CellID            *string `json:"cell_id"`
		IPAddress         *string `json:"ip_address"`
		Status            string  `json:"status"`
		LastHeartbeat     *string `json:"last_heartbeat"`
		EBPFLoaded        bool    `json:"ebpf_loaded"`
		ThreatLevel       int     `json:"threat_level"`
		ActiveConnections int64   `json:"active_connections"`
		MemoryUsageMB     int     `json:"memory_usage_mb"`
	}

	var gateways []GatewayInfo
	for rows.Next() {
		var gw GatewayInfo
		if err := rows.Scan(&gw.ID, &gw.CellID, &gw.IPAddress, &gw.Status,
			&gw.LastHeartbeat, &gw.EBPFLoaded, &gw.ThreatLevel,
			&gw.ActiveConnections, &gw.MemoryUsageMB); err != nil {
			continue
		}
		gateways = append(gateways, gw)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(gateways)
}

// handleQuota 查询用户配额
func (h *Handler) handleQuota(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// /internal/quota/{user_id}
	userID := r.URL.Path[len("/internal/quota/"):]
	if userID == "" {
		http.Error(w, "missing user_id", http.StatusBadRequest)
		return
	}

	remaining, err := h.enforcer.GetRemainingQuotaByUser(userID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"user_id":         userID,
		"remaining_quota": remaining,
	})
}

// handleStrategyPush 下发策略到指定 Cell
func (h *Handler) handleStrategyPush(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		CellID         string `json:"cell_id"`
		DefenseLevel   int32  `json:"defense_level"`
		JitterMeanUs   uint32 `json:"jitter_mean_us"`
		JitterStddevUs uint32 `json:"jitter_stddev_us"`
		NoiseIntensity uint32 `json:"noise_intensity"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	// 暂时返回 OK（实际需要调用 dispatcher.PushStrategyToCell）
	log.Printf("[REST] 策略下发请求: cell=%s, level=%d", req.CellID, req.DefenseLevel)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// handleHealth 健康检查
func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}
