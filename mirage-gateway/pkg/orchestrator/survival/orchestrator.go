package survival

import (
	"context"
	"time"

	orchestrator "mirage-gateway/pkg/orchestrator"
)

// SurvivalOrchestratorIface 生存编排器接口
type SurvivalOrchestratorIface interface {
	GetCurrentMode() orchestrator.SurvivalMode
	GetCurrentPolicy() *ModePolicy
	RequestTransition(ctx context.Context, target orchestrator.SurvivalMode, triggers []TriggerSignal) error
	EvaluateAndTransition(ctx context.Context) error
	CheckAdmission(serviceClass orchestrator.ServiceClass) error
	GetTransitionHistory(n int) []*TransitionRecord
	RecoverOnStartup(ctx context.Context) error
}

// TransitionRecord 迁移历史记录
type TransitionRecord struct {
	FromMode  orchestrator.SurvivalMode `json:"from_mode"`
	ToMode    orchestrator.SurvivalMode `json:"to_mode"`
	Triggers  []TriggerSignal           `json:"triggers"`
	TxID      string                    `json:"tx_id"`
	Timestamp time.Time                 `json:"timestamp"`
}
