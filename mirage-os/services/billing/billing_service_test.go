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
