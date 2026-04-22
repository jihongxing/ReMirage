package budget

import (
	"math"
	"testing"

	"mirage-gateway/pkg/orchestrator/commit"

	"pgregory.net/rapid"
)

// genTxType 生成随机合法 TxType
func genTxType(t *rapid.T) commit.TxType {
	types := []commit.TxType{
		commit.TxTypePersonaSwitch,
		commit.TxTypeLinkMigration,
		commit.TxTypeGatewayReassignment,
		commit.TxTypeSurvivalModeSwitch,
	}
	return types[rapid.IntRange(0, len(types)-1).Draw(t, "txTypeIdx")]
}

// genCommitTransaction 生成随机 CommitTransaction
func genCommitTransaction(t *rapid.T) *commit.CommitTransaction {
	txType := genTxType(t)
	return commit.NewCommitTransaction(txType, 0)
}

// costComponentMatrix 定义每种 TxType 应有非零成本的分量
var costComponentMatrix = map[commit.TxType]struct {
	bandwidth   bool
	latency     bool
	switchCost  bool
	entryBurn   bool
	gatewayLoad bool
}{
	commit.TxTypePersonaSwitch:       {bandwidth: true, latency: true, switchCost: false, entryBurn: false, gatewayLoad: false},
	commit.TxTypeLinkMigration:       {bandwidth: true, latency: true, switchCost: true, entryBurn: false, gatewayLoad: false},
	commit.TxTypeGatewayReassignment: {bandwidth: false, latency: false, switchCost: true, entryBurn: true, gatewayLoad: true},
	commit.TxTypeSurvivalModeSwitch:  {bandwidth: true, latency: true, switchCost: true, entryBurn: true, gatewayLoad: true},
}

// TestProperty2_CostComponentMatrixCorrectness
// Feature: v2-budget-engine, Property 2: 成本分量矩阵正确性
// **Validates: Requirements 2.2, 2.3, 2.4, 2.5, 2.7**
func TestProperty2_CostComponentMatrixCorrectness(t *testing.T) {
	model := NewDefaultCostModel()

	rapid.Check(t, func(t *rapid.T) {
		tx := genCommitTransaction(t)
		est, err := model.Estimate(tx)
		if err != nil {
			t.Fatalf("Estimate failed: %v", err)
		}

		expected := costComponentMatrix[tx.TxType]

		// 验证非零分量与矩阵一致
		if expected.bandwidth {
			if est.BandwidthCost <= 0 {
				t.Errorf("TxType=%s: bandwidth_cost should be > 0, got %f", tx.TxType, est.BandwidthCost)
			}
		} else {
			if est.BandwidthCost != 0 {
				t.Errorf("TxType=%s: bandwidth_cost should be 0, got %f", tx.TxType, est.BandwidthCost)
			}
		}

		if expected.latency {
			if est.LatencyCost <= 0 {
				t.Errorf("TxType=%s: latency_cost should be > 0, got %f", tx.TxType, est.LatencyCost)
			}
		} else {
			if est.LatencyCost != 0 {
				t.Errorf("TxType=%s: latency_cost should be 0, got %f", tx.TxType, est.LatencyCost)
			}
		}

		if expected.switchCost {
			if est.SwitchCost <= 0 {
				t.Errorf("TxType=%s: switch_cost should be > 0, got %f", tx.TxType, est.SwitchCost)
			}
		} else {
			if est.SwitchCost != 0 {
				t.Errorf("TxType=%s: switch_cost should be 0, got %f", tx.TxType, est.SwitchCost)
			}
		}

		if expected.entryBurn {
			if est.EntryBurnCost <= 0 {
				t.Errorf("TxType=%s: entry_burn_cost should be > 0, got %f", tx.TxType, est.EntryBurnCost)
			}
		} else {
			if est.EntryBurnCost != 0 {
				t.Errorf("TxType=%s: entry_burn_cost should be 0, got %f", tx.TxType, est.EntryBurnCost)
			}
		}

		if expected.gatewayLoad {
			if est.GatewayLoadCost <= 0 {
				t.Errorf("TxType=%s: gateway_load_cost should be > 0, got %f", tx.TxType, est.GatewayLoadCost)
			}
		} else {
			if est.GatewayLoadCost != 0 {
				t.Errorf("TxType=%s: gateway_load_cost should be 0, got %f", tx.TxType, est.GatewayLoadCost)
			}
		}

		// 所有成本分量非负
		if est.BandwidthCost < 0 {
			t.Errorf("bandwidth_cost is negative: %f", est.BandwidthCost)
		}
		if est.LatencyCost < 0 {
			t.Errorf("latency_cost is negative: %f", est.LatencyCost)
		}
		if est.SwitchCost < 0 {
			t.Errorf("switch_cost is negative: %f", est.SwitchCost)
		}
		if est.EntryBurnCost < 0 {
			t.Errorf("entry_burn_cost is negative: %f", est.EntryBurnCost)
		}
		if est.GatewayLoadCost < 0 {
			t.Errorf("gateway_load_cost is negative: %f", est.GatewayLoadCost)
		}
		if est.TotalCost < 0 {
			t.Errorf("total_cost is negative: %f", est.TotalCost)
		}
	})
}

// TestProperty3_CostEstimateTotalCostInvariant
// Feature: v2-budget-engine, Property 3: CostEstimate 总成本不变量
// **Validates: Requirements 2.6**
func TestProperty3_CostEstimateTotalCostInvariant(t *testing.T) {
	model := NewDefaultCostModel()

	rapid.Check(t, func(t *rapid.T) {
		tx := genCommitTransaction(t)
		est, err := model.Estimate(tx)
		if err != nil {
			t.Fatalf("Estimate failed: %v", err)
		}

		expectedTotal := est.BandwidthCost + est.LatencyCost + est.SwitchCost + est.EntryBurnCost + est.GatewayLoadCost

		if math.Abs(est.TotalCost-expectedTotal) > 1e-10 {
			t.Errorf("total_cost invariant violated: total_cost=%f, sum=%f", est.TotalCost, expectedTotal)
		}
	})
}
