package audit

import (
	"encoding/json"
	"time"

	"mirage-gateway/pkg/orchestrator"
	"mirage-gateway/pkg/orchestrator/commit"
)

// SessionTimelineEntry Session 时间线条目
type SessionTimelineEntry struct {
	EntryID      string                    `json:"entry_id" gorm:"primaryKey;size:64"`
	SessionID    string                    `json:"session_id" gorm:"index;size:64;not null"`
	FromState    orchestrator.SessionPhase `json:"from_state" gorm:"size:16;not null"`
	ToState      orchestrator.SessionPhase `json:"to_state" gorm:"size:16;not null"`
	Reason       string                    `json:"reason" gorm:"size:256"`
	LinkID       string                    `json:"link_id" gorm:"size:64"`
	PersonaID    string                    `json:"persona_id" gorm:"size:64"`
	SurvivalMode orchestrator.SurvivalMode `json:"survival_mode" gorm:"size:16"`
	Timestamp    time.Time                 `json:"timestamp" gorm:"index;not null"`
}

// TableName 指定表名
func (SessionTimelineEntry) TableName() string { return "session_timeline" }

// LinkHealthTimelineEntry Link 健康时间线条目
type LinkHealthTimelineEntry struct {
	EntryID     string                 `json:"entry_id" gorm:"primaryKey;size:64"`
	LinkID      string                 `json:"link_id" gorm:"index;size:64;not null"`
	HealthScore float64                `json:"health_score" gorm:"type:numeric(5,2)"`
	RTTMs       int64                  `json:"rtt_ms"`
	LossRate    float64                `json:"loss_rate" gorm:"type:numeric(5,4)"`
	JitterMs    int64                  `json:"jitter_ms"`
	Phase       orchestrator.LinkPhase `json:"phase" gorm:"size:16;not null"`
	EventType   string                 `json:"event_type" gorm:"size:32;not null"`
	Timestamp   time.Time              `json:"timestamp" gorm:"index;not null"`
}

// TableName 指定表名
func (LinkHealthTimelineEntry) TableName() string { return "link_health_timeline" }

// PersonaVersionTimelineEntry Persona 版本时间线条目
type PersonaVersionTimelineEntry struct {
	EntryID     string    `json:"entry_id" gorm:"primaryKey;size:64"`
	SessionID   string    `json:"session_id" gorm:"index;size:64;not null"`
	PersonaID   string    `json:"persona_id" gorm:"index;size:64;not null"`
	FromVersion uint64    `json:"from_version"`
	ToVersion   uint64    `json:"to_version"`
	EventType   string    `json:"event_type" gorm:"size:32;not null"`
	Timestamp   time.Time `json:"timestamp" gorm:"index;not null"`
}

// TableName 指定表名
func (PersonaVersionTimelineEntry) TableName() string { return "persona_version_timeline" }

// SurvivalModeTimelineEntry Survival Mode 时间线条目
type SurvivalModeTimelineEntry struct {
	EntryID   string                    `json:"entry_id" gorm:"primaryKey;size:64"`
	FromMode  orchestrator.SurvivalMode `json:"from_mode" gorm:"size:16;not null"`
	ToMode    orchestrator.SurvivalMode `json:"to_mode" gorm:"size:16;not null"`
	Triggers  json.RawMessage           `json:"triggers" gorm:"type:jsonb"`
	TxID      string                    `json:"tx_id" gorm:"size:64"`
	Timestamp time.Time                 `json:"timestamp" gorm:"index;not null"`
}

// TableName 指定表名
func (SurvivalModeTimelineEntry) TableName() string { return "survival_mode_timeline" }

// TransactionTimelineEntry Transaction 时间线条目
type TransactionTimelineEntry struct {
	EntryID   string          `json:"entry_id" gorm:"primaryKey;size:64"`
	TxID      string          `json:"tx_id" gorm:"index;size:64;not null"`
	FromPhase commit.TxPhase  `json:"from_phase" gorm:"size:16;not null"`
	ToPhase   commit.TxPhase  `json:"to_phase" gorm:"size:16;not null"`
	PhaseData json.RawMessage `json:"phase_data" gorm:"type:jsonb;default:'{}'"`
	Timestamp time.Time       `json:"timestamp" gorm:"index;not null"`
}

// TableName 指定表名
func (TransactionTimelineEntry) TableName() string { return "transaction_timeline" }
