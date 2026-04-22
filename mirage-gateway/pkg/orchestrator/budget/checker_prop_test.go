package budget

import (
	"context"
	"testing"

	"mirage-gateway/pkg/orchestrator/commit"

	"pgregory.net/rapid"
)

// --- Mock implementations for controlled testing ---

// mockCostModel returns a fixed CostEstimate
type mockCostModel struct {
	estimate *CostEstimate
	err      error
}

func (m *mockCostModel) Estimate(_ *commit.CommitTransaction) (*CostEstimate, error) {
	return m.estimate, m.err
}

// mockLedger returns controlled switch/entry burn counts
type mockLedger struct {
	switchCount    int
	entryBurnCount int
}

func (m *mockLedger) Record(_ *LedgerEntry)                {}
func (m *mockLedger) SwitchCountInLastHour(_ string) int   { return m.switchCount }
func (m *mockLedger) EntryBurnCountInLastDay(_ string) int { return m.entryBurnCount }
func (m *mockLedger) Cleanup()                             {}

// mockStore returns a fixed BudgetProfile
type mockStore struct {
	profile *BudgetProfile
	err     error
}

func (m *mockStore) Get(_ context.Context, _ string) (*BudgetProfile, error) {
	return m.profile, m.err
}
func (m *mockStore) Save(_ context.Context, _ *BudgetProfile) error { return nil }
func (m *mockStore) LoadAll(_ context.Context) ([]*BudgetProfile, error) {
	return nil, nil
}

// --- Generators ---

func checkerGenSurvivalMode() *rapid.Generator[string] {
	return rapid.SampledFrom([]string{"Normal", "LowNoise", "Hardened", "Degraded", "Escape", "LastResort"})
}

func checkerGenProfile(t *rapid.T) *BudgetProfile {
	return &BudgetProfile{
		ProfileID:             "test-profile",
		SessionID:             "test-session",
		LatencyBudgetMs:       rapid.Int64Range(1, 1000).Draw(t, "latency_budget_ms"),
		BandwidthBudgetRatio:  rapid.Float64Range(0.01, 1.0).Draw(t, "bandwidth_budget_ratio"),
		SwitchBudgetPerHour:   rapid.IntRange(0, 100).Draw(t, "switch_budget_per_hour"),
		EntryBurnBudgetPerDay: rapid.IntRange(0, 50).Draw(t, "entry_burn_budget_per_day"),
		GatewayLoadBudget:     rapid.Float64Range(0.01, 1.0).Draw(t, "gateway_load_budget"),
		HardenedAllowed:       rapid.Bool().Draw(t, "hardened_allowed"),
		EscapeAllowed:         rapid.Bool().Draw(t, "escape_allowed"),
		LastResortAllowed:     rapid.Bool().Draw(t, "last_resort_allowed"),
	}
}

func checkerGenCostEstimate(t *rapid.T) *CostEstimate {
	bw := rapid.Float64Range(0, 0.5).Draw(t, "bandwidth_cost")
	lat := rapid.Float64Range(0, 500).Draw(t, "latency_cost")
	sw := rapid.Float64Range(0, 0.5).Draw(t, "switch_cost")
	eb := rapid.Float64Range(0, 0.5).Draw(t, "entry_burn_cost")
	gl := rapid.Float64Range(0, 0.5).Draw(t, "gateway_load_cost")
	return &CostEstimate{
		BandwidthCost:   bw,
		LatencyCost:     lat,
		SwitchCost:      sw,
		EntryBurnCost:   eb,
		GatewayLoadCost: gl,
		TotalCost:       bw + lat + sw + eb + gl,
	}
}

// --- Property 4: Decision tree correctness ---
// **Validates: Requirements 4.2, 4.3, 4.4, 4.5, 4.6, 4.7**

