package budget

import (
	"context"
	"fmt"

	"mirage-gateway/pkg/orchestrator/commit"
)

// BudgetCheckerImpl 实现 commit.BudgetChecker 接口
type BudgetCheckerImpl struct {
	costModel InternalCostModel
	slaPolicy ExternalSLAPolicy
	ledger    BudgetLedger
	store     BudgetProfileStore
}

// compile-time interface check
var _ commit.BudgetChecker = (*BudgetCheckerImpl)(nil)

// NewBudgetCheckerImpl 创建 BudgetCheckerImpl
func NewBudgetCheckerImpl(costModel InternalCostModel, slaPolicy ExternalSLAPolicy, ledger BudgetLedger, store BudgetProfileStore) *BudgetCheckerImpl {
	return &BudgetCheckerImpl{
		costModel: costModel,
		slaPolicy: slaPolicy,
		ledger:    ledger,
		store:     store,
	}
}

// Check 执行预算判定，返回 nil（allow 类）或 ErrBudgetDenied（deny 类）
func (bc *BudgetCheckerImpl) Check(ctx context.Context, tx *commit.CommitTransaction) error {
	decision, err := bc.Evaluate(ctx, tx)
	if err != nil {
		return err
	}
	switch decision.Verdict {
	case VerdictAllow, VerdictAllowDegraded, VerdictAllowWithCharge:
		return nil
	default:
		return &ErrBudgetDenied{
			Verdict: decision.Verdict,
			Reason:  decision.DenyReason,
		}
	}
}

// Evaluate 执行完整预算判定，返回 BudgetDecision
func (bc *BudgetCheckerImpl) Evaluate(ctx context.Context, tx *commit.CommitTransaction) (*BudgetDecision, error) {
	// Step 1: Get BudgetProfile
	profile, err := bc.store.Get(ctx, tx.TargetSessionID)
	if err != nil {
		return nil, fmt.Errorf("budget checker: failed to get profile: %w", err)
	}

	// Step 2: Estimate cost
	costEst, err := bc.costModel.Estimate(tx)
	if err != nil {
		return nil, fmt.Errorf("budget checker: failed to estimate cost: %w", err)
	}

	decision := &BudgetDecision{
		CostEstimate:    costEst,
		RemainingBudget: profile,
	}

	// Step 3: Check SLA permissions (for SurvivalModeSwitch)
	if tx.TxType == commit.TxTypeSurvivalModeSwitch {
		if reason := bc.checkSLAPermission(profile, tx.TargetSurvivalMode); reason != "" {
			decision.Verdict = VerdictDenyAndHold
			decision.DenyReason = reason
			return decision, nil
		}
	}

	// Step 4: Compare cost vs budget
	overBudgetRatio := bc.computeOverBudgetRatio(tx, profile, costEst)

	// Step 4a: Check daily suspend threshold first
	if bc.shouldSuspend(tx.TargetSessionID, profile) {
		decision.Verdict = VerdictDenyAndSuspend
		decision.DenyReason = "daily cumulative consumption exceeds 150% of daily budget"
		return decision, nil
	}

	// Step 4b: Within budget
	if overBudgetRatio <= 0 {
		decision.Verdict = VerdictAllow
		return decision, nil
	}

	// Step 4c: Over budget but within threshold
	if overBudgetRatio <= OverBudgetThreshold {
		decision.Verdict = VerdictAllowDegraded
		return decision, nil
	}

	// Step 4d: Over budget beyond threshold
	decision.Verdict = VerdictDenyAndHold
	decision.DenyReason = fmt.Sprintf("cost exceeds budget by %.1f%%, threshold is %.1f%%", overBudgetRatio*100, OverBudgetThreshold*100)
	return decision, nil
}

