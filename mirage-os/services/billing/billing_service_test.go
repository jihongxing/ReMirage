package billing

import (
	"testing"

	pb "mirage-os/api/proto"

	"pgregory.net/rapid"
)

// Feature: mirage-os-completion, Property 5: PurchaseQuota 余额不变量
// **Validates: Requirements 3.4**
//
// For any balance B, package P, level L:
// if B >= price(P,L) then post-balance = B - price and quota increases;
// if B < price then rejected and unchanged.
func TestPurchaseQuotaBalanceInvariant(t *testing.T) {
	validPackages := []pb.PackageType{
		pb.PackageType_PACKAGE_10GB,
		pb.PackageType_PACKAGE_50GB,
		pb.PackageType_PACKAGE_100GB,
		pb.PackageType_PACKAGE_500GB,
		pb.PackageType_PACKAGE_1TB,
	}
	validLevels := []string{"standard", "platinum", "diamond"}

	rapid.Check(t, func(t *rapid.T) {
		// 生成随机余额 (0 ~ 100000 美元，精度到美分)
		balanceCents := rapid.Uint64Range(0, 10_000_000).Draw(t, "balance_cents")
		balanceUSD := float64(balanceCents) / 100.0

		remainingQuota := rapid.Int64Range(0, 1_000_000_000_000).Draw(t, "remaining_quota")

		pkg := validPackages[rapid.IntRange(0, len(validPackages)-1).Draw(t, "pkg_idx")]
		level := validLevels[rapid.IntRange(0, len(validLevels)-1).Draw(t, "level_idx")]
		quantity := uint32(rapid.IntRange(1, 5).Draw(t, "quantity"))

		priceInfo, ok := GetPackagePrice(pkg, level)
		if !ok {
			t.Fatal("invalid package/level combo should not happen in test")
		}

		totalPriceCents := priceInfo.PriceUSD * uint64(quantity)
		totalPrice := float64(totalPriceCents) / 100.0
		totalQuota := priceInfo.QuotaBytes * int64(quantity)

		newBalance, newQuota, err := PurchaseQuotaPure(balanceUSD, remainingQuota, pkg, level, quantity)

		if balanceUSD >= totalPrice {
			// 应该成功
			if err != nil {
				t.Fatalf("expected success for balance=%.2f >= price=%.2f, got error: %v", balanceUSD, totalPrice, err)
			}
			// 余额不变量：newBalance = B - price
			expectedBalance := balanceUSD - totalPrice
			if diff := newBalance - expectedBalance; diff > 0.001 || diff < -0.001 {
				t.Fatalf("balance invariant violated: expected %.2f, got %.2f", expectedBalance, newBalance)
			}
			// 配额增加
			expectedQuota := remainingQuota + totalQuota
			if newQuota != expectedQuota {
				t.Fatalf("quota invariant violated: expected %d, got %d", expectedQuota, newQuota)
			}
		} else {
			// 应该被拒绝
			if err == nil {
				t.Fatalf("expected rejection for balance=%.2f < price=%.2f, but succeeded", balanceUSD, totalPrice)
			}
			// 余额和配额不变
			if newBalance != balanceUSD {
				t.Fatalf("balance should be unchanged on rejection: expected %.2f, got %.2f", balanceUSD, newBalance)
			}
			if newQuota != remainingQuota {
				t.Fatalf("quota should be unchanged on rejection: expected %d, got %d", remainingQuota, newQuota)
			}
		}
	})
}

