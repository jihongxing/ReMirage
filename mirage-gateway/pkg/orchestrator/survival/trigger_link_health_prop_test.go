package survival

import (
	"context"
	"testing"

	orchestrator "mirage-gateway/pkg/orchestrator"

	"pgregory.net/rapid"
)

// Property 3: Link Health 触发阈值映射
func TestProperty3_LinkHealthTriggerThresholds(t *testing.T) {
	trigger := NewLinkHealthTrigger()

	rapid.Check(t, func(t *rapid.T) {
		count := rapid.IntRange(1, 10).Draw(t, "link_count")
		links := make([]*orchestrator.LinkState, count)
		var sum float64
		for i := 0; i < count; i++ {
			score := rapid.Float64Range(0, 100).Draw(t, "health_score")
			links[i] = &orchestrator.LinkState{HealthScore: score}
			sum += score
		}
		avg := sum / float64(count)

		signal := trigger.Evaluate(context.Background(), links)

		switch {
		case avg < 10:
			if signal == nil {
				t.Fatalf("avg=%.2f: expected Escape signal, got nil", avg)
			}
			if signal.Severity != ModeSeverity[orchestrator.SurvivalModeEscape] {
				t.Fatalf("avg=%.2f: expected severity %d, got %d", avg, ModeSeverity[orchestrator.SurvivalModeEscape], signal.Severity)
			}
		case avg < 30:
			if signal == nil {
				t.Fatalf("avg=%.2f: expected Degraded signal, got nil", avg)
			}
			if signal.Severity != ModeSeverity[orchestrator.SurvivalModeDegraded] {
				t.Fatalf("avg=%.2f: expected severity %d, got %d", avg, ModeSeverity[orchestrator.SurvivalModeDegraded], signal.Severity)
			}
		case avg < 60:
			if signal == nil {
				t.Fatalf("avg=%.2f: expected Hardened signal, got nil", avg)
			}
			if signal.Severity != ModeSeverity[orchestrator.SurvivalModeHardened] {
				t.Fatalf("avg=%.2f: expected severity %d, got %d", avg, ModeSeverity[orchestrator.SurvivalModeHardened], signal.Severity)
			}
		default:
			if signal != nil {
				t.Fatalf("avg=%.2f: expected nil signal, got severity %d", avg, signal.Severity)
			}
		}

		if signal != nil && signal.Source != TriggerSourceLinkHealth {
			t.Fatalf("expected source LinkHealth, got %s", signal.Source)
		}
	})
}
