// Package entitlement - V2 Entitlement API handler
package entitlement

import (
	"encoding/json"
	"mirage-os/pkg/models"
	"net/http"
	"time"

	"gorm.io/gorm"
)

// EntitlementResponse entitlement API 响应结构
type EntitlementResponse struct {
	ServiceClass   string    `json:"service_class"`
	ExpiresAt      string    `json:"expires_at"`
	QuotaRemaining int64     `json:"quota_remaining"`
	Banned         bool      `json:"banned"`
	FetchedAt      time.Time `json:"fetched_at"`
}

// Handler Entitlement API 处理器
type Handler struct {
	db *gorm.DB
}

// NewHandler 创建 Entitlement Handler
func NewHandler(db *gorm.DB) *Handler {
	return &Handler{db: db}
}

// RegisterRoutes 注册路由
// 暴露边界：entitlement 为用户级接口，需要 Ed25519 签名认证
// 由 QueryAuthMiddleware 强制认证，不再信任裸 X-Client-ID
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v2/entitlement", h.handleEntitlement)
}

func (h *Handler) handleEntitlement(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// 认证后的 Client ID（由 QueryAuthMiddleware 验证签名后放行）
	// 从已验证的 X-Client-ID 或 X-Authenticated-User 获取
	userID := r.Header.Get("X-Authenticated-User")
	if userID == "" {
		userID = r.Header.Get("X-Client-ID")
	}
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	var user models.User
	if err := h.db.Where("user_id = ?", userID).First(&user).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// 确定 service class
	serviceClass := "standard"
	switch user.CellLevel {
	case 2:
		serviceClass = "platinum"
	case 3:
		serviceClass = "diamond"
	}

	expiresAt := ""
	if user.SubscriptionExpiresAt != nil {
		expiresAt = user.SubscriptionExpiresAt.Format(time.RFC3339)
	} else if user.QuotaExpiresAt != nil {
		expiresAt = user.QuotaExpiresAt.Format(time.RFC3339)
	}

	resp := EntitlementResponse{
		ServiceClass:   serviceClass,
		ExpiresAt:      expiresAt,
		QuotaRemaining: user.RemainingQuota,
		Banned:         user.Status == "banned",
		FetchedAt:      time.Now().UTC(),
	}

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
