package budget

// BudgetDecision 预算判定完整响应
type BudgetDecision struct {
	Verdict         BudgetVerdict  `json:"verdict"`
	CostEstimate    *CostEstimate  `json:"cost_estimate"`
	RemainingBudget *BudgetProfile `json:"remaining_budget"`
	DenyReason      string         `json:"deny_reason,omitempty"`
}
