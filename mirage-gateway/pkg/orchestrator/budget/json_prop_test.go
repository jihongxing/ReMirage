package budget

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"pgregory.net/rapid"
)

// genBudgetProfile generates a random valid BudgetProfile with a fixed time to avoid precision issues.
func genBudgetProfile(t *rapid.T, now time.Time) *BudgetProfile {
	return &BudgetProfile{
		ProfileID:             rapid.StringMatching(`[a-zA-Z0-9\-]{1,64}`).Draw(t, "profile_id"),
		SessionID:             rapid.StringMatching(`[a-zA-Z0-9\-]{0,64}`).Draw(t, "session_id"),
		LatencyBudgetMs:       rapid.Int64Range(1, math.MaxInt64).Draw(t, "latency_budget_ms"),
		BandwidthBudgetRatio:  rapid.Float64Range(0.0, 1.0).Draw(t, "bandwidth_budget_ratio"),
		SwitchBudgetPerHour:   rapid.IntRange(0, 10000).Draw(t, "switch_budget_per_hour"),
		EntryBurnBudgetPerDay: rapid.IntRange(0, 10000).Draw(t, "entry_burn_budget_per_day"),
		GatewayLoadBudget:     rapid.Float64Range(0.0, 1.0).Draw(t, "gateway_load_budget"),
		HardenedAllowed:       rapid.Bool().Draw(t, "hardened_allowed"),
		EscapeAllowed:         rapid.Bool().Draw(t, "escape_allowed"),
		LastResortAllowed:     rapid.Bool().Draw(t, "last_resort_allowed"),
		CreatedAt:             now,
		UpdatedAt:             now,
	}
}

// genCostEstimate generates a random CostEstimate where TotalCost = sum of other 5 fields.
func genCostEstimate(t *rapid.T) *CostEstimate {
	bw := rapid.Float64Range(0, 1000).Draw(t, "bandwidth_cost")
	lat := rapid.Float64Range(0, 1000).Draw(t, "latency_cost")
	sw := rapid.Float64Range(0, 1000).Draw(t, "switch_cost")
	eb := rapid.Float64Range(0, 1000).Draw(t, "entry_burn_cost")
	gl := rapid.Float64Range(0, 1000).Draw(t, "gateway_load_cost")
	return &CostEstimate{
		BandwidthCost:   bw,
		LatencyCost:     lat,
		SwitchCost:      sw,
		EntryBurnCost:   eb,
		GatewayLoadCost: gl,
		TotalCost:       bw + lat + sw + eb + gl,
	}
}

var allVerdicts = []BudgetVerdict{
	VerdictAllow, VerdictAllowDegraded, VerdictAllowWithCharge,
	VerdictDenyAndHold, VerdictDenyAndSuspend,
}

// genBudgetDecision generates a random BudgetDecision.
func genBudgetDecision(t *rapid.T, now time.Time) *BudgetDecision {
	return &BudgetDecision{
		Verdict:         allVerdicts[rapid.IntRange(0, len(allVerdicts)-1).Draw(t, "verdict_idx")],
		CostEstimate:    genCostEstimate(t),
		RemainingBudget: genBudgetProfile(t, now),
		DenyReason:      rapid.StringMatching(`[a-zA-Z0-9 _\-]{0,100}`).Draw(t, "deny_reason"),
	}
}

// ---------- Property 8: JSON round-trip ----------

