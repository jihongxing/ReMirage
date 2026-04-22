// Package observabilityquery - V2 观测与审计查询 API
package observabilityquery

import (
	"encoding/json"
	"mirage-os/pkg/models"
	"net/http"
	"strings"
	"time"

	"gorm.io/gorm"
)

// Handler Observability Query API 处理器
type Handler struct {
	db *gorm.DB
}

// NewHandler 创建 Handler 实例
func NewHandler(db *gorm.DB) *Handler {
	return &Handler{db: db}
}

// RegisterRoutes 注册路由到 ServeMux
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v2/audit/records", h.handleAuditRecords)
	mux.HandleFunc("/api/v2/audit/records/", h.handleAuditRecordDetail)
	mux.HandleFunc("/api/v2/timelines/sessions/", h.handleSessionTimeline)
	mux.HandleFunc("/api/v2/timelines/links/", h.handleLinkHealthTimeline)
	mux.HandleFunc("/api/v2/timelines/personas/", h.handlePersonaTimeline)
	mux.HandleFunc("/api/v2/timelines/survival-modes", h.handleSurvivalModeTimeline)
	mux.HandleFunc("/api/v2/timelines/transactions/", h.handleTransactionTimeline)
	mux.HandleFunc("/api/v2/diagnostics/sessions/", h.handleSessionDiagnostic)
	mux.HandleFunc("/api/v2/diagnostics/system", h.handleSystemDiagnostic)
	mux.HandleFunc("/api/v2/diagnostics/transactions/", h.handleTransactionDiagnostic)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// applyTimeRange 对查询应用 start/end 时间范围过滤
func applyTimeRange(q *gorm.DB, r *http.Request) *gorm.DB {
	if start := r.URL.Query().Get("start"); start != "" {
		if t, err := time.Parse(time.RFC3339, start); err == nil {
			q = q.Where("timestamp >= ?", t)
		}
	}
	if end := r.URL.Query().Get("end"); end != "" {
		if t, err := time.Parse(time.RFC3339, end); err == nil {
			q = q.Where("timestamp <= ?", t)
		}
	}
	return q
}

// 9.1 GET /api/v2/audit/records?tx_type=&start=&end=&rollback_triggered=
func (h *Handler) handleAuditRecords(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	q := h.db.Model(&models.V2AuditRecord{})
	if txType := r.URL.Query().Get("tx_type"); txType != "" {
		q = q.Where("tx_type = ?", txType)
	}
	if start := r.URL.Query().Get("start"); start != "" {
		t, err := time.Parse(time.RFC3339, start)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid start time: "+err.Error())
			return
		}
		q = q.Where("initiated_at >= ?", t)
	}
	if end := r.URL.Query().Get("end"); end != "" {
		t, err := time.Parse(time.RFC3339, end)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid end time: "+err.Error())
			return
		}
		q = q.Where("initiated_at <= ?", t)
	}
	if rb := r.URL.Query().Get("rollback_triggered"); rb != "" {
		switch rb {
		case "true":
			q = q.Where("rollback_triggered = ?", true)
		case "false":
			q = q.Where("rollback_triggered = ?", false)
		default:
			writeError(w, http.StatusBadRequest, "rollback_triggered must be true or false")
			return
		}
	}
	var records []models.V2AuditRecord
	if err := q.Order("initiated_at DESC").Find(&records).Error; err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, records)
}

// 9.2 GET /api/v2/audit/records/{tx_id}
func (h *Handler) handleAuditRecordDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	txID := strings.TrimPrefix(r.URL.Path, "/api/v2/audit/records/")
	if txID == "" {
		writeError(w, http.StatusBadRequest, "tx_id is required")
		return
	}
	var record models.V2AuditRecord
	if err := h.db.Where("tx_id = ?", txID).First(&record).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeError(w, http.StatusNotFound, "audit record not found: "+txID)
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, record)
}

// 9.3 GET /api/v2/timelines/sessions/{session_id}?start=&end=
func (h *Handler) handleSessionTimeline(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	sessionID := strings.TrimPrefix(r.URL.Path, "/api/v2/timelines/sessions/")
	if sessionID == "" {
		writeError(w, http.StatusBadRequest, "session_id is required")
		return
	}
	q := h.db.Where("session_id = ?", sessionID)
	q = applyTimeRange(q, r)
	var entries []models.V2SessionTimeline
	if err := q.Order("timestamp ASC").Find(&entries).Error; err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, entries)
}

// 9.4 GET /api/v2/timelines/links/{link_id}/health?start=&end=
func (h *Handler) handleLinkHealthTimeline(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/v2/timelines/links/")
	linkID, ok := strings.CutSuffix(path, "/health")
	if !ok || linkID == "" {
		writeError(w, http.StatusBadRequest, "link_id is required (use /api/v2/timelines/links/{link_id}/health)")
		return
	}
	q := h.db.Where("link_id = ?", linkID)
	q = applyTimeRange(q, r)
	var entries []models.V2LinkHealthTimeline
	if err := q.Order("timestamp ASC").Find(&entries).Error; err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, entries)
}

