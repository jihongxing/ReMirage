// Package commit - CommitTransaction 结构体定义
package commit

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// CommitTransaction 提交事务对象
type CommitTransaction struct {
	TxID               string          `json:"tx_id" gorm:"primaryKey;size:64"`
	TxType             TxType          `json:"tx_type" gorm:"size:32;not null;index;check:tx_type IN ('PersonaSwitch','LinkMigration','GatewayReassignment','SurvivalModeSwitch')"`
	TxPhase            TxPhase         `json:"tx_phase" gorm:"size:16;not null;index;check:tx_phase IN ('Preparing','Validating','ShadowWriting','Flipping','Acknowledging','Committed','RolledBack','Failed')"`
	TxScope            TxScope         `json:"tx_scope" gorm:"size:16;not null;check:tx_scope IN ('Session','Link','Global')"`
	TargetSessionID    string          `json:"target_session_id" gorm:"index;size:64"`
	TargetLinkID       string          `json:"target_link_id" gorm:"size:64"`
	TargetPersonaID    string          `json:"target_persona_id" gorm:"size:64"`
	TargetSurvivalMode string          `json:"target_survival_mode" gorm:"size:16"`
	PrepareState       json.RawMessage `json:"prepare_state" gorm:"type:jsonb;default:'{}'"`
	ValidateState      json.RawMessage `json:"validate_state" gorm:"type:jsonb;default:'{}'"`
	ShadowState        json.RawMessage `json:"shadow_state" gorm:"type:jsonb;default:'{}'"`
	FlipState          json.RawMessage `json:"flip_state" gorm:"type:jsonb;default:'{}'"`
	AckState           json.RawMessage `json:"ack_state" gorm:"type:jsonb;default:'{}'"`
	CommitState        json.RawMessage `json:"commit_state" gorm:"type:jsonb;default:'{}'"`
	RollbackMarker     uint64          `json:"rollback_marker" gorm:"not null;default:0"`
	CreatedAt          time.Time       `json:"created_at" gorm:"index;autoCreateTime"`
	FinishedAt         *time.Time      `json:"finished_at,omitempty"`
}

// TableName 指定表名
func (CommitTransaction) TableName() string { return "commit_transactions" }

// NewCommitTransaction 创建新事务
func NewCommitTransaction(txType TxType, lastSuccessfulEpoch uint64) *CommitTransaction {
	return &CommitTransaction{
		TxID:           uuid.New().String(),
		TxType:         txType,
		TxPhase:        TxPhasePreparing,
		TxScope:        TxTypeScopeMap[txType],
		RollbackMarker: lastSuccessfulEpoch,
		PrepareState:   json.RawMessage(`{}`),
		ValidateState:  json.RawMessage(`{}`),
		ShadowState:    json.RawMessage(`{}`),
		FlipState:      json.RawMessage(`{}`),
		AckState:       json.RawMessage(`{}`),
		CommitState:    json.RawMessage(`{}`),
	}
}
