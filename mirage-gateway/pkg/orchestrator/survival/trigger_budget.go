package survival

import (
	"context"
	"time"

	orchestrator "mirage-gateway/pkg/orchestrator"
	"mirage-gateway/pkg/orchestrator/budget"
)

type budgetTrigger struct{}

// NewBudgetTrigger 创建 BudgetTrigger
func NewBudgetTrigger() BudgetTriggerIface {
	return &budgetTrigger{}
}

func (t *budgetTrigger) Evaluate(_ context.Context, verdict budget.BudgetVerdict) *TriggerSignal {
	if verdict != budget.VerdictDenyAndSuspend {
		return nil
	}
	return &TriggerSignal{
		Source:    TriggerSourceBudget,
		Reason:    "budget verdict deny_and_suspend → Degraded",
		Severity:  ModeSeverity[orchestrator.SurvivalModeDegraded],
		Timestamp: time.Now(),
		Metadata: map[string]interface{}{
			"verdict":     string(verdict),
			"target_mode": string(orchestrator.SurvivalModeDegraded),
		},
	}
}
