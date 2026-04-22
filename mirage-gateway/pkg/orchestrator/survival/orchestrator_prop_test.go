package survival

import (
	"context"
	"testing"
	"time"

	orchestrator "mirage-gateway/pkg/orchestrator"
	"mirage-gateway/pkg/orchestrator/commit"

	"pgregory.net/rapid"
)

// mockCommitEngine 用于测试的 mock
type mockCommitEngine struct {
	txCounter int
}

func (m *mockCommitEngine) BeginTransaction(_ context.Context, req *commit.BeginTxRequest) (*commit.CommitTransaction, error) {
	m.txCounter++
	return &commit.CommitTransaction{
		TxID:    "tx-" + string(rune('0'+m.txCounter)),
		TxType:  req.TxType,
		TxPhase: commit.TxPhasePreparing,
	}, nil
}
func (m *mockCommitEngine) ExecuteTransaction(_ context.Context, _ string) error { return nil }
func (m *mockCommitEngine) RollbackTransaction(_ context.Context, _ string, _ string) error {
	return nil
}
func (m *mockCommitEngine) GetTransaction(_ context.Context, _ string) (*commit.CommitTransaction, error) {
	return nil, nil
}
func (m *mockCommitEngine) ListTransactions(_ context.Context, _ *commit.TxFilter) ([]*commit.CommitTransaction, error) {
	return nil, nil
}
func (m *mockCommitEngine) GetActiveTransactions(_ context.Context) ([]*commit.CommitTransaction, error) {
	return nil, nil
}
func (m *mockCommitEngine) RecoverOnStartup(_ context.Context) error { return nil }

func newTestOrchestrator() *survivalOrchestrator {
	return &survivalOrchestrator{
		currentMode:   orchestrator.SurvivalModeNormal,
		currentPolicy: DefaultModePolicies[orchestrator.SurvivalModeNormal],
		enteredAt:     time.Now().Add(-10 * time.Minute),
		lastUpgradeAt: time.Now().Add(-10 * time.Minute),
		evaluator:     NewTriggerEvaluator(),
		constraint:    NewTransitionConstraint(DefaultConstraintConfig),
		admission:     NewSessionAdmissionController(AdmissionOpen),
		commitEngine:  &mockCommitEngine{},
		history:       make([]*TransitionRecord, 0),
	}
}

// Property 12: 迁移历史记录完整性
func TestProperty12_TransitionRecordCompleteness(t *testing.T) {
	// 从 Normal 出发的合法目标
	validTargets := ValidTransitions[orchestrator.SurvivalModeNormal]

	rapid.Check(t, func(t *rapid.T) {
		orch := newTestOrchestrator()
		target := validTargets[rapid.IntRange(0, len(validTargets)-1).Draw(t, "target")]

		triggers := []TriggerSignal{{
			Source:   TriggerSourcePolicy,
			Reason:   "test trigger",
			Severity: ModeSeverity[target],
		}}

		err := orch.RequestTransition(context.Background(), target, triggers)
		if err != nil {
			t.Fatalf("transition failed: %v", err)
		}

		history := orch.GetTransitionHistory(1)
		if len(history) != 1 {
			t.Fatalf("expected 1 record, got %d", len(history))
		}

		rec := history[0]
		if rec.FromMode != orchestrator.SurvivalModeNormal {
			t.Fatalf("expected from Normal, got %s", rec.FromMode)
		}
		if rec.ToMode != target {
			t.Fatalf("expected to %s, got %s", target, rec.ToMode)
		}
		if rec.TxID == "" {
			t.Fatal("TxID is empty")
		}
		if len(rec.Triggers) == 0 {
			t.Fatal("Triggers is empty")
		}
		if rec.Triggers[0].Source == "" {
			t.Fatal("trigger source is empty")
		}
		if rec.Triggers[0].Reason == "" {
			t.Fatal("trigger reason is empty")
		}
		if rec.Timestamp.IsZero() {
			t.Fatal("timestamp is zero")
		}
	})
}
