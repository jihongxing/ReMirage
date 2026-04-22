package budget

import (
	"fmt"

	"mirage-gateway/pkg/orchestrator/commit"
)

// DefaultCostModel 默认内部成本模型实现
type DefaultCostModel struct{}

// NewDefaultCostModel 创建默认成本模型
func NewDefaultCostModel() *DefaultCostModel {
	return &DefaultCostModel{}
}

// Estimate 根据事务类型估算五类成本分量
// 成本分量矩阵：
//
//	PersonaSwitch:       bandwidth ✓, latency ✓
//	LinkMigration:       bandwidth ✓, latency ✓, switch ✓
//	GatewayReassignment: switch ✓, entry_burn ✓, gateway_load ✓
//	SurvivalModeSwitch:  bandwidth ✓, latency ✓, switch ✓, entry_burn ✓, gateway_load ✓
func (m *DefaultCostModel) Estimate(tx *commit.CommitTransaction) (*CostEstimate, error) {
	if tx == nil {
		return nil, fmt.Errorf("transaction is nil")
	}

	est := &CostEstimate{}

	switch tx.TxType {
	case commit.TxTypePersonaSwitch:
		est.BandwidthCost = 0.1
		est.LatencyCost = 0.1

	case commit.TxTypeLinkMigration:
		est.BandwidthCost = 0.1
		est.LatencyCost = 0.1
		est.SwitchCost = 0.1

	case commit.TxTypeGatewayReassignment:
		est.SwitchCost = 0.1
		est.EntryBurnCost = 0.1
		est.GatewayLoadCost = 0.1

	case commit.TxTypeSurvivalModeSwitch:
		est.BandwidthCost = 0.1
		est.LatencyCost = 0.1
		est.SwitchCost = 0.1
		est.EntryBurnCost = 0.1
		est.GatewayLoadCost = 0.1

	default:
		return nil, fmt.Errorf("unknown tx_type: %s", tx.TxType)
	}

	est.TotalCost = est.BandwidthCost + est.LatencyCost + est.SwitchCost + est.EntryBurnCost + est.GatewayLoadCost

	return est, nil
}
