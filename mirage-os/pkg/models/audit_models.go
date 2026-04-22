package models

import (
	"encoding/json"
	"time"
)

// ============================================
// V2 观测与审计模型
// ============================================

// V2AuditRecord mirrors audit.AuditRecord for GORM AutoMigrate
type V2AuditRecord struct {
	AuditID           string          `gorm:"primaryKey;size:64"`
	TxID              string          `gorm:"index;size:64;not null"`
	TxType            string          `gorm:"index;size:32;not null"`
	InitiatedAt       time.Time       `gorm:"index;not null"`
	FinishedAt        time.Time       `gorm:"not null"`
	InitiationReason  string          `gorm:"size:256"`
	TargetState       json.RawMessage `gorm:"type:jsonb;default:'{}'"`
	BudgetVerdict     string          `gorm:"size:32"`
	DenyReason        string          `gorm:"size:256"`
	FlipSuccess       bool            `gorm:"not null;default:false"`
	RollbackTriggered bool            `gorm:"index;not null;default:false"`
	CreatedAt         time.Time       `gorm:"autoCreateTime"`
}

// TableName 指定表名
func (V2AuditRecord) TableName() string { return "audit_records" }

// V2SessionTimeline mirrors audit.SessionTimelineEntry
type V2SessionTimeline struct {
	EntryID      string    `gorm:"primaryKey;size:64"`
	SessionID    string    `gorm:"index;size:64;not null"`
	FromState    string    `gorm:"size:16;not null"`
	ToState      string    `gorm:"size:16;not null"`
	Reason       string    `gorm:"size:256"`
	LinkID       string    `gorm:"size:64"`
	PersonaID    string    `gorm:"size:64"`
	SurvivalMode string    `gorm:"size:16"`
	Timestamp    time.Time `gorm:"index;not null"`
}

// TableName 指定表名
func (V2SessionTimeline) TableName() string { return "session_timeline" }

// V2LinkHealthTimeline mirrors audit.LinkHealthTimelineEntry
type V2LinkHealthTimeline struct {
	EntryID     string  `gorm:"primaryKey;size:64"`
	LinkID      string  `gorm:"index;size:64;not null"`
	HealthScore float64 `gorm:"type:numeric(5,2)"`
	RTTMs       int64
	LossRate    float64 `gorm:"type:numeric(5,4)"`
	JitterMs    int64
	Phase       string    `gorm:"size:16;not null"`
	EventType   string    `gorm:"size:32;not null"`
	Timestamp   time.Time `gorm:"index;not null"`
}

// TableName 指定表名
func (V2LinkHealthTimeline) TableName() string { return "link_health_timeline" }

// V2PersonaVersionTimeline mirrors audit.PersonaVersionTimelineEntry
type V2PersonaVersionTimeline struct {
	EntryID     string `gorm:"primaryKey;size:64"`
	SessionID   string `gorm:"index;size:64;not null"`
	PersonaID   string `gorm:"index;size:64;not null"`
	FromVersion uint64
	ToVersion   uint64
	EventType   string    `gorm:"size:32;not null"`
	Timestamp   time.Time `gorm:"index;not null"`
}

// TableName 指定表名
func (V2PersonaVersionTimeline) TableName() string { return "persona_version_timeline" }

// V2SurvivalModeTimeline mirrors audit.SurvivalModeTimelineEntry
type V2SurvivalModeTimeline struct {
	EntryID   string          `gorm:"primaryKey;size:64"`
	FromMode  string          `gorm:"size:16;not null"`
	ToMode    string          `gorm:"size:16;not null"`
	Triggers  json.RawMessage `gorm:"type:jsonb"`
	TxID      string          `gorm:"size:64"`
	Timestamp time.Time       `gorm:"index;not null"`
}

// TableName 指定表名
func (V2SurvivalModeTimeline) TableName() string { return "survival_mode_timeline" }

// V2TransactionTimeline mirrors audit.TransactionTimelineEntry
type V2TransactionTimeline struct {
	EntryID   string          `gorm:"primaryKey;size:64"`
	TxID      string          `gorm:"index;size:64;not null"`
	FromPhase string          `gorm:"size:16;not null"`
	ToPhase   string          `gorm:"size:16;not null"`
	PhaseData json.RawMessage `gorm:"type:jsonb;default:'{}'"`
	Timestamp time.Time       `gorm:"index;not null"`
}

// TableName 指定表名
func (V2TransactionTimeline) TableName() string { return "transaction_timeline" }
