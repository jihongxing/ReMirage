package survival

import (
	"testing"

	orchestrator "mirage-gateway/pkg/orchestrator"

	"pgregory.net/rapid"
)

// Property 2: ModePolicy 绑定完整性与正确性
func TestProperty2_ModePolicyBindingCompleteness(t *testing.T) {
	expectedAggr := map[orchestrator.SurvivalMode]SwitchAggressiveness{
		orchestrator.SurvivalModeNormal:     AggressivenessConservative,
		orchestrator.SurvivalModeLowNoise:   AggressivenessConservative,
		orchestrator.SurvivalModeHardened:   AggressivenessModerate,
		orchestrator.SurvivalModeDegraded:   AggressivenessConservative,
		orchestrator.SurvivalModeEscape:     AggressivenessAggressive,
		orchestrator.SurvivalModeLastResort: AggressivenessAggressive,
	}
	expectedAdm := map[orchestrator.SurvivalMode]SessionAdmissionPolicy{
		orchestrator.SurvivalModeNormal:     AdmissionOpen,
		orchestrator.SurvivalModeLowNoise:   AdmissionOpen,
		orchestrator.SurvivalModeHardened:   AdmissionOpen,
		orchestrator.SurvivalModeDegraded:   AdmissionRestrictNew,
		orchestrator.SurvivalModeEscape:     AdmissionHighPriorityOnly,
		orchestrator.SurvivalModeLastResort: AdmissionClosed,
	}

	rapid.Check(t, func(t *rapid.T) {
		mode := genSurvivalMode(t)
		policy := DefaultModePolicies[mode]

		if policy == nil {
			t.Fatalf("no policy for mode %s", mode)
		}
		if policy.TransportPolicy == "" {
			t.Fatal("transport_policy is empty")
		}
		if policy.PersonaPolicy == "" {
			t.Fatal("persona_policy is empty")
		}
		if policy.BudgetPolicy == "" {
			t.Fatal("budget_policy is empty")
		}
		if policy.SwitchAggressiveness == "" {
			t.Fatal("switch_aggressiveness is empty")
		}
		if policy.SessionAdmissionPolicy == "" {
			t.Fatal("session_admission_policy is empty")
		}
		if policy.SwitchAggressiveness != expectedAggr[mode] {
			t.Fatalf("mode %s: expected aggressiveness %s, got %s", mode, expectedAggr[mode], policy.SwitchAggressiveness)
		}
		if policy.SessionAdmissionPolicy != expectedAdm[mode] {
			t.Fatalf("mode %s: expected admission %s, got %s", mode, expectedAdm[mode], policy.SessionAdmissionPolicy)
		}
	})
}
