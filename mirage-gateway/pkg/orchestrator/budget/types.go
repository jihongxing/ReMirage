// Package budget - V2 Budget Engine 预算引擎
package budget

// BudgetVerdict 预算判定结果枚举
type BudgetVerdict string

const (
	VerdictAllow           BudgetVerdict = "allow"
	VerdictAllowDegraded   BudgetVerdict = "allow_degraded"
	VerdictAllowWithCharge BudgetVerdict = "allow_with_charge"
	VerdictDenyAndHold     BudgetVerdict = "deny_and_hold"
	VerdictDenyAndSuspend  BudgetVerdict = "deny_and_suspend"
)

// OverBudgetThreshold 超预算降级阈值（20%）
const OverBudgetThreshold = 0.20

// DailySuspendThreshold 日预算挂起阈值（150%）
const DailySuspendThreshold = 1.50