// checkSLAPermission 检查 SLA 权限，返回拒绝原因（空字符串表示允许）
func (bc *BudgetCheckerImpl) checkSLAPermission(profile *BudgetProfile, targetMode string) string {
	switch targetMode {
	case "Hardened":
		if !profile.HardenedAllowed {
			return "survival mode Hardened not allowed by current budget profile"
		}
	case "Escape":
		if !profile.EscapeAllowed {
			return "survival mode Escape not allowed by current budget profile"
		}
	case "LastResort":
		if !profile.LastResortAllowed {
			return "survival mode LastResort not allowed by current budget profile"
		}
	}
	return ""
}

// computeOverBudgetRatio 计算最大超预算比率
// 返回值 <= 0 表示在预算内，> 0 表示超出比率
func (bc *BudgetCheckerImpl) computeOverBudgetRatio(tx *commit.CommitTransaction, profile *BudgetProfile, cost *CostEstimate) float64 {
	maxRatio := 0.0

	// Switch budget check
	if cost.SwitchCost > 0 && profile.SwitchBudgetPerHour > 0 {
		switchCount := float64(bc.ledger.SwitchCountInLastHour(tx.TargetSessionID) + 1)
		switchBudget := float64(profile.SwitchBudgetPerHour)
		ratio := (switchCount - switchBudget) / switchBudget
		if ratio > maxRatio {
			maxRatio = ratio
		}
	} else if cost.SwitchCost > 0 && profile.SwitchBudgetPerHour == 0 {
		// Budget is 0 but cost is non-zero → maximally over budget
		return 1.0
	}

	// Entry burn budget check
	if cost.EntryBurnCost > 0 && profile.EntryBurnBudgetPerDay > 0 {
		entryCount := float64(bc.ledger.EntryBurnCountInLastDay(tx.TargetSessionID) + 1)
		entryBudget := float64(profile.EntryBurnBudgetPerDay)
		ratio := (entryCount - entryBudget) / entryBudget
		if ratio > maxRatio {
			maxRatio = ratio
		}
	} else if cost.EntryBurnCost > 0 && profile.EntryBurnBudgetPerDay == 0 {
		return 1.0
	}

	// Bandwidth budget check
	if cost.BandwidthCost > 0 && profile.BandwidthBudgetRatio > 0 {
		ratio := (cost.BandwidthCost - profile.BandwidthBudgetRatio) / profile.BandwidthBudgetRatio
		if ratio > maxRatio {
			maxRatio = ratio
		}
	} else if cost.BandwidthCost > 0 && profile.BandwidthBudgetRatio == 0 {
		return 1.0
	}

	// Latency budget check
	if cost.LatencyCost > 0 && profile.LatencyBudgetMs > 0 {
		ratio := (cost.LatencyCost - float64(profile.LatencyBudgetMs)) / float64(profile.LatencyBudgetMs)
		if ratio > maxRatio {
			maxRatio = ratio
		}
	} else if cost.LatencyCost > 0 && profile.LatencyBudgetMs == 0 {
		return 1.0
	}

	// Gateway load budget check
	if cost.GatewayLoadCost > 0 && profile.GatewayLoadBudget > 0 {
		ratio := (cost.GatewayLoadCost - profile.GatewayLoadBudget) / profile.GatewayLoadBudget
		if ratio > maxRatio {
			maxRatio = ratio
		}
	} else if cost.GatewayLoadCost > 0 && profile.GatewayLoadBudget == 0 {
		return 1.0
	}

	return maxRatio
}

// shouldSuspend 检查是否应挂起（日累计消耗 > 150% 日预算）
func (bc *BudgetCheckerImpl) shouldSuspend(sessionID string, profile *BudgetProfile) bool {
	if profile.EntryBurnBudgetPerDay <= 0 {
		return false
	}
	entryBurnCount := bc.ledger.EntryBurnCountInLastDay(sessionID)
	ratio := float64(entryBurnCount) / float64(profile.EntryBurnBudgetPerDay)
	return ratio > DailySuspendThreshold
}
