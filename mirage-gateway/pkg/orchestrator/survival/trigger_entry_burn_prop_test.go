package survival

import (
	"context"
	"testing"

	"pgregory.net/rapid"
)

// Property 4: Entry Burn 触发正确性
func TestProperty4_EntryBurnTriggerCorrectness(t *testing.T) {
	trigger := NewEntryBurnTrigger()

	rapid.Check(t, func(t *rapid.T) {
		burnCount := rapid.IntRange(0, 100).Draw(t, "burn_count")
		threshold := rapid.IntRange(0, 100).Draw(t, "threshold")

		signal := trigger.Evaluate(context.Background(), burnCount, threshold)

		if burnCount > threshold {
			if signal == nil {
				t.Fatalf("burnCount=%d > threshold=%d: expected signal, got nil", burnCount, threshold)
			}
			if signal.Source != TriggerSourceEntryBurn {
				t.Fatalf("expected source EntryBurn, got %s", signal.Source)
			}
		} else {
			if signal != nil {
				t.Fatalf("burnCount=%d <= threshold=%d: expected nil, got signal", burnCount, threshold)
			}
		}
	})
}
