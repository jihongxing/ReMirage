// Package topology - V2 Topology API handler
package topology

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"mirage-os/pkg/models"
	"net/http"
	"os"
	"sync/atomic"
	"time"

	"gorm.io/gorm"
)

// GatewayNode topology 响应中的 gateway 节点（与 Client 端 GatewayNode 结构对齐）
type GatewayNode struct {
	IP       string `json:"ip"`
	Port     int    `json:"port"`
	Priority uint8  `json:"priority"`
	Region   string `json:"region"`
	CellID   string `json:"cell_id"`
}

// RouteTableResponse topology API 响应结构（与 Client 端 RouteTableResponse 类型对齐）
type RouteTableResponse struct {
	Version     uint64        `json:"version"`
	PublishedAt time.Time     `json:"published_at"`
	Gateways    []GatewayNode `json:"gateways"`
	Signature   []byte        `json:"signature"`
}

// hmacBody is the canonical form used for HMAC computation (aligned with Client-side hmacBody).
type hmacBody struct {
	Gateways    []GatewayNode `json:"gateways"`
	Version     uint64        `json:"version"`
	PublishedAt time.Time     `json:"published_at"`
}

// Handler Topology API 处理器
type Handler struct {
	db      *gorm.DB
	version atomic.Uint64
	psk     []byte
}

// NewHandler 创建 Topology Handler
func NewHandler(db *gorm.DB) *Handler {
	h := &Handler{db: db}
	// PSK 从环境变量加载
	if pskHex := os.Getenv("MIRAGE_PSK"); pskHex != "" {
		h.psk, _ = hex.DecodeString(pskHex)
	}
	return h
}

// RegisterRoutes 注册路由
// 暴露边界：topology 为内部接口，仅允许经过 HMAC 认证的 Gateway 调用
// 不应暴露到公网，由 QueryAuthMiddleware 强制认证
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v2/topology", h.handleTopology)
}

func (h *Handler) handleTopology(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// 查询在线 gateways
	var gateways []models.Gateway
	if err := h.db.Where("status = ? AND is_online = ?", "ONLINE", true).Find(&gateways).Error; err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	nodes := make([]GatewayNode, 0, len(gateways))
	for _, gw := range gateways {
		nodes = append(nodes, GatewayNode{
			IP:       gw.IPAddress,
			Port:     443,
			Priority: 0,
			Region:   gw.CellID,
			CellID:   gw.CellID,
		})
	}

	version := h.version.Add(1)
	publishedAt := time.Now().UTC()

	resp := RouteTableResponse{
		Version:     version,
		PublishedAt: publishedAt,
		Gateways:    nodes,
	}

	// HMAC 签名（与 Client 端 ComputeHMAC 对齐：json.Marshal(hmacBody{...})）
	if len(h.psk) > 0 {
		body := hmacBody{
			Gateways:    nodes,
			Version:     version,
			PublishedAt: publishedAt,
		}
		data, _ := json.Marshal(body)
		mac := hmac.New(sha256.New, h.psk)
		mac.Write(data)
		resp.Signature = mac.Sum(nil)
	}

	// ETag 支持
	etag := fmt.Sprintf(`"%d"`, version)
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("ETag", etag)

	writeJSON(w, http.StatusOK, resp)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
