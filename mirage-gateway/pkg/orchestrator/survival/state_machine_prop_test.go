package survival

import (
	"errors"
	"testing"

	orchestrator "mirage-gateway/pkg/orchestrator"

	"pgregory.net/rapid"
)

func genSurvivalMode(t *rapid.T) orchestrator.SurvivalMode {
	return AllSurvivalModes[rapid.IntRange(0, len(AllSurvivalModes)-1).Draw(t, "mode")]
}

// Property 1: 状态机转换合法性
func TestProperty1_StateMachineTransitionValidity(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		from := genSurvivalMode(t)
		to := genSurvivalMode(t)

		err := ValidateTransition(from, to)

		// 自迁移必须拒绝
		if from == to {
			if err == nil {
				t.Fatalf("self-transition %s→%s should be rejected", from, to)
			}
			var invErr *ErrInvalidTransition
			if !errors.As(err, &invErr) {
				t.Fatalf("expected ErrInvalidTransition, got %T", err)
			}
			if invErr.From != from || invErr.To != to {
				t.Fatalf("error fields mismatch: got From=%s To=%s", invErr.From, invErr.To)
			}
			return
		}

		// 检查是否在合法路径中
		isValid := false
		if targets, ok := ValidTransitions[from]; ok {
			for _, tgt := range targets {
				if tgt == to {
					isValid = true
					break
				}
			}
		}

		if isValid {
			if err != nil {
				t.Fatalf("valid transition %s→%s returned error: %v", from, to, err)
			}
		} else {
			if err == nil {
				t.Fatalf("invalid transition %s→%s should be rejected", from, to)
			}
			var invErr *ErrInvalidTransition
			if !errors.As(err, &invErr) {
				t.Fatalf("expected ErrInvalidTransition, got %T", err)
			}
			if invErr.From != from || invErr.To != to {
				t.Fatalf("error fields mismatch: got From=%s To=%s", invErr.From, invErr.To)
			}
		}
	})
}
