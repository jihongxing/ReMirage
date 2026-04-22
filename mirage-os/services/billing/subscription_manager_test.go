package billing

import (
	"testing"

	"mirage-os/pkg/models"

	"pgregory.net/rapid"
)

// Feature: v1-tiered-service, Task 8.6: 到期且不续费的用户处理后 cell_level = 1
// **Validates: Requirements 1.6, 7.3**
//
// For any expired user with auto_renew=false (regardless of balance),
// ProcessExpiredPure should always return cell_level=1.
func TestProperty_ExpiredNoRenewDowngradesToStandard(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		cellLevel := rapid.IntRange(2, 3).Draw(t, "cell_level")

		// auto_renew = false → always downgrade regardless of balance
		balanceCents := rapid.Uint64Range(0, 100_000_000).Draw(t, "balance_cents")
		balanceUSD := float64(balanceCents) / 100.0

		planType := rapid.SampledFrom([]string{
			models.PlanStandardMonthly,
			models.PlanPlatinumMonthly,
			models.PlanDiamondMonthly,
		}).Draw(t, "plan_type")

		newLevel := ProcessExpiredPure(balanceUSD, cellLevel, false, planType)

		if newLevel != 1 {
			t.Fatalf("expected downgrade to 1 when auto_renew=false, got %d (balance=%.2f, plan=%s)",
				newLevel, balanceUSD, planType)
		}
	})
}

// Feature: v1-tiered-service, Task 8.6 (supplement): 到期且余额不足时降级
// **Validates: Requirements 7.3**
//
// For any expired user with insufficient balance (even if auto_renew=true),
// ProcessExpiredPure should return cell_level=1.
func TestProperty_ExpiredInsufficientBalanceDowngrades(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		cellLevel := rapid.IntRange(2, 3).Draw(t, "cell_level")
		autoRenew := rapid.Bool().Draw(t, "auto_renew")

		// Generate balance insufficient for any plan (< $299, cheapest plan)
		balanceCents := rapid.Uint64Range(0, 29899).Draw(t, "balance_cents")
		balanceUSD := float64(balanceCents) / 100.0

		planType := rapid.SampledFrom([]string{
			models.PlanStandardMonthly,
			models.PlanPlatinumMonthly,
			models.PlanDiamondMonthly,
		}).Draw(t, "plan_type")

		newLevel := ProcessExpiredPure(balanceUSD, cellLevel, autoRenew, planType)

		if newLevel != 1 {
			t.Fatalf("expected downgrade to 1 with insufficient balance, got %d (balance=%.2f, autoRenew=%v, plan=%s)",
				newLevel, balanceUSD, autoRenew, planType)
		}
	})
}

// Feature: v1-tiered-service, Task 8.6 (supplement): auto_renew=true 且余额充足时保持等级
// **Validates: Requirements 7.2**
//
// For any expired user with auto_renew=true and sufficient balance,
// ProcessExpiredPure should keep the current cell_level.
func TestProperty_ExpiredAutoRenewSufficientBalanceKeepsLevel(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		cellLevel := rapid.IntRange(2, 3).Draw(t, "cell_level")

		planType := rapid.SampledFrom([]string{
			models.PlanStandardMonthly,
			models.PlanPlatinumMonthly,
			models.PlanDiamondMonthly,
		}).Draw(t, "plan_type")

		// Get the price for this plan and generate sufficient balance
		priceUSD, ok := GetTierPrice(planType)
		if !ok {
			t.Fatal("valid plan should have price")
		}
		priceFloat := float64(priceUSD) / 100.0

		// Balance >= price
		extraCents := rapid.Uint64Range(0, 5_000_000).Draw(t, "extra_cents")
		balanceUSD := priceFloat + float64(extraCents)/100.0

		newLevel := ProcessExpiredPure(balanceUSD, cellLevel, true, planType)

		if newLevel != cellLevel {
			t.Fatalf("expected level to stay %d with auto_renew=true and sufficient balance, got %d (balance=%.2f, plan=%s)",
				cellLevel, newLevel, balanceUSD, planType)
		}
	})
}
