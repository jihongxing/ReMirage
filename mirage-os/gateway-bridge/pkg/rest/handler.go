// Package rest - 内部 REST API
// 供 NestJS api-server 调用的 HTTP 接口
// 监听在 gateway-bridge 的 :7000 端口
package rest

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

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
	mux.HandleFunc("/internal/webhook/xmr", h.handleXMRWebhook)
	mux.HandleFunc("/internal/billing/", h.handleBilling)
	mux.HandleFunc("/internal/resonance/publish", h.handleResonancePublish)
	mux.HandleFunc("/internal/gateway/", h.handleGatewayAction)
}

// handleGateways 查询所有在线 Gateway
func (h *Handler) handleGateways(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	rows, err := h.db.Query(`
		SELECT id, cell_id, ip_address, status, last_heartbeat_at, ebpf_loaded,
		       threat_level, active_connections, memory_usage_mb
		FROM gateways
		ORDER BY last_heartbeat_at DESC NULLS LAST
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

// handleXMRWebhook 处理 Monero 充值到账通知
func (h *Handler) handleXMRWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		UserID        string  `json:"user_id"`
		TxHash        string  `json:"tx_hash"`
		AmountXMR     float64 `json:"amount_xmr"`
		Confirmations int     `json:"confirmations"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	if req.UserID == "" || req.AmountXMR <= 0 {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}

	// 按 $150/XMR 汇率转换为配额（GB）
	xmrPrice := 150.0
	pricePerGB := 0.10
	quotaGB := (req.AmountXMR * xmrPrice) / pricePerGB
	quotaBytes := quotaGB * 1024 * 1024 * 1024

	// 更新用户配额
	_, err := h.db.Exec(`
		UPDATE users SET
			remaining_quota = remaining_quota + $1,
			total_deposit = total_deposit + $2,
			updated_at = NOW()
		WHERE id = $3
	`, quotaBytes, req.AmountXMR, req.UserID)
	if err != nil {
		log.Printf("[REST] XMR webhook DB error: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	log.Printf("[REST] XMR 到账: user=%s, amount=%.4f XMR, quota=+%.2f GB", req.UserID, req.AmountXMR, quotaGB)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":         "accepted",
		"user_id":        req.UserID,
		"quota_added_gb": quotaGB,
	})
}

// handleBilling 查询用户计费信息
func (h *Handler) handleBilling(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// /internal/billing/{user_id}
	userID := r.URL.Path[len("/internal/billing/"):]
	if userID == "" {
		http.Error(w, "missing user_id", http.StatusBadRequest)
		return
	}

	var totalBusiness, totalDefense int64
	var totalCost float64
	err := h.db.QueryRow(`
		SELECT COALESCE(SUM(business_bytes), 0),
		       COALESCE(SUM(defense_bytes), 0),
		       COALESCE(SUM(total_cost), 0)
		FROM billing_logs WHERE user_id = $1
	`, userID).Scan(&totalBusiness, &totalDefense, &totalCost)
	if err != nil && err != sql.ErrNoRows {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"user_id":        userID,
		"consumed_bytes": totalBusiness + totalDefense,
		"business_bytes": totalBusiness,
		"defense_bytes":  totalDefense,
		"total_cost":     totalCost,
	})
}

// handleResonancePublish 发布信令共振信息
func (h *Handler) handleResonancePublish(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Gateways []struct {
			IP       string `json:"ip"`
			Port     int    `json:"port"`
			Priority int    `json:"priority"`
		} `json:"gateways"`
		Domains []string `json:"domains"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	// 写入 Redis 供 mock 信令服务读取
	ctx := context.Background()
	payload, _ := json.Marshal(req)
	h.rdb.Set(ctx, "resonance:signal_payload", string(payload), 5*time.Minute)

	log.Printf("[REST] 信令共振发布: %d gateways, %d domains", len(req.Gateways), len(req.Domains))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":   "published",
		"gateways": len(req.Gateways),
		"domains":  len(req.Domains),
	})
}

// handleGatewayAction 处理 Gateway 操作（kill 等）
func (h *Handler) handleGatewayAction(w http.ResponseWriter, r *http.Request) {
	// /internal/gateway/{id}/kill
	path := r.URL.Path[len("/internal/gateway/"):]
	parts := strings.SplitN(path, "/", 2)
	if len(parts) < 2 {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}

	gatewayID := parts[0]
	action := parts[1]

	switch action {
	case "kill":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		// 标记 Gateway 为 killed
		_, err := h.db.Exec(`UPDATE gateways SET status = 'killed', updated_at = NOW() WHERE gateway_id = $1`, gatewayID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// 通过 Redis 发布 kill 指令
		ctx := context.Background()
		h.rdb.Publish(ctx, fmt.Sprintf("gateway:%s:cmd", gatewayID), "SCORCHED_EARTH")

		log.Printf("[REST] 焦土指令已下发: gateway=%s", gatewayID)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "killed", "gateway_id": gatewayID})

	default:
		http.Error(w, "unknown action: "+action, http.StatusBadRequest)
	}
}
