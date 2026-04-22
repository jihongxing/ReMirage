package budget

import (
	"mirage-gateway/pkg/orchestrator"
	"testing"
)

func TestDefaultSLAPolicy_Standard(t *testing.T) {
	p := NewDefaultSLAPolicy()
	policy := p.GetPolicy(orchestrator.ServiceClassStandard)

	if policy.HardenedAllowed != false {
		t.Errorf("Standard hardened_allowed: got %v, want false", policy.HardenedAllowed)
	}
	if policy.EscapeAllowed != false {
		t.Errorf("Standard escape_allowed: got %v, want false", policy.EscapeAllowed)
	}
	if policy.LastResortAllowed != false {
		t.Errorf("Standard last_resort_allowed: got %v, want false", policy.LastResortAllowed)
	}
	if policy.MaxSwitchPerHour != 5 {
		t.Errorf("Standard max_switch_per_hour: got %d, want 5", policy.MaxSwitchPerHour)
	}
	if policy.MaxEntryBurnPerDay != 2 {
		t.Errorf("Standard max_entry_burn_per_day: got %d, want 2", policy.MaxEntryBurnPerDay)
	}
}

func TestDefaultSLAPolicy_Platinum(t *testing.T) {
	p := NewDefaultSLAPolicy()
	policy := p.GetPolicy(orchestrator.ServiceClassPlatinum)

	if policy.HardenedAllowed != true {
		t.Errorf("Platinum hardened_allowed: got %v, want true", policy.HardenedAllowed)
	}
	if policy.EscapeAllowed != false {
		t.Errorf("Platinum escape_allowed: got %v, want false", policy.EscapeAllowed)
	}
	if policy.LastResortAllowed != false {
		t.Errorf("Platinum last_resort_allowed: got %v, want false", policy.LastResortAllowed)
	}
	if policy.MaxSwitchPerHour != 15 {
		t.Errorf("Platinum max_switch_per_hour: got %d, want 15", policy.MaxSwitchPerHour)
	}
	if policy.MaxEntryBurnPerDay != 5 {
		t.Errorf("Platinum max_entry_burn_per_day: got %d, want 5", policy.MaxEntryBurnPerDay)
	}
}

func TestDefaultSLAPolicy_Diamond(t *testing.T) {
	p := NewDefaultSLAPolicy()
	policy := p.GetPolicy(orchestrator.ServiceClassDiamond)

	if policy.HardenedAllowed != true {
		t.Errorf("Diamond hardened_allowed: got %v, want true", policy.HardenedAllowed)
	}
	if policy.EscapeAllowed != true {
		t.Errorf("Diamond escape_allowed: got %v, want true", policy.EscapeAllowed)
	}
	if policy.LastResortAllowed != true {
		t.Errorf("Diamond last_resort_allowed: got %v, want true", policy.LastResortAllowed)
	}
	if policy.MaxSwitchPerHour != 30 {
		t.Errorf("Diamond max_switch_per_hour: got %d, want 30", policy.MaxSwitchPerHour)
	}
	if policy.MaxEntryBurnPerDay != 10 {
		t.Errorf("Diamond max_entry_burn_per_day: got %d, want 10", policy.MaxEntryBurnPerDay)
	}
}

func TestDefaultSLAPolicy_UnknownFallsBackToStandard(t *testing.T) {
	p := NewDefaultSLAPolicy()
	unknown := p.GetPolicy(orchestrator.ServiceClass("Unknown"))
	standard := p.GetPolicy(orchestrator.ServiceClassStandard)

	if unknown.HardenedAllowed != standard.HardenedAllowed ||
		unknown.EscapeAllowed != standard.EscapeAllowed ||
		unknown.LastResortAllowed != standard.LastResortAllowed ||
		unknown.MaxSwitchPerHour != standard.MaxSwitchPerHour ||
		unknown.MaxEntryBurnPerDay != standard.MaxEntryBurnPerDay {
		t.Errorf("Unknown ServiceClass should return Standard policy, got %+v", unknown)
	}
}