func TestProperty4_DecisionTreeCorrectness(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		txType := rapid.SampledFrom(commit.AllTxTypes).Draw(t, "tx_type")
		survivalMode := checkerGenSurvivalMode().Draw(t, "survival_mode")
		profile := checkerGenProfile(t)
		cost := checkerGenCostEstimate(t)
		switchCount := rapid.IntRange(0, 200).Draw(t, "switch_count")
		entryBurnCount := rapid.IntRange(0, 200).Draw(t, "entry_burn_count")

		tx := &commit.CommitTransaction{
			TxID:               "test-tx",
			TxType:             txType,
			TxPhase:            commit.TxPhaseValidating,
			TxScope:            commit.TxTypeScopeMap[txType],
			TargetSessionID:    "test-session",
			TargetSurvivalMode: survivalMode,
		}

		checker := NewBudgetCheckerImpl(
			&mockCostModel{estimate: cost},
			nil, // slaPolicy not used directly by checker
			&mockLedger{switchCount: switchCount, entryBurnCount: entryBurnCount},
			&mockStore{profile: profile},
		)

		decision, err := checker.Evaluate(context.Background(), tx)
		if err != nil {
			t.Fatalf("Evaluate returned error: %v", err)
		}

		// Rule 1: SLA permission denial for SurvivalModeSwitch
		if txType == commit.TxTypeSurvivalModeSwitch {
			slaBlocked := false
			switch survivalMode {
			case "Hardened":
				slaBlocked = !profile.HardenedAllowed
			case "Escape":
				slaBlocked = !profile.EscapeAllowed
			case "LastResort":
				slaBlocked = !profile.LastResortAllowed
			}
			if slaBlocked {
				if decision.Verdict != VerdictDenyAndHold {
					t.Fatalf("SLA blocked mode %s but verdict=%s, expected deny_and_hold", survivalMode, decision.Verdict)
				}
				if decision.DenyReason == "" {
					t.Fatal("deny verdict must have non-empty deny_reason")
				}
				return // SLA denial takes precedence
			}
		}

		// Rule 2: Daily suspend check
		dailySuspend := false
		if profile.EntryBurnBudgetPerDay > 0 {
			ratio := float64(entryBurnCount) / float64(profile.EntryBurnBudgetPerDay)
			if ratio > DailySuspendThreshold {
				dailySuspend = true
			}
		}
		if dailySuspend {
			if decision.Verdict != VerdictDenyAndSuspend {
				t.Fatalf("daily suspend condition met (entryBurn=%d, budget=%d) but verdict=%s, expected deny_and_suspend",
					entryBurnCount, profile.EntryBurnBudgetPerDay, decision.Verdict)
			}
			if decision.DenyReason == "" {
				t.Fatal("deny_and_suspend verdict must have non-empty deny_reason")
			}
			return
		}

		// Rule 3: Compute over-budget ratio
		overBudgetRatio := checkerComputeOverBudgetRatio(cost, profile, switchCount, entryBurnCount)

		// Rule 4: Within budget → allow
		if overBudgetRatio <= 0 {
			if decision.Verdict != VerdictAllow {
				t.Fatalf("within budget (ratio=%.4f) but verdict=%s, expected allow", overBudgetRatio, decision.Verdict)
			}
			return
		}

		// Rule 5: Over budget ≤ 20% → allow_degraded
		if overBudgetRatio <= OverBudgetThreshold {
			if decision.Verdict != VerdictAllowDegraded {
				t.Fatalf("over budget by %.4f (≤ threshold) but verdict=%s, expected allow_degraded", overBudgetRatio, decision.Verdict)
			}
			return
		}

		// Rule 6: Over budget > 20% → deny_and_hold
		if decision.Verdict != VerdictDenyAndHold {
			t.Fatalf("over budget by %.4f (> threshold) but verdict=%s, expected deny_and_hold", overBudgetRatio, decision.Verdict)
		}
		if decision.DenyReason == "" {
			t.Fatal("deny_and_hold verdict must have non-empty deny_reason")
		}
	})
}

