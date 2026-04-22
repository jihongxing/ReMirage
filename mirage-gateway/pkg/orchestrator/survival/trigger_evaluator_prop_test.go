package survival

import (
	"context"
	"testing"
	"time"

	orchestrator "mirage-gateway/pkg/orchestrator"

	"pgregory.net/rapid"
)

// Property 6: 触发因素合并取最高严重度
func TestProperty6_TriggerEvaluatorMergesHighestSeverity(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		evaluator := NewTriggerEvaluator()

		count := rapid.IntRange(1, 10).Draw(t, "signal_count")
		maxSeverity := 0
		sources := make([]TriggerSource, count)

		for i := 0; i < count; i++ {
			severity := rapid.IntRange(1, 5).Draw(t, "severity")
			source := AllTriggerSources[rapid.IntRange(0, len(AllTriggerSources)-1).Draw(t, "source")]
			sources[i] = source
			if severity > maxSeverity {
				maxSeverity = severity
			}
			evaluator.SubmitSignal(&TriggerSignal{
				Source:    source,
				Reason:    "test",
				Severity:  severity,
				Timestamp: time.Now(),
			})
		}

		// 使用 Normal 作为当前模式，确保所有信号都能触发迁移
		advice, err := evaluator.Evaluate(context.Background(), orchestrator.SurvivalModeNormal)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if advice == nil {
			t.Fatal("expected advice, got nil")
		}

		expectedMode := SeverityToMode[maxSeverity]
		if advice.TargetMode != expectedMode {
			t.Fatalf("expected target_mode %s (severity %d), got %s", expectedMode, maxSeverity, advice.TargetMode)
		}

		if len(advice.Triggers) != count {
			t.Fatalf("expected %d triggers, got %d", count, len(advice.Triggers))
		}

		// 验证所有输入信号的 source 都在 triggers 中
		for i, src := range sources {
			if advice.Triggers[i].Source != src {
				t.Fatalf("trigger[%d] source mismatch: expected %s, got %s", i, src, advice.Triggers[i].Source)
			}
		}
	})
}
