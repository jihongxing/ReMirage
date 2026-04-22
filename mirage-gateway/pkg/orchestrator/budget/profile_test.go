package budget

import (
	"testing"
)

func TestDefaultBudgetProfile(t *testing.T) {
	bp := DefaultBudgetProfile()

	if bp.ProfileID == "" {
		t.Error("ProfileID should not be empty")
	}
	if bp.SessionID != "" {
		t.Errorf("SessionID should be empty, got %q", bp.SessionID)
	}
	if bp.LatencyBudgetMs <= 0 {
		t.Errorf("LatencyBudgetMs should be > 0, got %d", bp.LatencyBudgetMs)
	}
	if bp.BandwidthBudgetRatio < 0.0 || bp.BandwidthBudgetRatio > 1.0 {
		t.Errorf("BandwidthBudgetRatio should be in [0.0, 1.0], got %f", bp.BandwidthBudgetRatio)
	}
	if bp.SwitchBudgetPerHour < 0 {
		t.Errorf("SwitchBudgetPerHour should be >= 0, got %d", bp.SwitchBudgetPerHour)
	}
	if bp.EntryBurnBudgetPerDay < 0 {
		t.Errorf("EntryBurnBudgetPerDay should be >= 0, got %d", bp.EntryBurnBudgetPerDay)
	}
	if bp.GatewayLoadBudget < 0.0 || bp.GatewayLoadBudget > 1.0 {
		t.Errorf("GatewayLoadBudget should be in [0.0, 1.0], got %f", bp.GatewayLoadBudget)
	}
	if bp.HardenedAllowed {
		t.Error("HardenedAllowed should be false")
	}
	if bp.EscapeAllowed {
		t.Error("EscapeAllowed should be false")
	}
	if bp.LastResortAllowed {
		t.Error("LastResortAllowed should be false")
	}

	// DefaultBudgetProfile must pass validation
	if err := bp.Validate(); err != nil {
		t.Errorf("DefaultBudgetProfile should pass validation, got %v", err)
	}
}

func TestValidate_ValidProfile(t *testing.T) {
	bp := DefaultBudgetProfile()
	if err := bp.Validate(); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

func TestValidate_LatencyBudgetMs(t *testing.T) {
	bp := DefaultBudgetProfile()

	bp.LatencyBudgetMs = 0
	assertValidationError(t, bp, "latency_budget_ms")

	bp.LatencyBudgetMs = -1
	assertValidationError(t, bp, "latency_budget_ms")

	bp.LatencyBudgetMs = 1
	if err := bp.Validate(); err != nil {
		t.Errorf("latency_budget_ms=1 should be valid, got %v", err)
	}
}

func TestValidate_BandwidthBudgetRatio(t *testing.T) {
	bp := DefaultBudgetProfile()

	// Boundary: 0.0 is valid
	bp.BandwidthBudgetRatio = 0.0
	if err := bp.Validate(); err != nil {
		t.Errorf("bandwidth_budget_ratio=0.0 should be valid, got %v", err)
	}

	// Boundary: 1.0 is valid
	bp.BandwidthBudgetRatio = 1.0
	if err := bp.Validate(); err != nil {
		t.Errorf("bandwidth_budget_ratio=1.0 should be valid, got %v", err)
	}

	// Out of range
	bp.BandwidthBudgetRatio = -0.01
	assertValidationError(t, bp, "bandwidth_budget_ratio")

	bp.BandwidthBudgetRatio = 1.01
	assertValidationError(t, bp, "bandwidth_budget_ratio")
}

func TestValidate_SwitchBudgetPerHour(t *testing.T) {
	bp := DefaultBudgetProfile()

	// Boundary: 0 is valid
	bp.SwitchBudgetPerHour = 0
	if err := bp.Validate(); err != nil {
		t.Errorf("switch_budget_per_hour=0 should be valid, got %v", err)
	}

	bp.SwitchBudgetPerHour = -1
	assertValidationError(t, bp, "switch_budget_per_hour")
}

func TestValidate_EntryBurnBudgetPerDay(t *testing.T) {
	bp := DefaultBudgetProfile()

	// Boundary: 0 is valid
	bp.EntryBurnBudgetPerDay = 0
	if err := bp.Validate(); err != nil {
		t.Errorf("entry_burn_budget_per_day=0 should be valid, got %v", err)
	}

	bp.EntryBurnBudgetPerDay = -1
	assertValidationError(t, bp, "entry_burn_budget_per_day")
}

func TestValidate_GatewayLoadBudget(t *testing.T) {
	bp := DefaultBudgetProfile()

	// Boundary: 0.0 is valid
	bp.GatewayLoadBudget = 0.0
	if err := bp.Validate(); err != nil {
		t.Errorf("gateway_load_budget=0.0 should be valid, got %v", err)
	}

	// Boundary: 1.0 is valid
	bp.GatewayLoadBudget = 1.0
	if err := bp.Validate(); err != nil {
		t.Errorf("gateway_load_budget=1.0 should be valid, got %v", err)
	}

	bp.GatewayLoadBudget = -0.01
	assertValidationError(t, bp, "gateway_load_budget")

	bp.GatewayLoadBudget = 1.01
	assertValidationError(t, bp, "gateway_load_budget")
}

func TestTableName(t *testing.T) {
	bp := BudgetProfile{}
	if bp.TableName() != "budget_profiles" {
		t.Errorf("expected budget_profiles, got %s", bp.TableName())
	}
}

// assertValidationError checks that Validate returns ErrInvalidBudgetProfile with the expected field.
func assertValidationError(t *testing.T, bp *BudgetProfile, expectedField string) {
	t.Helper()
	err := bp.Validate()
	if err == nil {
		t.Fatalf("expected validation error for field %s, got nil", expectedField)
	}
	invErr, ok := err.(*ErrInvalidBudgetProfile)
	if !ok {
		t.Fatalf("expected *ErrInvalidBudgetProfile, got %T", err)
	}
	if invErr.Field != expectedField {
		t.Errorf("expected field %s, got %s", expectedField, invErr.Field)
	}
}