// TestProperty4_CheckMethodConsistency verifies Check returns nil for allow verdicts and ErrBudgetDenied for deny verdicts
func TestProperty4_CheckMethodConsistency(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		txType := rapid.SampledFrom(commit.AllTxTypes).Draw(t, "tx_type")
		profile := checkerGenProfile(t)
		cost := checkerGenCostEstimate(t)

		tx := &commit.CommitTransaction{
			TxID:               "test-tx",
			TxType:             txType,
			TxPhase:            commit.TxPhaseValidating,
			TxScope:            commit.TxTypeScopeMap[txType],
			TargetSessionID:    "test-session",
			TargetSurvivalMode: "Normal", // always allowed
		}

		checker := NewBudgetCheckerImpl(
			&mockCostModel{estimate: cost},
			nil,
			&mockLedger{switchCount: 0, entryBurnCount: 0},
			&mockStore{profile: profile},
		)

		decision, err := checker.Evaluate(context.Background(), tx)
		if err != nil {
			t.Fatalf("Evaluate returned error: %v", err)
		}

		checkErr := checker.Check(context.Background(), tx)

		switch decision.Verdict {
		case VerdictAllow, VerdictAllowDegraded, VerdictAllowWithCharge:
			if checkErr != nil {
				t.Fatalf("Check returned error for allow verdict %s: %v", decision.Verdict, checkErr)
			}
		case VerdictDenyAndHold, VerdictDenyAndSuspend:
			if checkErr == nil {
				t.Fatalf("Check returned nil for deny verdict %s", decision.Verdict)
			}
			budgetErr, ok := checkErr.(*ErrBudgetDenied)
			if !ok {
				t.Fatalf("Check returned non-ErrBudgetDenied error: %T", checkErr)
			}
			if budgetErr.Verdict != decision.Verdict {
				t.Fatalf("ErrBudgetDenied verdict=%s, expected=%s", budgetErr.Verdict, decision.Verdict)
			}
		}
	})
}

// checkerComputeOverBudgetRatio mirrors the checker's logic for test verification
func checkerComputeOverBudgetRatio(cost *CostEstimate, profile *BudgetProfile, switchCount, entryBurnCount int) float64 {
	maxRatio := 0.0

	if cost.SwitchCost > 0 {
		if profile.SwitchBudgetPerHour == 0 {
			return 1.0
		}
		sc := float64(switchCount + 1)
		sb := float64(profile.SwitchBudgetPerHour)
		ratio := (sc - sb) / sb
		if ratio > maxRatio {
			maxRatio = ratio
		}
	}

	if cost.EntryBurnCost > 0 {
		if profile.EntryBurnBudgetPerDay == 0 {
			return 1.0
		}
		ec := float64(entryBurnCount + 1)
		eb := float64(profile.EntryBurnBudgetPerDay)
		ratio := (ec - eb) / eb
		if ratio > maxRatio {
			maxRatio = ratio
		}
	}

	if cost.BandwidthCost > 0 {
		if profile.BandwidthBudgetRatio == 0 {
			return 1.0
		}
		ratio := (cost.BandwidthCost - profile.BandwidthBudgetRatio) / profile.BandwidthBudgetRatio
		if ratio > maxRatio {
			maxRatio = ratio
		}
	}

	if cost.LatencyCost > 0 {
		if profile.LatencyBudgetMs == 0 {
			return 1.0
		}
		ratio := (cost.LatencyCost - float64(profile.LatencyBudgetMs)) / float64(profile.LatencyBudgetMs)
		if ratio > maxRatio {
			maxRatio = ratio
		}
	}

	if cost.GatewayLoadCost > 0 {
		if profile.GatewayLoadBudget == 0 {
			return 1.0
		}
		ratio := (cost.GatewayLoadCost - profile.GatewayLoadBudget) / profile.GatewayLoadBudget
		if ratio > maxRatio {
			maxRatio = ratio
		}
	}

	return maxRatio
}
