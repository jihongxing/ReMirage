// Package personaquery - V2 Persona Query API
package personaquery

import (
	"encoding/json"
	"mirage-os/pkg/models"
	"net/http"
	"strings"

	"gorm.io/gorm"
)

// Handler Persona Query API 处理器
type Handler struct {
	db *gorm.DB
}

// NewHandler 创建 Handler 实例
func NewHandler(db *gorm.DB) *Handler {
	return &Handler{db: db}
}

// RegisterRoutes 注册路由到 ServeMux
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v2/personas/", h.handlePersonaRoutes)
	mux.HandleFunc("/api/v2/sessions/", h.handleSessionPersona)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// 路由分发：/api/v2/personas/{persona_id} 和 /api/v2/personas/{persona_id}/versions
func (h *Handler) handlePersonaRoutes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/v2/personas/")
	if strings.HasSuffix(path, "/versions") {
		personaID := strings.TrimSuffix(path, "/versions")
		h.handleListVersions(w, personaID)
		return
	}
	h.handleGetLatest(w, path)
}

// GET /api/v2/personas/{persona_id} - 返回最新版本
func (h *Handler) handleGetLatest(w http.ResponseWriter, personaID string) {
	if personaID == "" {
		writeError(w, http.StatusBadRequest, "persona_id is required")
		return
	}
	var manifest models.PersonaManifest
	err := h.db.Where("persona_id = ?", personaID).
		Order("version DESC").
		First(&manifest).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			writeError(w, http.StatusNotFound, "persona not found: "+personaID)
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, manifest)
}

// GET /api/v2/personas/{persona_id}/versions - 返回全部版本列表
func (h *Handler) handleListVersions(w http.ResponseWriter, personaID string) {
	if personaID == "" {
		writeError(w, http.StatusBadRequest, "persona_id is required")
		return
	}
	var manifests []models.PersonaManifest
	err := h.db.Where("persona_id = ?", personaID).
		Order("version DESC").
		Find(&manifests).Error
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if len(manifests) == 0 {
		writeError(w, http.StatusNotFound, "persona not found: "+personaID)
		return
	}
	writeJSON(w, http.StatusOK, manifests)
}

// GET /api/v2/sessions/{session_id}/persona - 返回 Session 当前 Active Persona
func (h *Handler) handleSessionPersona(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/v2/sessions/")
	if path == "" || !strings.HasSuffix(path, "/persona") {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	sessionID := strings.TrimSuffix(path, "/persona")
	if sessionID == "" || strings.Contains(sessionID, "/") {
		writeError(w, http.StatusBadRequest, "session_id is required")
		return
	}

	// 查找 Session 的 current_persona_id
	var session models.V2SessionState
	err := h.db.Where("session_id = ?", sessionID).First(&session).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			writeError(w, http.StatusNotFound, "session not found: "+sessionID)
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if session.CurrentPersonaID == "" {
		writeError(w, http.StatusNotFound, "no active persona for session: "+sessionID)
		return
	}

	// 查找 Active 状态的 Persona
	var manifest models.PersonaManifest
	err = h.db.Where("persona_id = ? AND lifecycle = ?", session.CurrentPersonaID, models.PersonaLifecycleActive).
		First(&manifest).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			writeError(w, http.StatusNotFound, "active persona not found for session: "+sessionID)
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, manifest)
}
