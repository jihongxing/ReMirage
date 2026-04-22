package survival

import (
	"context"
	"sync"

	orchestrator "mirage-gateway/pkg/orchestrator"
)

type triggerEvaluator struct {
	mu      sync.Mutex
	signals []*TriggerSignal
}

// NewTriggerEvaluator 创建 TriggerEvaluator
func NewTriggerEvaluator() TriggerEvaluatorIface {
	return &triggerEvaluator{}
}

func (e *triggerEvaluator) SubmitSignal(signal *TriggerSignal) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.signals = append(e.signals, signal)
}

func (e *triggerEvaluator) Evaluate(_ context.Context, currentMode orchestrator.SurvivalMode) (*ModeTransitionAdvice, error) {
	e.mu.Lock()
	signals := make([]*TriggerSignal, len(e.signals))
	copy(signals, e.signals)
	e.signals = e.signals[:0]
	e.mu.Unlock()

	if len(signals) == 0 {
		return nil, nil
	}

	// 找最高严重度
	maxSeverity := -1
	for _, s := range signals {
		if s.Severity > maxSeverity {
			maxSeverity = s.Severity
		}
	}

	targetMode, ok := SeverityToMode[maxSeverity]
	if !ok {
		return nil, nil
	}

	// 如果目标模式严重度不高于当前模式，无需迁移
	currentSeverity := ModeSeverity[currentMode]
	if maxSeverity <= currentSeverity {
		return nil, nil
	}

	triggers := make([]TriggerSignal, len(signals))
	for i, s := range signals {
		triggers[i] = *s
	}

	return &ModeTransitionAdvice{
		TargetMode: targetMode,
		Triggers:   triggers,
		Confidence: 1.0,
	}, nil
}
