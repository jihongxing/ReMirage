// Package provisioning - 内部 HTTP API
// 供 NestJS API Server 调用的阅后即焚兑换接口
package provisioning

import (
	"encoding/json"
	"log"
	"net/http"
)

// HTTPHandler 内部 HTTP 处理器
type HTTPHandler struct {
	provisioner *Provisioner
}

// NewHTTPHandler 创建 HTTP 处理器
func NewHTTPHandler(p *Provisioner) *HTTPHandler {
	return &HTTPHandler{provisioner: p}
}

// RegisterRoutes 注册路由
func (h *HTTPHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/internal/delivery/redeem", h.handleRedeem)
	mux.HandleFunc("/internal/delivery/status/", h.handleStatus)
	mux.HandleFunc("/internal/provision", h.handleProvision)
}

// handleRedeem 兑换阅后即焚链接
func (h *HTTPHandler) handleRedeem(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Token      string `json:"token"`
		DecryptKey string `json:"decrypt_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	plaintext, err := h.provisioner.RedeemBurnLink(req.Token, req.DecryptKey)
	if err != nil {
		errMsg := err.Error()
		switch errMsg {
		case "链接不存在或已销毁":
			w.WriteHeader(http.StatusNotFound)
		case "链接已被使用":
			w.WriteHeader(http.StatusGone)
		case "链接已过期":
			w.WriteHeader(http.StatusGone)
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
		json.NewEncoder(w).Encode(map[string]string{"error": errMsg})
		return
	}

	// 直接返回解密后的 JSON
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Write(plaintext)
}

// handleStatus 查询链接状态（不暴露内容）
func (h *HTTPHandler) handleStatus(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Path[len("/internal/delivery/status/"):]
	if token == "" {
		http.Error(w, "missing token", http.StatusBadRequest)
		return
	}

	exists, consumed, expired := h.provisioner.GetLinkStatus(token)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"exists":   exists,
		"consumed": consumed,
		"expired":  expired,
	})
}

// handleProvision 手动触发配置（管理接口）
func (h *HTTPHandler) handleProvision(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		UID            string `json:"uid"`
		AmountPiconero uint64 `json:"amount_piconero"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	if err := h.provisioner.OnXMRConfirmed(req.UID, req.AmountPiconero); err != nil {
		log.Printf("[Provisioner] ❌ 手动配置失败: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
