package survival

import (
	"context"
	"testing"
	"time"

	orchestrator "mirage-gateway/pkg/orchestrator"
)

// mockPersonaEngine
type mockPersonaEngine struct {
	lastPolicy string
}

func (m *mockPersonaEngine) ApplyPolicy(policyName string) error {
	m.lastPolicy = policyName
	return nil
}

// mockGSwitchAdapter
type mockGSwitchAdapterForTest struct {
	escapeTriggered bool
	escapeReason    string
}

func (m *mockGSwitchAdapterForTest) TriggerEscape(reason string) error {
	m.escapeTriggered = true
	m.escapeReason = reason
	return nil
}

func newIntegrationOrchestrator() (*survivalOrchestrator, *mockPersonaEngine, *mockGSwitchAdapterForTest) {
	persona := &mockPersonaEngine{}
	gswitch := &mockGSwitchAdapterForTest{}

	orch := &survivalOrchestrator{
		currentMode:    orchestrator.SurvivalModeNormal,
		currentPolicy:  DefaultModePolicies[orchestrator.SurvivalModeNormal],
		enteredAt:      time.Now().Add(-10 * time.Minute),
		lastUpgradeAt:  time.Now().Add(-10 * time.Minute),
		evaluator:      NewTriggerEvaluator(),
		constraint:     NewTransitionConstraint(DefaultConstraintConfig),
		admission:      NewSessionAdmissionController(AdmissionOpen),
		commitEngine:   &mockCommitEngine{},
		persona:        persona,
		gswitchAdapter: gswitch,
		history:        make([]*TransitionRecord, 0),
	}
	return orch, persona, gswitch
}

func TestIntegration_FullModeSwitch(t *testing.T) {
	orch, persona, _ := newIntegrationOrchestrator()
	ctx := context.Background()

	triggers := []TriggerSignal{{
		Source:   TriggerSourcePolicy,
		Reason:   "test",
		Severity: ModeSeverity[orchestrator.SurvivalModeHardened],
	}}

	err := orch.RequestTransition(ctx, orchestrator.SurvivalModeHardened, triggers)
	if err != nil {
		t.Fatalf("transition failed: %v", err)
	}

	if orch.GetCurrentMode() != orchestrator.SurvivalModeHardened {
		t.Fatalf("expected Hardened, got %s", orch.GetCurrentMode())
	}

	if persona.lastPolicy != "hardened" {
		t.Fatalf("expected persona policy 'hardened', got %q", persona.lastPolicy)
	}

	policy := orch.GetCurrentPolicy()
	if policy.SwitchAggressiveness != AggressivenessModerate {
		t.Fatalf("expected Moderate aggressiveness, got %s", policy.SwitchAggressiveness)
	}
}

func TestIntegration_EscapeTriggersGSwitch(t *testing.T) {
	orch, _, gswitch := newIntegrationOrchestrator()
	ctx := context.Background()

	// Normal → Hardened first
	triggers := []TriggerSignal{{Source: TriggerSourcePolicy, Reason: "test", Severity: 2}}
	_ = orch.RequestTransition(ctx, orchestrator.SurvivalModeHardened, triggers)

	// Wait for dwell time
	orch.enteredAt = time.Now().Add(-5 * time.Minute)
	orch.lastUpgradeAt = time.Now().Add(-5 * time.Minute)

	// Hardened → Escape
	triggers = []TriggerSignal{{Source: TriggerSourcePolicy, Reason: "escape test", Severity: 4}}
	err := orch.RequestTransition(ctx, orchestrator.SurvivalModeEscape, triggers)
	if err != nil {
		t.Fatalf("transition to Escape failed: %v", err)
	}

	if !gswitch.escapeTriggered {
		t.Fatal("expected G-Switch TriggerEscape to be called")
	}
}

func TestIntegration_RecoverOnStartup(t *testing.T) {
	orch, _, _ := newIntegrationOrchestrator()
	ctx := context.Background()

	err := orch.RecoverOnStartup(ctx)
	if err != nil {
		t.Fatalf("recovery failed: %v", err)
	}
}

func TestIntegration_TransitionHistory(t *testing.T) {
	orch, _, _ := newIntegrationOrchestrator()
	ctx := context.Background()

	// 执行两次迁移
	triggers := []TriggerSignal{{Source: TriggerSourcePolicy, Reason: "t1", Severity: 2}}
	_ = orch.RequestTransition(ctx, orchestrator.SurvivalModeHardened, triggers)

	orch.enteredAt = time.Now().Add(-5 * time.Minute)
	orch.lastUpgradeAt = time.Now().Add(-5 * time.Minute)

	triggers = []TriggerSignal{{Source: TriggerSourcePolicy, Reason: "t2", Severity: 0}}
	_ = orch.RequestTransition(ctx, orchestrator.SurvivalModeNormal, triggers)

	history := orch.GetTransitionHistory(10)
	if len(history) != 2 {
		t.Fatalf("expected 2 records, got %d", len(history))
	}
}

func TestIntegration_AdmissionPolicyUpdatedOnSwitch(t *testing.T) {
	orch, _, _ := newIntegrationOrchestrator()
	ctx := context.Background()

	// Normal → Degraded (RestrictNew)
	triggers := []TriggerSignal{{Source: TriggerSourcePolicy, Reason: "test", Severity: 3}}
	_ = orch.RequestTransition(ctx, orchestrator.SurvivalModeDegraded, triggers)

	// Standard 应被拒绝
	err := orch.CheckAdmission(orchestrator.ServiceClassStandard)
	if err == nil {
		t.Fatal("expected admission denied for Standard in Degraded mode")
	}

	// Diamond 应被允许
	err = orch.CheckAdmission(orchestrator.ServiceClassDiamond)
	if err != nil {
		t.Fatalf("expected admission allowed for Diamond, got: %v", err)
	}
}
