// Package statequery - V2 三层状态查询 API
package statequery

import (
	"encoding/json"
	"mirage-os/pkg/models"
	"net/http"
	"strings"

	"gorm.io/gorm"
)

// Handler State Query API 处理器
type Handler struct {
	db *gorm.DB
}

// NewHandler 创建 Handler 实例
func NewHandler(db *gorm.DB) *Handler {
	return &Handler{db: db}
}

// RegisterRoutes 注册路由到 ServeMux
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v2/links", h.handleLinks)
	mux.HandleFunc("/api/v2/links/", h.handleLinkDetail)
	mux.HandleFunc("/api/v2/sessions", h.handleSessions)
	mux.HandleFunc("/api/v2/sessions/", h.handleSessionRoutes)
	mux.HandleFunc("/api/v2/control/", h.handleControl)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// GET /api/v2/links?gateway_id=
func (h *Handler) handleLinks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	gatewayID := r.URL.Query().Get("gateway_id")
	if gatewayID == "" {
		writeError(w, http.StatusBadRequest, "gateway_id is required")
		return
	}
	var links []models.V2LinkState
	if err := h.db.Where("gateway_id = ?", gatewayID).Find(&links).Error; err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, links)
}

// GET /api/v2/links/{link_id}
func (h *Handler) handleLinkDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	linkID := strings.TrimPrefix(r.URL.Path, "/api/v2/links/")
	if linkID == "" {
		writeError(w, http.StatusBadRequest, "link_id is required")
		return
	}
	var link models.V2LinkState
	if err := h.db.Where("link_id = ?", linkID).First(&link).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeError(w, http.StatusNotFound, "link not found: "+linkID)
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, link)
}

// GET /api/v2/sessions?gateway_id=&user_id=&state=
func (h *Handler) handleSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	q := h.db.Model(&models.V2SessionState{})
	if gid := r.URL.Query().Get("gateway_id"); gid != "" {
		q = q.Where("gateway_id = ?", gid)
	}
	if uid := r.URL.Query().Get("user_id"); uid != "" {
		q = q.Where("user_id = ?", uid)
	}
	if state := r.URL.Query().Get("state"); state != "" {
		q = q.Where("state = ?", state)
	}
	var sessions []models.V2SessionState
	if err := q.Find(&sessions).Error; err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, sessions)
}

// 路由分发：/api/v2/sessions/{session_id} 和 /api/v2/sessions/{session_id}/topology
func (h *Handler) handleSessionRoutes(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v2/sessions/")
	if strings.HasSuffix(path, "/topology") {
		sessionID := strings.TrimSuffix(path, "/topology")
		h.handleSessionTopology(w, r, sessionID)
		return
	}
	h.handleSessionDetail(w, r, path)
}

// GET /api/v2/sessions/{session_id}
func (h *Handler) handleSessionDetail(w http.ResponseWriter, r *http.Request, sessionID string) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if sessionID == "" {
		writeError(w, http.StatusBadRequest, "session_id is required")
		return
	}
	var session models.V2SessionState
	if err := h.db.Where("session_id = ?", sessionID).First(&session).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeError(w, http.StatusNotFound, "session not found: "+sessionID)
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, session)
}

// GET /api/v2/sessions/{session_id}/topology
func (h *Handler) handleSessionTopology(w http.ResponseWriter, r *http.Request, sessionID string) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var session models.V2SessionState
	if err := h.db.Where("session_id = ?", sessionID).First(&session).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeError(w, http.StatusNotFound, "session not found: "+sessionID)
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	type TopologyView struct {
		Session models.V2SessionState  `json:"session"`
		Link    *models.V2LinkState    `json:"link,omitempty"`
		Control *models.V2ControlState `json:"control,omitempty"`
	}

	topo := TopologyView{Session: session}

	if session.CurrentLinkID != "" {
		var link models.V2LinkState
		if err := h.db.Where("link_id = ?", session.CurrentLinkID).First(&link).Error; err == nil {
			topo.Link = &link
		}
	}

	var control models.V2ControlState
	if err := h.db.Where("gateway_id = ?", session.GatewayID).First(&control).Error; err == nil {
		topo.Control = &control
	}

	writeJSON(w, http.StatusOK, topo)
}

// GET /api/v2/control/{gateway_id}
func (h *Handler) handleControl(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	gatewayID := strings.TrimPrefix(r.URL.Path, "/api/v2/control/")
	if gatewayID == "" {
		writeError(w, http.StatusBadRequest, "gateway_id is required")
		return
	}
	var control models.V2ControlState
	if err := h.db.Where("gateway_id = ?", gatewayID).First(&control).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeError(w, http.StatusNotFound, "control state not found: "+gatewayID)
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, control)
}