// 9.5 GET /api/v2/timelines/personas/{session_id}?start=&end=
func (h *Handler) handlePersonaTimeline(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	sessionID := strings.TrimPrefix(r.URL.Path, "/api/v2/timelines/personas/")
	if sessionID == "" {
		writeError(w, http.StatusBadRequest, "session_id is required")
		return
	}
	q := h.db.Where("session_id = ?", sessionID)
	q = applyTimeRange(q, r)
	var entries []models.V2PersonaVersionTimeline
	if err := q.Order("timestamp ASC").Find(&entries).Error; err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, entries)
}

// 9.6 GET /api/v2/timelines/survival-modes?start=&end=
func (h *Handler) handleSurvivalModeTimeline(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	q := h.db.Model(&models.V2SurvivalModeTimeline{})
	q = applyTimeRange(q, r)
	var entries []models.V2SurvivalModeTimeline
	if err := q.Order("timestamp ASC").Find(&entries).Error; err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, entries)
}

// 9.7 GET /api/v2/timelines/transactions/{tx_id}
func (h *Handler) handleTransactionTimeline(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	txID := strings.TrimPrefix(r.URL.Path, "/api/v2/timelines/transactions/")
	if txID == "" {
		writeError(w, http.StatusBadRequest, "tx_id is required")
		return
	}
	var entries []models.V2TransactionTimeline
	if err := h.db.Where("tx_id = ?", txID).Order("timestamp ASC").Find(&entries).Error; err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, entries)
}

// 9.8 GET /api/v2/diagnostics/sessions/{session_id}
func (h *Handler) handleSessionDiagnostic(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	sessionID := strings.TrimPrefix(r.URL.Path, "/api/v2/diagnostics/sessions/")
	if sessionID == "" {
		writeError(w, http.StatusBadRequest, "session_id is required")
		return
	}

	// 查询 session_states
	var session models.V2SessionState
	if err := h.db.Where("session_id = ?", sessionID).First(&session).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeError(w, http.StatusNotFound, "session not found: "+sessionID)
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// 查询当前 link phase
	var linkPhase string
	if session.CurrentLinkID != "" {
		var link models.V2LinkState
		if err := h.db.Where("link_id = ?", session.CurrentLinkID).First(&link).Error; err == nil {
			linkPhase = link.Phase
		}
	}

	// 查询 persona_version from control_states
	var personaVersion uint64
	var control models.V2ControlState
	if err := h.db.Where("gateway_id = ?", session.GatewayID).First(&control).Error; err == nil {
		personaVersion = control.PersonaVersion
	}

	// 查询最近一次切换原因和回滚原因
	var lastSwitchReason, lastRollbackReason string
	var lastSwitch models.V2SessionTimeline
	if err := h.db.Where("session_id = ?", sessionID).
		Order("timestamp DESC").First(&lastSwitch).Error; err == nil {
		lastSwitchReason = lastSwitch.Reason
	}
	var lastRollback models.V2PersonaVersionTimeline
	if err := h.db.Where("session_id = ? AND event_type = ?", sessionID, "rollback").
		Order("timestamp DESC").First(&lastRollback).Error; err == nil {
		lastRollbackReason = "persona rollback v" + formatUint(lastRollback.FromVersion) + " -> v" + formatUint(lastRollback.ToVersion)
	}

	diag := sessionDiagnosticResponse{
		SessionID:             sessionID,
		CurrentLinkID:         session.CurrentLinkID,
		CurrentLinkPhase:      linkPhase,
		CurrentPersonaID:      session.CurrentPersonaID,
		CurrentPersonaVersion: personaVersion,
		CurrentSurvivalMode:   session.CurrentSurvivalMode,
		SessionState:          session.State,
		LastSwitchReason:      lastSwitchReason,
		LastRollbackReason:    lastRollbackReason,
	}
	writeJSON(w, http.StatusOK, diag)
}

