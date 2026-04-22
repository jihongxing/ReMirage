// Package transactionquery - V2 Transaction Query API
package transactionquery

import (
	"encoding/json"
	"mirage-os/pkg/models"
	"net/http"
	"strings"
	"time"

	"gorm.io/gorm"
)

// Handler Transaction Query API 处理器
type Handler struct {
	db *gorm.DB
}

// NewHandler 创建 Handler 实例
func NewHandler(db *gorm.DB) *Handler {
	return &Handler{db: db}
}

// RegisterRoutes 注册路由
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v2/transactions/active", h.handleActive)
	mux.HandleFunc("/api/v2/transactions/", h.handleTxDetail)
	mux.HandleFunc("/api/v2/transactions", h.handleList)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// GET /api/v2/transactions/{tx_id}
func (h *Handler) handleTxDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	txID := strings.TrimPrefix(r.URL.Path, "/api/v2/transactions/")
	if txID == "" || txID == "active" {
		return
	}
	var tx models.CommitTransaction
	if err := h.db.Where("tx_id = ?", txID).First(&tx).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeError(w, http.StatusNotFound, "transaction not found: "+txID)
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, tx)
}

// GET /api/v2/transactions?tx_type=&tx_phase=&target_session_id=&created_after=&created_before=
func (h *Handler) handleList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	q := h.db.Model(&models.CommitTransaction{})

	if txType := r.URL.Query().Get("tx_type"); txType != "" {
		q = q.Where("tx_type = ?", txType)
	}
	if txPhase := r.URL.Query().Get("tx_phase"); txPhase != "" {
		q = q.Where("tx_phase = ?", txPhase)
	}
	if sessionID := r.URL.Query().Get("target_session_id"); sessionID != "" {
		q = q.Where("target_session_id = ?", sessionID)
	}
	if after := r.URL.Query().Get("created_after"); after != "" {
		if t, err := time.Parse(time.RFC3339, after); err == nil {
			q = q.Where("created_at >= ?", t)
		}
	}
	if before := r.URL.Query().Get("created_before"); before != "" {
		if t, err := time.Parse(time.RFC3339, before); err == nil {
			q = q.Where("created_at <= ?", t)
		}
	}

	var txs []models.CommitTransaction
	if err := q.Order("created_at DESC").Find(&txs).Error; err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, txs)
}

// GET /api/v2/transactions/active
func (h *Handler) handleActive(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var txs []models.CommitTransaction
	if err := h.db.Where("tx_phase NOT IN ?", []string{"Committed", "RolledBack", "Failed"}).
		Order("created_at DESC").Find(&txs).Error; err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, txs)
}
