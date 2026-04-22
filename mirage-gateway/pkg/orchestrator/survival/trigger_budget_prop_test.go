package survival

import (
	"context"
	"testing"

	orchestrator "mirage-gateway/pkg/orchestrator"
	"mirage-gateway/pkg/orchestrator/budget"

	"pgregory.net/rapid"
)

var allBudgetVerdicts = []budget.BudgetVerdict{
	budget.VerdictAllow,
	budget.VerdictAllowDegraded,
	budget.VerdictAllowWithCharge,
	budget.VerdictDenyAndHold,
	budget.VerdictDenyAndSuspend,
}

// Property 5: Budget 触发正确性
func TestProperty5_BudgetTriggerCorrectness(t *testing.T) {
	trigger := NewBudgetTrigger()

	rapid.Check(t, func(t *rapid.T) {
		verdict := allBudgetVerdicts[rapid.IntRange(0, len(allBudgetVerdicts)-1).Draw(t, "verdict_idx")]

		signal := trigger.Evaluate(context.Background(), verdict)

		if verdict == budget.VerdictDenyAndSuspend {
			if signal == nil {
				t.Fatalf("verdict=%s: expected signal, got nil", verdict)
			}
			if signal.Source != TriggerSourceBudget {
				t.Fatalf("expected source Budget, got %s", signal.Source)
			}
			if signal.Severity != ModeSeverity[orchestrator.SurvivalModeDegraded] {
				t.Fatalf("expected severity %d (Degraded), got %d", ModeSeverity[orchestrator.SurvivalModeDegraded], signal.Severity)
			}
		} else {
			if signal != nil {
				t.Fatalf("verdict=%s: expected nil, got signal", verdict)
			}
		}
	})
}
