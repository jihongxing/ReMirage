package provisioning

import (
	"mirage-os/pkg/models"
	"testing"

	"pgregory.net/rapid"
)

// Feature: v1-tiered-service, Task 2.3: DetermineUserTier 直接返回 cell_level，不受余额影响
// **Validates: Requirements 1.1, 1.3**
//
// 对任意 cell_level ∈ [1,3] 和任意 balance，DetermineUserTier 返回 cell_level 本身。
func TestDetermineUserTierReturnsCellLevelRegardlessOfBalance(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		cellLevel := rapid.IntRange(1, 3).Draw(t, "cell_level")
		balanceUSD := float64(rapid.Uint64Range(0, 100_000_00).Draw(t, "balance_cents")) / 100.0

		user := &models.User{
			CellLevel:  cellLevel,
			BalanceUSD: balanceUSD,
		}

		tier := DetermineUserTier(user)

		if tier != cellLevel {
			t.Fatalf("DetermineUserTier should return cell_level=%d regardless of balance=%.2f, got %d",
				cellLevel, balanceUSD, tier)
		}
	})
}

// DetermineUserTier 对无效 cell_level 默认返回 1 (Standard)
func TestDetermineUserTierDefaultsToStandard(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// 生成无效的 cell_level（0 或 >3）
		invalidLevel := rapid.IntRange(-10, 0).Draw(t, "invalid_level")
		balanceUSD := float64(rapid.Uint64Range(0, 100_000_00).Draw(t, "balance_cents")) / 100.0

		user := &models.User{
			CellLevel:  invalidLevel,
			BalanceUSD: balanceUSD,
		}

		tier := DetermineUserTier(user)

		if tier != 1 {
			t.Fatalf("DetermineUserTier should default to 1 for invalid cell_level=%d, got %d",
				invalidLevel, tier)
		}
	})
}