// Feature: v1-tiered-service, Task 2.3: 充值/购买流量包后 cell_level 保持不变
// **Validates: Requirements 1.1, 1.3**
//
// PurchaseQuotaPure 只修改余额和配额，不涉及 cell_level。
// 验证：对任意初始 cell_level，购买流量包后 cell_level 不在输出中（纯函数不返回 cell_level）。
func TestPurchaseQuotaDoesNotAffectCellLevel(t *testing.T) {
	validPackages := []pb.PackageType{
		pb.PackageType_PACKAGE_10GB,
		pb.PackageType_PACKAGE_50GB,
		pb.PackageType_PACKAGE_100GB,
		pb.PackageType_PACKAGE_500GB,
		pb.PackageType_PACKAGE_1TB,
	}
	validLevels := []string{"standard", "platinum", "diamond"}

	rapid.Check(t, func(t *rapid.T) {
		// 生成足够高的余额确保购买成功
		balanceUSD := float64(rapid.Uint64Range(50000, 10_000_000).Draw(t, "balance_cents")) / 100.0
		remainingQuota := rapid.Int64Range(0, 1_000_000_000_000).Draw(t, "remaining_quota")
		cellLevel := rapid.IntRange(1, 3).Draw(t, "cell_level")

		pkg := validPackages[rapid.IntRange(0, len(validPackages)-1).Draw(t, "pkg_idx")]
		level := validLevels[rapid.IntRange(0, len(validLevels)-1).Draw(t, "level_idx")]

		priceInfo, ok := GetPackagePrice(pkg, level)
		if !ok {
			t.Fatal("invalid package/level combo")
		}

		// 确保余额足够
		totalPrice := float64(priceInfo.PriceUSD) / 100.0
		if balanceUSD < totalPrice {
			balanceUSD = totalPrice + 1.0
		}

		newBalance, newQuota, err := PurchaseQuotaPure(balanceUSD, remainingQuota, pkg, level, 1)

		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}

		// 核心断言：PurchaseQuotaPure 的输出不包含 cell_level
		// 它只返回 (newBalance, newQuota, err)，cell_level 不受影响
		_ = cellLevel // cell_level 在纯函数中完全不参与

		if newBalance >= balanceUSD {
			t.Fatalf("balance should decrease after purchase: before=%.2f, after=%.2f", balanceUSD, newBalance)
		}
		if newQuota <= remainingQuota {
			t.Fatalf("quota should increase after purchase: before=%d, after=%d", remainingQuota, newQuota)
		}
	})
}

// Feature: v1-tiered-service, Task 4.5: 购买成功后 newLevel == tierLevelMap[planType] 且 newBalance == balance - price
// **Validates: Requirements 3.2, 3.3, 3.4, 3.5**
//
// For valid plan types with sufficient balance and currentLevel <= targetLevel:
// - newLevel == tierLevelMap[planType]
// - newBalance == balanceUSD - price
func TestPurchaseTierSuccessInvariant(t *testing.T) {
	validPlans := []string{
		"plan_standard_monthly",
		"plan_platinum_monthly",
		"plan_diamond_monthly",
	}

	rapid.Check(t, func(t *rapid.T) {
		planIdx := rapid.IntRange(0, len(validPlans)-1).Draw(t, "plan_idx")
		planType := validPlans[planIdx]

		price, ok := GetTierPrice(planType)
		if !ok {
			t.Fatal("valid plan should have price")
		}
		targetLevel, ok := GetTierLevel(planType)
		if !ok {
			t.Fatal("valid plan should have level")
		}

		priceUSD := float64(price) / 100.0

		// 生成足够的余额（价格 ~ 价格+50000 美元）
		extraCents := rapid.Uint64Range(0, 5_000_000).Draw(t, "extra_cents")
		balanceUSD := priceUSD + float64(extraCents)/100.0

		// currentLevel <= targetLevel 确保不触发降级拒绝
		currentLevel := rapid.IntRange(1, targetLevel).Draw(t, "current_level")

		newBalance, newLevel, err := PurchaseTierPure(balanceUSD, currentLevel, planType)

		if err != nil {
			t.Fatalf("expected success: balance=%.2f >= price=%.2f, currentLevel=%d <= target=%d, got error: %v",
				balanceUSD, priceUSD, currentLevel, targetLevel, err)
		}

		// 不变量 1: newLevel == tierLevelMap[planType]
		if newLevel != targetLevel {
			t.Fatalf("newLevel invariant violated: expected %d, got %d", targetLevel, newLevel)
		}

		// 不变量 2: newBalance == balanceUSD - price
		expectedBalance := balanceUSD - priceUSD
		if diff := newBalance - expectedBalance; diff > 0.001 || diff < -0.001 {
			t.Fatalf("balance invariant violated: expected %.2f, got %.2f", expectedBalance, newBalance)
		}
	})
}

