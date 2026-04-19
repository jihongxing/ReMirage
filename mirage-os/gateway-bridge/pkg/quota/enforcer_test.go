package quota

import (
	"math"
	"testing"

	"mirage-os/gateway-bridge/pkg/config"

	"pgregory.net/rapid"
)

func newTestEnforcer() *Enforcer {
	return NewEnforcer(nil, config.PricingConfig{
		BusinessPricePerGB: 0.10,
		DefensePricePerGB:  0.05,
	})
}

// Feature: mirage-os-brain, Property 1: 流量费用计算精度
func TestProperty_CostCalculation(t *testing.T) {
	e := newTestEnforcer()
	rapid.Check(t, func(t *rapid.T) {
		businessBytes := rapid.Uint64Range(0, 100*1e9).Draw(t, "businessBytes")
		defenseBytes := rapid.Uint64Range(0, 100*1e9).Draw(t, "defenseBytes")
		multiplier := rapid.SampledFrom([]float64{1.0, 1.5, 2.0}).Draw(t, "multiplier")

		bc, dc, tc := e.CalculateCost(businessBytes, defenseBytes, multiplier)

		expectedBC := (float64(businessBytes) / 1e9) * 0.10 * multiplier
		expectedDC := (float64(defenseBytes) / 1e9) * 0.05 * multiplier
		expectedTC := expectedBC + expectedDC

		if math.Abs(bc-expectedBC) > 1e-10 {
			t.Fatalf("businessCost mismatch: got %v, want %v", bc, expectedBC)
		}
		if math.Abs(dc-expectedDC) > 1e-10 {
			t.Fatalf("defenseCost mismatch: got %v, want %v", dc, expectedDC)
		}
		if math.Abs(tc-expectedTC) > 1e-10 {
			t.Fatalf("totalCost mismatch: got %v, want %v", tc, expectedTC)
		}
	})
}

// Feature: mirage-os-brain, Property 2: 结算精度一致性
func TestProperty_SettlementConsistency(t *testing.T) {
	e := newTestEnforcer()
	rapid.Check(t, func(t *rapid.T) {
		initialQuota := rapid.Float64Range(1.0, 10000.0).Draw(t, "initialQuota")
		businessBytes := rapid.Uint64Range(0, 10*1e9).Draw(t, "businessBytes")
		defenseBytes := rapid.Uint64Range(0, 10*1e9).Draw(t, "defenseBytes")
		multiplier := rapid.SampledFrom([]float64{1.0, 1.5, 2.0}).Draw(t, "multiplier")

		_, _, totalCost := e.CalculateCost(businessBytes, defenseBytes, multiplier)
		newQuota := initialQuota - totalCost

		if math.Abs((initialQuota-newQuota)-totalCost) > 1e-10 {
			t.Fatalf("settlement inconsistency: initial=%v, new=%v, cost=%v", initialQuota, newQuota, totalCost)
		}
	})
}

// Feature: mirage-os-brain, Property 3: 配额归零触发
func TestProperty_QuotaExhaustion(t *testing.T) {
	e := newTestEnforcer()
	rapid.Check(t, func(t *rapid.T) {
		initialQuota := rapid.Float64Range(0.001, 1.0).Draw(t, "initialQuota")
		// 生成足够大的流量确保超过配额
		businessBytes := rapid.Uint64Range(uint64(100*1e9), uint64(1000*1e9)).Draw(t, "businessBytes")
		multiplier := 1.0

		_, _, totalCost := e.CalculateCost(businessBytes, 0, multiplier)
		remaining := initialQuota - totalCost

		if totalCost >= initialQuota && remaining > 0 {
			t.Fatalf("quota should be <= 0 when cost >= initial: cost=%v, initial=%v, remaining=%v",
				totalCost, initialQuota, remaining)
		}
	})
}

// Feature: mirage-os-brain, Property 4: 蜂窝级别倍率单调性
func TestProperty_MultiplierMonotonicity(t *testing.T) {
	e := newTestEnforcer()
	rapid.Check(t, func(t *rapid.T) {
		businessBytes := rapid.Uint64Range(1, 100*1e9).Draw(t, "businessBytes")
		defenseBytes := rapid.Uint64Range(1, 100*1e9).Draw(t, "defenseBytes")

		_, _, costStandard := e.CalculateCost(businessBytes, defenseBytes, 1.0)
		_, _, costPlatinum := e.CalculateCost(businessBytes, defenseBytes, 1.5)
		_, _, costDiamond := e.CalculateCost(businessBytes, defenseBytes, 2.0)

		if costDiamond <= costPlatinum {
			t.Fatalf("DIAMOND cost (%v) should > PLATINUM cost (%v)", costDiamond, costPlatinum)
		}
		if costPlatinum <= costStandard {
			t.Fatalf("PLATINUM cost (%v) should > STANDARD cost (%v)", costPlatinum, costStandard)
		}
	})
}

// 单元测试：空流量跳过结算
func TestCalculateCost_ZeroTraffic(t *testing.T) {
	e := newTestEnforcer()
	bc, dc, tc := e.CalculateCost(0, 0, 1.0)
	if bc != 0 || dc != 0 || tc != 0 {
		t.Fatalf("zero traffic should produce zero cost: bc=%v, dc=%v, tc=%v", bc, dc, tc)
	}
}
