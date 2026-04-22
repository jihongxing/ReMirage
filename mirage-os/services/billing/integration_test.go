package billing

import (
	"testing"

	"mirage-os/pkg/models"
	"mirage-os/pkg/strategy"
	"mirage-os/services/cellular"

	"pgregory.net/rapid"
)

// 9.1 等级购买端到端测试：购买 Platinum → cell_level 变为 2 → 分配到 cell_level=2 的资源池
func TestIntegration_PurchasePlatinumAndAllocate(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// 生成足够余额购买 Platinum ($999)
		balanceCents := rapid.Uint64Range(99900, 500000).Draw(t, "balance_cents")
		balanceUSD := float64(balanceCents) / 100.0
		currentLevel := 1 // Standard

		// Step 1: Purchase Platinum
		newBalance, newLevel, err := PurchaseTierPure(balanceUSD, currentLevel, models.PlanPlatinumMonthly)
		if err != nil {
			t.Fatalf("purchase should succeed: %v", err)
		}
		if newLevel != 2 {
			t.Fatalf("expected level 2 (Platinum), got %d", newLevel)
		}

		// Step 2: Allocate to cell_level=2 resource pool
		cells := []cellular.CellWithLoad{
			{Cell: models.Cell{CellID: "cell-std-1", CellLevel: 1, Status: "active"}, LoadPercent: 30},
			{Cell: models.Cell{CellID: "cell-plat-1", CellLevel: 2, Status: "active"}, LoadPercent: 20},
			{Cell: models.Cell{CellID: "cell-dia-1", CellLevel: 3, Status: "active"}, LoadPercent: 10},
		}
		result, allocLevel, err := cellular.SelectBestCellForTier(cells, newLevel)
		if err != nil {
			t.Fatalf("allocation should succeed: %v", err)
		}
		if result.Cell.CellLevel != 2 {
			t.Fatalf("should allocate to level 2 cell, got level %d", result.Cell.CellLevel)
		}
		if allocLevel != 2 {
			t.Fatalf("allocated level should be 2, got %d", allocLevel)
		}
		_ = newBalance
	})
}

// 9.2 升级测试：Standard → Diamond → cell_level 变为 3
func TestIntegration_UpgradeStandardToDiamond(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// 生成足够余额购买 Diamond ($2999)
		balanceCents := rapid.Uint64Range(299900, 1000000).Draw(t, "balance_cents")
		balanceUSD := float64(balanceCents) / 100.0
		currentLevel := 1 // Standard

		// Step 1: Purchase Diamond directly
		newBalance, newLevel, err := PurchaseTierPure(balanceUSD, currentLevel, models.PlanDiamondMonthly)
		if err != nil {
			t.Fatalf("purchase should succeed: %v", err)
		}
		if newLevel != 3 {
			t.Fatalf("expected level 3 (Diamond), got %d", newLevel)
		}
		expectedBalance := balanceUSD - 2999.0
		if diff := newBalance - expectedBalance; diff > 0.01 || diff < -0.01 {
			t.Fatalf("balance mismatch: expected %.2f, got %.2f", expectedBalance, newBalance)
		}
	})
}

// 9.3 到期降级测试：订阅到期 + auto_renew=false → cell_level 降为 1
func TestIntegration_ExpiryDowngrade(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		cellLevel := rapid.IntRange(2, 3).Draw(t, "cell_level")
		balanceUSD := float64(rapid.Uint64Range(0, 10000000).Draw(t, "balance_cents")) / 100.0
		planType := rapid.SampledFrom([]string{
			models.PlanPlatinumMonthly,
			models.PlanDiamondMonthly,
		}).Draw(t, "plan_type")

		// auto_renew = false → always downgrade
		newLevel := ProcessExpiredPure(balanceUSD, cellLevel, false, planType)
		if newLevel != 1 {
			t.Fatalf("expected downgrade to 1, got %d", newLevel)
		}
	})
}

// 9.4 自动续费测试：订阅到期 + auto_renew=true + 余额充足 → 订阅延长
func TestIntegration_AutoRenewSuccess(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		planType := rapid.SampledFrom([]string{
			models.PlanPlatinumMonthly,
			models.PlanDiamondMonthly,
		}).Draw(t, "plan_type")

		price, _ := GetTierPrice(planType)
		priceUSD := float64(price) / 100.0

		// Sufficient balance
		extraCents := rapid.Uint64Range(0, 5000000).Draw(t, "extra")
		balanceUSD := priceUSD + float64(extraCents)/100.0

		cellLevel := rapid.IntRange(2, 3).Draw(t, "cell_level")

		newLevel := ProcessExpiredPure(balanceUSD, cellLevel, true, planType)
		if newLevel != cellLevel {
			t.Fatalf("expected level to stay %d with auto-renew, got %d", cellLevel, newLevel)
		}
	})
}

// 9.5 配额熔断隔离测试：同一 Gateway 上两个用户，一个耗尽不影响另一个
// 从 billing/strategy 角度测试恢复优先级隔离
func TestIntegration_FuseIsolation(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Two users on same gateway, different levels
		sessions := []strategy.GatewaySessionWithLevel{
			{UserID: "user-diamond", GatewayID: "gw-1", CellLevel: 3},
			{UserID: "user-standard", GatewayID: "gw-1", CellLevel: 1},
		}

		// Sort by priority
		strategy.SortSessionsByPriority(sessions)

		// Diamond should be first (higher priority for recovery)
		if sessions[0].CellLevel != 3 {
			t.Fatal("Diamond user should have higher recovery priority")
		}
		if sessions[1].CellLevel != 1 {
			t.Fatal("Standard user should have lower recovery priority")
		}
	})
}
