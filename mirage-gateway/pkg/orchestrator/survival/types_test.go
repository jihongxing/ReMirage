package survival

import (
	"testing"

	orchestrator "mirage-gateway/pkg/orchestrator"
)

func TestValidTransitions_KeyCount(t *testing.T) {
	if len(ValidTransitions) != 6 {
		t.Errorf("expected 6 keys, got %d", len(ValidTransitions))
	}
}

func TestValidTransitions_TotalPaths(t *testing.T) {
	total := 0
	for _, targets := range ValidTransitions {
		total += len(targets)
	}
	if total != 16 {
		t.Errorf("expected 16 total paths, got %d", total)
	}
}

func TestModeSeverity_Ordering(t *testing.T) {
	ordered := []orchestrator.SurvivalMode{
		orchestrator.SurvivalModeNormal,
		orchestrator.SurvivalModeLowNoise,
		orchestrator.SurvivalModeHardened,
		orchestrator.SurvivalModeDegraded,
		orchestrator.SurvivalModeEscape,
		orchestrator.SurvivalModeLastResort,
	}
	for i := 1; i < len(ordered); i++ {
		if ModeSeverity[ordered[i]] <= ModeSeverity[ordered[i-1]] {
			t.Errorf("expected %s severity > %s severity", ordered[i], ordered[i-1])
		}
	}
}

func TestEnumStringValues(t *testing.T) {
	if string(AggressivenessConservative) != "Conservative" {
		t.Error("AggressivenessConservative mismatch")
	}
	if string(AggressivenessModerate) != "Moderate" {
		t.Error("AggressivenessModerate mismatch")
	}
	if string(AggressivenessAggressive) != "Aggressive" {
		t.Error("AggressivenessAggressive mismatch")
	}
	if string(AdmissionOpen) != "Open" {
		t.Error("AdmissionOpen mismatch")
	}
	if string(AdmissionRestrictNew) != "RestrictNew" {
		t.Error("AdmissionRestrictNew mismatch")
	}
	if string(AdmissionHighPriorityOnly) != "HighPriorityOnly" {
		t.Error("AdmissionHighPriorityOnly mismatch")
	}
	if string(AdmissionClosed) != "Closed" {
		t.Error("AdmissionClosed mismatch")
	}
	if string(TriggerSourceLinkHealth) != "LinkHealth" {
		t.Error("TriggerSourceLinkHealth mismatch")
	}
	if string(TriggerSourceEntryBurn) != "EntryBurn" {
		t.Error("TriggerSourceEntryBurn mismatch")
	}
	if string(TriggerSourceBudget) != "Budget" {
		t.Error("TriggerSourceBudget mismatch")
	}
	if string(TriggerSourcePolicy) != "Policy" {
		t.Error("TriggerSourcePolicy mismatch")
	}
}
