package survival

import (
	"context"
	"time"

	orchestrator "mirage-gateway/pkg/orchestrator"
	"mirage-gateway/pkg/orchestrator/budget"
)

// TriggerSignal 触发信号
type TriggerSignal struct {
	Source    TriggerSource          `json:"source"`
	Reason    string                 `json:"reason"`
	Severity  int                    `json:"severity"` // 0-5，对应 ModeSeverity
	Timestamp time.Time              `json:"timestamp"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// ModeTransitionAdvice 模式迁移建议
type ModeTransitionAdvice struct {
	TargetMode orchestrator.SurvivalMode `json:"target_mode"`
	Triggers   []TriggerSignal           `json:"triggers"`
	Confidence float64                   `json:"confidence"`
}

// TriggerEvaluatorIface 触发因素评估器接口
type TriggerEvaluatorIface interface {
	Evaluate(ctx context.Context, currentMode orchestrator.SurvivalMode) (*ModeTransitionAdvice, error)
	SubmitSignal(signal *TriggerSignal)
}

// LinkHealthTriggerIface 链路健康触发器接口
type LinkHealthTriggerIface interface {
	Evaluate(ctx context.Context, links []*orchestrator.LinkState) *TriggerSignal
}

// EntryBurnTriggerIface 入口战死触发器接口
type EntryBurnTriggerIface interface {
	Evaluate(ctx context.Context, burnCount int, threshold int) *TriggerSignal
}

// BudgetTriggerIface 预算触发器接口
type BudgetTriggerIface interface {
	Evaluate(ctx context.Context, verdict budget.BudgetVerdict) *TriggerSignal
}

// PolicyTriggerIface 策略指令触发器接口
type PolicyTriggerIface interface {
	Evaluate(ctx context.Context, targetMode orchestrator.SurvivalMode, reason string) *TriggerSignal
}