// TestProperty8_BudgetProfile_JSONRoundTrip
// Feature: v2-budget-engine, Property 8: JSON round-trip
// **Validates: Requirements 9.1, 9.4**
// For any valid BudgetProfile, JSON marshal → unmarshal produces an equivalent object.
func TestProperty8_BudgetProfile_JSONRoundTrip(t *testing.T) {
	now := time.Now().Truncate(time.Second).UTC()

	rapid.Check(t, func(t *rapid.T) {
		original := genBudgetProfile(t, now)

		data, err := json.Marshal(original)
		if err != nil {
			t.Fatalf("marshal failed: %v", err)
		}

		var decoded BudgetProfile
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}

		// Compare all fields
		if original.ProfileID != decoded.ProfileID {
			t.Fatalf("ProfileID mismatch: %q vs %q", original.ProfileID, decoded.ProfileID)
		}
		if original.SessionID != decoded.SessionID {
			t.Fatalf("SessionID mismatch: %q vs %q", original.SessionID, decoded.SessionID)
		}
		if original.LatencyBudgetMs != decoded.LatencyBudgetMs {
			t.Fatalf("LatencyBudgetMs mismatch: %d vs %d", original.LatencyBudgetMs, decoded.LatencyBudgetMs)
		}
		if original.BandwidthBudgetRatio != decoded.BandwidthBudgetRatio {
			t.Fatalf("BandwidthBudgetRatio mismatch: %f vs %f", original.BandwidthBudgetRatio, decoded.BandwidthBudgetRatio)
		}
		if original.SwitchBudgetPerHour != decoded.SwitchBudgetPerHour {
			t.Fatalf("SwitchBudgetPerHour mismatch: %d vs %d", original.SwitchBudgetPerHour, decoded.SwitchBudgetPerHour)
		}
		if original.EntryBurnBudgetPerDay != decoded.EntryBurnBudgetPerDay {
			t.Fatalf("EntryBurnBudgetPerDay mismatch: %d vs %d", original.EntryBurnBudgetPerDay, decoded.EntryBurnBudgetPerDay)
		}
		if original.GatewayLoadBudget != decoded.GatewayLoadBudget {
			t.Fatalf("GatewayLoadBudget mismatch: %f vs %f", original.GatewayLoadBudget, decoded.GatewayLoadBudget)
		}
		if original.HardenedAllowed != decoded.HardenedAllowed {
			t.Fatalf("HardenedAllowed mismatch: %v vs %v", original.HardenedAllowed, decoded.HardenedAllowed)
		}
		if original.EscapeAllowed != decoded.EscapeAllowed {
			t.Fatalf("EscapeAllowed mismatch: %v vs %v", original.EscapeAllowed, decoded.EscapeAllowed)
		}
		if original.LastResortAllowed != decoded.LastResortAllowed {
			t.Fatalf("LastResortAllowed mismatch: %v vs %v", original.LastResortAllowed, decoded.LastResortAllowed)
		}
		if !original.CreatedAt.Equal(decoded.CreatedAt) {
			t.Fatalf("CreatedAt mismatch: %v vs %v", original.CreatedAt, decoded.CreatedAt)
		}
		if !original.UpdatedAt.Equal(decoded.UpdatedAt) {
			t.Fatalf("UpdatedAt mismatch: %v vs %v", original.UpdatedAt, decoded.UpdatedAt)
		}
	})
}

// TestProperty8_CostEstimate_JSONRoundTrip
// Feature: v2-budget-engine, Property 8: JSON round-trip
// **Validates: Requirements 9.3**
// For any valid CostEstimate, JSON marshal → unmarshal produces an equivalent object.
func TestProperty8_CostEstimate_JSONRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		original := genCostEstimate(t)

		data, err := json.Marshal(original)
		if err != nil {
			t.Fatalf("marshal failed: %v", err)
		}

		var decoded CostEstimate
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}

		if original.BandwidthCost != decoded.BandwidthCost {
			t.Fatalf("BandwidthCost mismatch: %f vs %f", original.BandwidthCost, decoded.BandwidthCost)
		}
		if original.LatencyCost != decoded.LatencyCost {
			t.Fatalf("LatencyCost mismatch: %f vs %f", original.LatencyCost, decoded.LatencyCost)
		}
		if original.SwitchCost != decoded.SwitchCost {
			t.Fatalf("SwitchCost mismatch: %f vs %f", original.SwitchCost, decoded.SwitchCost)
		}
		if original.EntryBurnCost != decoded.EntryBurnCost {
			t.Fatalf("EntryBurnCost mismatch: %f vs %f", original.EntryBurnCost, decoded.EntryBurnCost)
		}
		if original.GatewayLoadCost != decoded.GatewayLoadCost {
			t.Fatalf("GatewayLoadCost mismatch: %f vs %f", original.GatewayLoadCost, decoded.GatewayLoadCost)
		}
		if original.TotalCost != decoded.TotalCost {
			t.Fatalf("TotalCost mismatch: %f vs %f", original.TotalCost, decoded.TotalCost)
		}
	})
}

