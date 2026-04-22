package budget

import "mirage-gateway/pkg/orchestrator/commit"

// CostEstimate 成本估算结果
type CostEstimate struct {
	BandwidthCost   float64 `json:"bandwidth_cost"`
	LatencyCost     float64 `json:"latency_cost"`
	SwitchCost      float64 `json:"switch_cost"`
	EntryBurnCost   float64 `json:"entry_burn_cost"`
	GatewayLoadCost float64 `json:"gateway_load_cost"`
	TotalCost       float64 `json:"total_cost"`
}

// InternalCostModel 内部成本模型
type InternalCostModel interface {
	// Estimate 根据事务类型估算五类成本
	Estimate(tx *commit.CommitTransaction) (*CostEstimate, error)
}
