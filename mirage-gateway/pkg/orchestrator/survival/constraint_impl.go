package survival

import (
	"time"

	orchestrator "mirage-gateway/pkg/orchestrator"
)

type transitionConstraint struct {
	config TransitionConstraintConfig
}

// NewTransitionConstraint 创建 TransitionConstraint
func NewTransitionConstraint(config TransitionConstraintConfig) TransitionConstraintIface {
	return &transitionConstraint{config: config}
}

func (c *transitionConstraint) Check(
	current orchestrator.SurvivalMode,
	target orchestrator.SurvivalMode,
	enteredCurrentAt time.Time,
	lastUpgradeAt time.Time,
	triggers []TriggerSignal,
) error {
	now := time.Now()

	hasPolicyTrigger := false
	for _, t := range triggers {
		if t.Source == TriggerSourcePolicy {
			hasPolicyTrigger = true
			break
		}
	}

	// 1. minimum_dwell_time 检查（所有迁移都受此约束，包括 Policy）
	if dwellTime, ok := c.config.MinDwellTimes[current]; ok && dwellTime > 0 {
		elapsed := now.Sub(enteredCurrentAt)
		if elapsed < dwellTime {
			return &ErrConstraintViolation{
				ConstraintType: "min_dwell_time",
				Remaining:      dwellTime - elapsed,
			}
		}
	}

	// Policy_Trigger 绕过 cooldown 和 hysteresis
	if hasPolicyTrigger {
		return nil
	}

	currentSeverity := ModeSeverity[current]
	targetSeverity := ModeSeverity[target]

	// 2. 升级迁移 cooldown 检查
	if targetSeverity > currentSeverity {
		elapsed := now.Sub(lastUpgradeAt)
		if elapsed < c.config.UpgradeCooldown {
			return &ErrConstraintViolation{
				ConstraintType: "cooldown",
				Remaining:      c.config.UpgradeCooldown - elapsed,
			}
		}
	}

	// 3. 降级迁移 hysteresis 检查
	if targetSeverity < currentSeverity {
		// 检查触发因素改善幅度是否超过阈值
		// 简化实现：如果没有触发信号或最高严重度仍接近当前模式，拒绝降级
		if len(triggers) == 0 {
			return &ErrConstraintViolation{
				ConstraintType: "hysteresis",
				Remaining:      0,
			}
		}
		maxTriggerSeverity := 0
		for _, t := range triggers {
			if t.Severity > maxTriggerSeverity {
				maxTriggerSeverity = t.Severity
			}
		}
		// 触发因素改善幅度必须超过升级阈值的 hysteresis_margin
		threshold := float64(currentSeverity) * (1.0 - c.config.HysteresisMargin)
		if float64(maxTriggerSeverity) >= threshold {
			return &ErrConstraintViolation{
				ConstraintType: "hysteresis",
				Remaining:      0,
			}
		}
	}

	return nil
}