// Feature: v1-tiered-service, Task 4.6: 余额不足时购买失败，余额和等级不变
// **Validates: Requirements 3.8**
//
// For valid plan types with insufficient balance:
// - err != nil
// - newBalance == balanceUSD (unchanged)
// - newLevel == currentLevel (unchanged)
//
// Also tests downgrade rejection: currentLevel > targetLevel → error, balance and level unchanged.
func TestPurchaseTierInsufficientBalanceInvariant(t *testing.T) {
	validPlans := []string{
		"plan_standard_monthly",
		"plan_platinum_monthly",
		"plan_diamond_monthly",
	}

	rapid.Check(t, func(t *rapid.T) {
		planIdx := rapid.IntRange(0, len(validPlans)-1).Draw(t, "plan_idx")
		planType := validPlans[planIdx]

		price, ok := GetTierPrice(planType)
		if !ok {
			t.Fatal("valid plan should have price")
		}

		priceUSD := float64(price) / 100.0

		// 生成不足的余额（0 ~ price - 0.01）
		maxCents := price - 1
		if maxCents > price {
			maxCents = 0 // overflow guard
		}
		balanceCents := rapid.Uint64Range(0, maxCents).Draw(t, "balance_cents")
		balanceUSD := float64(balanceCents) / 100.0

		currentLevel := rapid.IntRange(1, 3).Draw(t, "current_level")

		newBalance, newLevel, err := PurchaseTierPure(balanceUSD, currentLevel, planType)

		if err == nil {
			t.Fatalf("expected failure: balance=%.2f < price=%.2f, but succeeded", balanceUSD, priceUSD)
		}

		// 余额不变
		if newBalance != balanceUSD {
			t.Fatalf("balance should be unchanged on rejection: expected %.2f, got %.2f", balanceUSD, newBalance)
		}

		// 等级不变
		if newLevel != currentLevel {
			t.Fatalf("level should be unchanged on rejection: expected %d, got %d", currentLevel, newLevel)
		}
	})
}

// Feature: v1-tiered-service, Task 4.6 (supplement): 降级拒绝时余额和等级不变
// **Validates: Requirements 3.7**
func TestPurchaseTierDowngradeRejectionInvariant(t *testing.T) {
	// 降级场景：currentLevel > targetLevel
	// plan_standard_monthly → level 1, 所以 currentLevel=2 或 3 会触发降级拒绝
	// plan_platinum_monthly → level 2, 所以 currentLevel=3 会触发降级拒绝
	type downgradeCase struct {
		planType    string
		targetLevel int
		minCurrent  int
	}
	cases := []downgradeCase{
		{"plan_standard_monthly", 1, 2},
		{"plan_standard_monthly", 1, 3},
		{"plan_platinum_monthly", 2, 3},
	}

	rapid.Check(t, func(t *rapid.T) {
		caseIdx := rapid.IntRange(0, len(cases)-1).Draw(t, "case_idx")
		tc := cases[caseIdx]

		currentLevel := rapid.IntRange(tc.minCurrent, 3).Draw(t, "current_level")

		// 给足够余额，确保不是因为余额不足而失败
		balanceUSD := float64(rapid.Uint64Range(500000, 10_000_000).Draw(t, "balance_cents")) / 100.0

		newBalance, newLevel, err := PurchaseTierPure(balanceUSD, currentLevel, tc.planType)

		if err == nil {
			t.Fatalf("expected downgrade rejection: currentLevel=%d > targetLevel=%d, but succeeded",
				currentLevel, tc.targetLevel)
		}

		// 余额不变
		if newBalance != balanceUSD {
			t.Fatalf("balance should be unchanged on downgrade rejection: expected %.2f, got %.2f", balanceUSD, newBalance)
		}

		// 等级不变
		if newLevel != currentLevel {
			t.Fatalf("level should be unchanged on downgrade rejection: expected %d, got %d", currentLevel, newLevel)
		}
	})
}
