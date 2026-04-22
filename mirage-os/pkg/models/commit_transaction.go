// Package models - CommitTransaction GORM 模型（mirage-os 侧）
package models

import (
	"encoding/json"
	"time"
)

// CommitTransaction 提交事务对象
type CommitTransaction struct {
	TxID               string          `gorm:"column:tx_id;primaryKey;size:64" json:"tx_id"`
	TxType             string          `gorm:"column:tx_type;size:32;not null;index;check:tx_type IN ('PersonaSwitch','LinkMigration','GatewayReassignment','SurvivalModeSwitch')" json:"tx_type"`
	TxPhase            string          `gorm:"column:tx_phase;size:16;not null;index;check:tx_phase IN ('Preparing','Validating','ShadowWriting','Flipping','Acknowledging','Committed','RolledBack','Failed')" json:"tx_phase"`
	TxScope            string          `gorm:"column:tx_scope;size:16;not null;check:tx_scope IN ('Session','Link','Global')" json:"tx_scope"`
	TargetSessionID    string          `gorm:"column:target_session_id;index;size:64" json:"target_session_id"`
	TargetLinkID       string          `gorm:"column:target_link_id;size:64" json:"target_link_id"`
	TargetPersonaID    string          `gorm:"column:target_persona_id;size:64" json:"target_persona_id"`
	TargetSurvivalMode string          `gorm:"column:target_survival_mode;size:16" json:"target_survival_mode"`
	PrepareState       json.RawMessage `gorm:"column:prepare_state;type:jsonb;default:'{}'" json:"prepare_state"`
	ValidateState      json.RawMessage `gorm:"column:validate_state;type:jsonb;default:'{}'" json:"validate_state"`
	ShadowState        json.RawMessage `gorm:"column:shadow_state;type:jsonb;default:'{}'" json:"shadow_state"`
	FlipState          json.RawMessage `gorm:"column:flip_state;type:jsonb;default:'{}'" json:"flip_state"`
	AckState           json.RawMessage `gorm:"column:ack_state;type:jsonb;default:'{}'" json:"ack_state"`
	CommitState        json.RawMessage `gorm:"column:commit_state;type:jsonb;default:'{}'" json:"commit_state"`
	RollbackMarker     uint64          `gorm:"column:rollback_marker;not null;default:0" json:"rollback_marker"`
	CreatedAt          time.Time       `gorm:"column:created_at;index;autoCreateTime" json:"created_at"`
	FinishedAt         *time.Time      `gorm:"column:finished_at" json:"finished_at,omitempty"`
}

// TableName 指定表名
func (CommitTransaction) TableName() string { return "commit_transactions" }
