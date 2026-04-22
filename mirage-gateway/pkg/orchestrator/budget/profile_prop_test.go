package budget

import (
	"strings"
	"testing"

	"pgregory.net/rapid"
)

// **Validates: Requirements 1.3, 1.4**
// Property 1: BudgetProfile 校验正确性
// For any BudgetProfile, when all numeric fields are in valid range, Validate() returns nil;
// when any field is out of range, Validate() returns ErrInvalidBudgetProfile containing the field name.

func TestProperty1_ValidProfilePassesValidation(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		bp := &BudgetProfile{
			ProfileID:             rapid.String().Draw(t, "profile_id"),
			SessionID:             rapid.String().Draw(t, "session_id"),
			LatencyBudgetMs:       rapid.Int64Range(1, 100000).Draw(t, "latency_budget_ms"),
			BandwidthBudgetRatio:  rapid.Float64Range(0.0, 1.0).Draw(t, "bandwidth_budget_ratio"),
			SwitchBudgetPerHour:   rapid.IntRange(0, 10000).Draw(t, "switch_budget_per_hour"),
			EntryBurnBudgetPerDay: rapid.IntRange(0, 10000).Draw(t, "entry_burn_budget_per_day"),
			GatewayLoadBudget:     rapid.Float64Range(0.0, 1.0).Draw(t, "gateway_load_budget"),
			HardenedAllowed:       rapid.Bool().Draw(t, "hardened_allowed"),
			EscapeAllowed:         rapid.Bool().Draw(t, "escape_allowed"),
			LastResortAllowed:     rapid.Bool().Draw(t, "last_resort_allowed"),
		}

		err := bp.Validate()
		if err != nil {
			t.Fatalf("valid profile should pass validation, got: %v", err)
		}
	})
}

func TestProperty1_InvalidLatencyBudgetMs(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		bp := validProfile(t)
		bp.LatencyBudgetMs = rapid.Int64Range(-10000, 0).Draw(t, "invalid_latency")

		err := bp.Validate()
		if err == nil {
			t.Fatal("expected validation error for latency_budget_ms <= 0")
		}
		invErr, ok := err.(*ErrInvalidBudgetProfile)
		if !ok {
			t.Fatalf("expected *ErrInvalidBudgetProfile, got %T", err)
		}
		if invErr.Field != "latency_budget_ms" {
			t.Fatalf("expected field latency_budget_ms, got %s", invErr.Field)
		}
		if !strings.Contains(invErr.Error(), "latency_budget_ms") {
			t.Fatalf("error message should contain field name, got: %s", invErr.Error())
		}
	})
}

func TestProperty1_InvalidBandwidthBudgetRatio(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		bp := validProfile(t)
		// Generate value outside [0.0, 1.0]
		if rapid.Bool().Draw(t, "below_or_above") {
			bp.BandwidthBudgetRatio = rapid.Float64Range(-100.0, -0.001).Draw(t, "invalid_bw")
		} else {
			bp.BandwidthBudgetRatio = rapid.Float64Range(1.001, 100.0).Draw(t, "invalid_bw")
		}

		err := bp.Validate()
		if err == nil {
			t.Fatal("expected validation error for bandwidth_budget_ratio out of [0.0, 1.0]")
		}
		invErr, ok := err.(*ErrInvalidBudgetProfile)
		if !ok {
			t.Fatalf("expected *ErrInvalidBudgetProfile, got %T", err)
		}
		if invErr.Field != "bandwidth_budget_ratio" {
			t.Fatalf("expected field bandwidth_budget_ratio, got %s", invErr.Field)
		}
	})
}

func TestProperty1_InvalidSwitchBudgetPerHour(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		bp := validProfile(t)
		bp.SwitchBudgetPerHour = rapid.IntRange(-10000, -1).Draw(t, "invalid_switch")

		err := bp.Validate()
		if err == nil {
			t.Fatal("expected validation error for switch_budget_per_hour < 0")
		}
		invErr, ok := err.(*ErrInvalidBudgetProfile)
		if !ok {
			t.Fatalf("expected *ErrInvalidBudgetProfile, got %T", err)
		}
		if invErr.Field != "switch_budget_per_hour" {
			t.Fatalf("expected field switch_budget_per_hour, got %s", invErr.Field)
		}
	})
}

func TestProperty1_InvalidEntryBurnBudgetPerDay(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		bp := validProfile(t)
		bp.EntryBurnBudgetPerDay = rapid.IntRange(-10000, -1).Draw(t, "invalid_entry_burn")

		err := bp.Validate()
		if err == nil {
			t.Fatal("expected validation error for entry_burn_budget_per_day < 0")
		}
		invErr, ok := err.(*ErrInvalidBudgetProfile)
		if !ok {
			t.Fatalf("expected *ErrInvalidBudgetProfile, got %T", err)
		}
		if invErr.Field != "entry_burn_budget_per_day" {
			t.Fatalf("expected field entry_burn_budget_per_day, got %s", invErr.Field)
		}
	})
}

func TestProperty1_InvalidGatewayLoadBudget(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		bp := validProfile(t)
		if rapid.Bool().Draw(t, "below_or_above") {
			bp.GatewayLoadBudget = rapid.Float64Range(-100.0, -0.001).Draw(t, "invalid_gw")
		} else {
			bp.GatewayLoadBudget = rapid.Float64Range(1.001, 100.0).Draw(t, "invalid_gw")
		}

		err := bp.Validate()
		if err == nil {
			t.Fatal("expected validation error for gateway_load_budget out of [0.0, 1.0]")
		}
		invErr, ok := err.(*ErrInvalidBudgetProfile)
		if !ok {
			t.Fatalf("expected *ErrInvalidBudgetProfile, got %T", err)
		}
		if invErr.Field != "gateway_load_budget" {
			t.Fatalf("expected field gateway_load_budget, got %s", invErr.Field)
		}
	})
}

// validProfile generates a BudgetProfile with all fields in valid range.
func validProfile(t *rapid.T) *BudgetProfile {
	return &BudgetProfile{
		ProfileID:             "test-profile",
		SessionID:             "",
		LatencyBudgetMs:       rapid.Int64Range(1, 100000).Draw(t, "latency"),
		BandwidthBudgetRatio:  rapid.Float64Range(0.0, 1.0).Draw(t, "bw_ratio"),
		SwitchBudgetPerHour:   rapid.IntRange(0, 10000).Draw(t, "switch"),
		EntryBurnBudgetPerDay: rapid.IntRange(0, 10000).Draw(t, "entry_burn"),
		GatewayLoadBudget:     rapid.Float64Range(0.0, 1.0).Draw(t, "gw_load"),
	}
}
