package audit

import (
	"time"

	"mirage-gateway/pkg/orchestrator"
	"mirage-gateway/pkg/orchestrator/commit"
)

// SessionDiagnostic Session 诊断视图
type SessionDiagnostic struct {
	SessionID             string                    `json:"session_id"`
	CurrentLinkID         string                    `json:"current_link_id"`
	CurrentLinkPhase      orchestrator.LinkPhase    `json:"current_link_phase"`
	CurrentPersonaID      string                    `json:"current_persona_id"`
	CurrentPersonaVersion uint64                    `json:"current_persona_version"`
	CurrentSurvivalMode   orchestrator.SurvivalMode `json:"current_survival_mode"`
	SessionState          orchestrator.SessionPhase `json:"session_state"`
	LastSwitchReason      string                    `json:"last_switch_reason"`
	LastRollbackReason    string                    `json:"last_rollback_reason"`
}

// SystemDiagnostic 系统诊断视图
type SystemDiagnostic struct {
	CurrentSurvivalMode  orchestrator.SurvivalMode `json:"current_survival_mode"`
	LastModeSwitchReason string                    `json:"last_mode_switch_reason"`
	LastModeSwitchTime   *time.Time                `json:"last_mode_switch_time"`
	ActiveSessionCount   int                       `json:"active_session_count"`
	ActiveLinkCount      int                       `json:"active_link_count"`
	ActiveTransaction    *ActiveTxInfo             `json:"active_transaction"`
}

// ActiveTxInfo 活跃事务摘要
type ActiveTxInfo struct {
	TxID    string         `json:"tx_id"`
	TxType  commit.TxType  `json:"tx_type"`
	TxPhase commit.TxPhase `json:"tx_phase"`
}

// TransactionDiagnostic 事务诊断视图
type TransactionDiagnostic struct {
	TxID               string                    `json:"tx_id"`
	TxType             commit.TxType             `json:"tx_type"`
	CurrentPhase       commit.TxPhase            `json:"current_phase"`
	PhaseDurations     map[string]time.Duration  `json:"phase_durations"`
	StuckDuration      time.Duration             `json:"stuck_duration"`
	TargetSessionID    string                    `json:"target_session_id"`
	TargetSurvivalMode orchestrator.SurvivalMode `json:"target_survival_mode"`
}