// TestProperty8_BudgetDecision_JSONRoundTrip
// Feature: v2-budget-engine, Property 8: JSON round-trip
// **Validates: Requirements 9.2**
// For any valid BudgetDecision, JSON marshal → unmarshal produces an equivalent object.
func TestProperty8_BudgetDecision_JSONRoundTrip(t *testing.T) {
	now := time.Now().Truncate(time.Second).UTC()

	rapid.Check(t, func(t *rapid.T) {
		original := genBudgetDecision(t, now)

		data, err := json.Marshal(original)
		if err != nil {
			t.Fatalf("marshal failed: %v", err)
		}

		var decoded BudgetDecision
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}

		if original.Verdict != decoded.Verdict {
			t.Fatalf("Verdict mismatch: %q vs %q", original.Verdict, decoded.Verdict)
		}
		if original.DenyReason != decoded.DenyReason {
			t.Fatalf("DenyReason mismatch: %q vs %q", original.DenyReason, decoded.DenyReason)
		}

		// CostEstimate
		if original.CostEstimate.TotalCost != decoded.CostEstimate.TotalCost {
			t.Fatalf("CostEstimate.TotalCost mismatch: %f vs %f", original.CostEstimate.TotalCost, decoded.CostEstimate.TotalCost)
		}
		if original.CostEstimate.BandwidthCost != decoded.CostEstimate.BandwidthCost {
			t.Fatalf("CostEstimate.BandwidthCost mismatch")
		}
		if original.CostEstimate.LatencyCost != decoded.CostEstimate.LatencyCost {
			t.Fatalf("CostEstimate.LatencyCost mismatch")
		}
		if original.CostEstimate.SwitchCost != decoded.CostEstimate.SwitchCost {
			t.Fatalf("CostEstimate.SwitchCost mismatch")
		}
		if original.CostEstimate.EntryBurnCost != decoded.CostEstimate.EntryBurnCost {
			t.Fatalf("CostEstimate.EntryBurnCost mismatch")
		}
		if original.CostEstimate.GatewayLoadCost != decoded.CostEstimate.GatewayLoadCost {
			t.Fatalf("CostEstimate.GatewayLoadCost mismatch")
		}

		// RemainingBudget
		rb := original.RemainingBudget
		drb := decoded.RemainingBudget
		if rb.ProfileID != drb.ProfileID {
			t.Fatalf("RemainingBudget.ProfileID mismatch")
		}
		if rb.SessionID != drb.SessionID {
			t.Fatalf("RemainingBudget.SessionID mismatch")
		}
		if rb.LatencyBudgetMs != drb.LatencyBudgetMs {
			t.Fatalf("RemainingBudget.LatencyBudgetMs mismatch")
		}
		if rb.BandwidthBudgetRatio != drb.BandwidthBudgetRatio {
			t.Fatalf("RemainingBudget.BandwidthBudgetRatio mismatch")
		}
		if rb.SwitchBudgetPerHour != drb.SwitchBudgetPerHour {
			t.Fatalf("RemainingBudget.SwitchBudgetPerHour mismatch")
		}
		if rb.EntryBurnBudgetPerDay != drb.EntryBurnBudgetPerDay {
			t.Fatalf("RemainingBudget.EntryBurnBudgetPerDay mismatch")
		}
		if rb.GatewayLoadBudget != drb.GatewayLoadBudget {
			t.Fatalf("RemainingBudget.GatewayLoadBudget mismatch")
		}
		if rb.HardenedAllowed != drb.HardenedAllowed {
			t.Fatalf("RemainingBudget.HardenedAllowed mismatch")
		}
		if rb.EscapeAllowed != drb.EscapeAllowed {
			t.Fatalf("RemainingBudget.EscapeAllowed mismatch")
		}
		if rb.LastResortAllowed != drb.LastResortAllowed {
			t.Fatalf("RemainingBudget.LastResortAllowed mismatch")
		}
	})
}

// TestProperty8_BudgetProfile_SnakeCaseKeys
// Feature: v2-budget-engine, Property 8: JSON round-trip (snake_case verification)
// **Validates: Requirements 9.4**
// For any valid BudgetProfile, JSON output uses snake_case key names and contains no camelCase keys.
func TestProperty8_BudgetProfile_SnakeCaseKeys(t *testing.T) {
	now := time.Now().Truncate(time.Second).UTC()

	expectedKeys := map[string]bool{
		"profile_id":                true,
		"session_id":                true,
		"latency_budget_ms":         true,
		"bandwidth_budget_ratio":    true,
		"switch_budget_per_hour":    true,
		"entry_burn_budget_per_day": true,
		"gateway_load_budget":       true,
		"hardened_allowed":          true,
		"escape_allowed":            true,
		"last_resort_allowed":       true,
		"created_at":                true,
		"updated_at":                true,
	}

	// camelCase variants that must NOT appear
	forbiddenKeys := map[string]bool{
		"profileId":             true,
		"sessionId":             true,
		"latencyBudgetMs":       true,
		"bandwidthBudgetRatio":  true,
		"switchBudgetPerHour":   true,
		"entryBurnBudgetPerDay": true,
		"gatewayLoadBudget":     true,
		"hardenedAllowed":       true,
		"escapeAllowed":         true,
		"lastResortAllowed":     true,
		"createdAt":             true,
		"updatedAt":             true,
		"ProfileID":             true,
		"SessionID":             true,
		"LatencyBudgetMs":       true,
		"BandwidthBudgetRatio":  true,
		"SwitchBudgetPerHour":   true,
		"EntryBurnBudgetPerDay": true,
		"GatewayLoadBudget":     true,
		"HardenedAllowed":       true,
		"EscapeAllowed":         true,
		"LastResortAllowed":     true,
		"CreatedAt":             true,
		"UpdatedAt":             true,
	}

	rapid.Check(t, func(t *rapid.T) {
		bp := genBudgetProfile(t, now)

		data, err := json.Marshal(bp)
		if err != nil {
			t.Fatalf("marshal failed: %v", err)
		}

		var m map[string]interface{}
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatalf("unmarshal to map failed: %v", err)
		}

		// Verify all expected snake_case keys exist
		for key := range expectedKeys {
			if _, ok := m[key]; !ok {
				t.Fatalf("expected snake_case key %q not found in JSON output", key)
			}
		}

		// Verify no forbidden camelCase/PascalCase keys exist
		for key := range m {
			if forbiddenKeys[key] {
				t.Fatalf("forbidden camelCase/PascalCase key %q found in JSON output", key)
			}
		}
	})
}
