package audit

import (
	"encoding/json"
	"time"

	"mirage-gateway/pkg/orchestrator/commit"
)

// AuditRecord 事务审计记录
type AuditRecord struct {
	AuditID           string          `json:"audit_id" gorm:"primaryKey;size:64"`
	TxID              string          `json:"tx_id" gorm:"index;size:64;not null"`
	TxType            commit.TxType   `json:"tx_type" gorm:"index;size:32;not null"`
	InitiatedAt       time.Time       `json:"initiated_at" gorm:"index;not null"`
	FinishedAt        time.Time       `json:"finished_at" gorm:"not null"`
	InitiationReason  string          `json:"initiation_reason" gorm:"size:256"`
	TargetState       json.RawMessage `json:"target_state" gorm:"type:jsonb;default:'{}'"`
	BudgetVerdict     string          `json:"budget_verdict" gorm:"size:32"`
	DenyReason        string          `json:"deny_reason,omitempty" gorm:"size:256"`
	FlipSuccess       bool            `json:"flip_success" gorm:"not null;default:false"`
	RollbackTriggered bool            `json:"rollback_triggered" gorm:"index;not null;default:false"`
	CreatedAt         time.Time       `json:"created_at" gorm:"autoCreateTime"`
}

// TableName 指定表名
func (AuditRecord) TableName() string { return "audit_records" }

// Validate 校验 AuditRecord 必填字段
func (r *AuditRecord) Validate() error {
	if r.AuditID == "" {
		return &ErrInvalidAuditRecord{Field: "audit_id", Message: "must not be empty"}
	}
	if r.TxID == "" {
		return &ErrInvalidAuditRecord{Field: "tx_id", Message: "must not be empty"}
	}
	if r.TxType == "" {
		return &ErrInvalidAuditRecord{Field: "tx_type", Message: "must not be empty"}
	}
	if r.InitiatedAt.IsZero() {
		return &ErrInvalidAuditRecord{Field: "initiated_at", Message: "must not be zero"}
	}
	if r.FinishedAt.IsZero() {
		return &ErrInvalidAuditRecord{Field: "finished_at", Message: "must not be zero"}
	}
	return nil
}