// 9.9 GET /api/v2/diagnostics/system
func (h *Handler) handleSystemDiagnostic(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// 活跃 session 数（非 Closed）
	var activeSessionCount int64
	h.db.Model(&models.V2SessionState{}).Where("state != ?", models.SessionPhaseClosed).Count(&activeSessionCount)

	// 活跃 link 数（Active 或 Degrading）
	var activeLinkCount int64
	h.db.Model(&models.V2LinkState{}).Where("phase IN ?", []string{models.LinkPhaseActive, models.LinkPhaseDegrading}).Count(&activeLinkCount)

	// 当前 survival mode：从最新的 survival_mode_timeline 条目获取
	var currentMode string
	var lastModeReason string
	var lastModeTime *time.Time
	var latestMode models.V2SurvivalModeTimeline
	if err := h.db.Order("timestamp DESC").First(&latestMode).Error; err == nil {
		currentMode = latestMode.ToMode
		lastModeReason = "transition from " + latestMode.FromMode + " to " + latestMode.ToMode
		lastModeTime = &latestMode.Timestamp
	} else {
		currentMode = "Normal"
	}

	// 活跃事务
	var activeTx *activeTxResponse
	var control models.V2ControlState
	if err := h.db.Where("active_tx_id != ''").First(&control).Error; err == nil {
		var tx models.CommitTransaction
		if err := h.db.Where("tx_id = ?", control.ActiveTxID).First(&tx).Error; err == nil {
			activeTx = &activeTxResponse{
				TxID:    tx.TxID,
				TxType:  tx.TxType,
				TxPhase: tx.TxPhase,
			}
		}
	}

	diag := systemDiagnosticResponse{
		CurrentSurvivalMode:  currentMode,
		LastModeSwitchReason: lastModeReason,
		LastModeSwitchTime:   lastModeTime,
		ActiveSessionCount:   int(activeSessionCount),
		ActiveLinkCount:      int(activeLinkCount),
		ActiveTransaction:    activeTx,
	}
	writeJSON(w, http.StatusOK, diag)
}

// 9.10 GET /api/v2/diagnostics/transactions/{tx_id}
func (h *Handler) handleTransactionDiagnostic(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	txID := strings.TrimPrefix(r.URL.Path, "/api/v2/diagnostics/transactions/")
	if txID == "" {
		writeError(w, http.StatusBadRequest, "tx_id is required")
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

	// 查询 transaction_timeline 计算 phase_durations
	var entries []models.V2TransactionTimeline
	h.db.Where("tx_id = ?", txID).Order("timestamp ASC").Find(&entries)

	phaseDurations := make(map[string]string)
	for i := 1; i < len(entries); i++ {
		dur := entries[i].Timestamp.Sub(entries[i-1].Timestamp)
		phaseDurations[entries[i-1].ToPhase] = dur.String()
	}

	// stuck_duration：非终态时计算
	var stuckDuration string
	terminalPhases := map[string]bool{"Committed": true, "RolledBack": true, "Failed": true}
	if !terminalPhases[tx.TxPhase] {
		if len(entries) > 0 {
			lastEntry := entries[len(entries)-1]
			stuckDuration = time.Since(lastEntry.Timestamp).Truncate(time.Second).String()
		} else {
			stuckDuration = time.Since(tx.CreatedAt).Truncate(time.Second).String()
		}
	}

	diag := transactionDiagnosticResponse{
		TxID:               tx.TxID,
		TxType:             tx.TxType,
		CurrentPhase:       tx.TxPhase,
		PhaseDurations:     phaseDurations,
		StuckDuration:      stuckDuration,
		TargetSessionID:    tx.TargetSessionID,
		TargetSurvivalMode: tx.TargetSurvivalMode,
	}
	writeJSON(w, http.StatusOK, diag)
}

// ============================================
// 响应结构体
// ============================================

type sessionDiagnosticResponse struct {
	SessionID             string `json:"session_id"`
	CurrentLinkID         string `json:"current_link_id"`
	CurrentLinkPhase      string `json:"current_link_phase"`
	CurrentPersonaID      string `json:"current_persona_id"`
	CurrentPersonaVersion uint64 `json:"current_persona_version"`
	CurrentSurvivalMode   string `json:"current_survival_mode"`
	SessionState          string `json:"session_state"`
	LastSwitchReason      string `json:"last_switch_reason"`
	LastRollbackReason    string `json:"last_rollback_reason"`
}

type systemDiagnosticResponse struct {
	CurrentSurvivalMode  string            `json:"current_survival_mode"`
	LastModeSwitchReason string            `json:"last_mode_switch_reason"`
	LastModeSwitchTime   *time.Time        `json:"last_mode_switch_time"`
	ActiveSessionCount   int               `json:"active_session_count"`
	ActiveLinkCount      int               `json:"active_link_count"`
	ActiveTransaction    *activeTxResponse `json:"active_transaction"`
}

type activeTxResponse struct {
	TxID    string `json:"tx_id"`
	TxType  string `json:"tx_type"`
	TxPhase string `json:"tx_phase"`
}

type transactionDiagnosticResponse struct {
	TxID               string            `json:"tx_id"`
	TxType             string            `json:"tx_type"`
	CurrentPhase       string            `json:"current_phase"`
	PhaseDurations     map[string]string `json:"phase_durations"`
	StuckDuration      string            `json:"stuck_duration"`
	TargetSessionID    string            `json:"target_session_id"`
	TargetSurvivalMode string            `json:"target_survival_mode"`
}

func formatUint(v uint64) string {
	if v == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte(v%10) + '0'
		v /= 10
	}
	return string(buf[i:])
}
