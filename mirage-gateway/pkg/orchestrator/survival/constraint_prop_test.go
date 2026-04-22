package survival

import (
	"errors"
	"testing"
	"time"

	orchestrator "mirage-gateway/pkg/orchestrator"

	"pgregory.net/rapid"
)

// Property 7: 迁移约束综合判定
func TestProperty7_TransitionConstraintCheck(t *testing.T) {
	config := DefaultConstraintConfig
	constraint := NewTransitionConstraint(config)

	rapid.Check(t, func(t *rapid.T) {
		current := genSurvivalMode(t)
		target := genSurvivalMode(t)
		if current == target {
			return // 跳过自迁移
		}

		// 生成时间参数
		dwellSeconds := rapid.IntRange(0, 300).Draw(t, "dwell_seconds")
		cooldownSeconds := rapid.IntRange(0, 120).Draw(t, "cooldown_seconds")
		hasPolicyTrigger := rapid.Bool().Draw(t, "has_policy_trigger")

		now := time.Now()
		enteredCurrentAt := now.Add(-time.Duration(dwellSeconds) * time.Second)
		lastUpgradeAt := now.Add(-time.Duration(cooldownSeconds) * time.Second)

		var triggers []TriggerSignal
		if hasPolicyTrigger {
			triggers = append(triggers, TriggerSignal{
				Source:   TriggerSourcePolicy,
				Reason:   "test policy",
				Severity: ModeSeverity[target],
			})
		} else {
			// 添加低严重度触发信号用于降级场景
			triggers = append(triggers, TriggerSignal{
				Source:   TriggerSourceLinkHealth,
				Reason:   "test",
				Severity: 0,
			})
		}

		err := constraint.Check(current, target, enteredCurrentAt, lastUpgradeAt, triggers)

		// 验证 minimum_dwell_time
		minDwell := config.MinDwellTimes[current]
		elapsed := now.Sub(enteredCurrentAt)
		if minDwell > 0 && elapsed < minDwell {
			if err == nil {
				t.Fatalf("expected min_dwell_time violation for %s (elapsed=%v, min=%v)", current, elapsed, minDwell)
			}
			var cv *ErrConstraintViolation
			if !errors.As(err, &cv) {
				t.Fatalf("expected ErrConstraintViolation, got %T", err)
			}
			if cv.ConstraintType != "min_dwell_time" {
				t.Fatalf("expected constraint type min_dwell_time, got %s", cv.ConstraintType)
			}
			if cv.Remaining <= 0 {
				t.Fatal("expected positive remaining time")
			}
			return
		}

		// Policy trigger 绕过 cooldown 和 hysteresis
		if hasPolicyTrigger {
			if err != nil {
				t.Fatalf("policy trigger should bypass cooldown/hysteresis, got: %v", err)
			}
			return
		}

		// 升级迁移 cooldown 检查
		currentSev := ModeSeverity[current]
		targetSev := ModeSeverity[target]
		if targetSev > currentSev {
			cooldownElapsed := now.Sub(lastUpgradeAt)
			if cooldownElapsed < config.UpgradeCooldown {
				if err == nil {
					t.Fatal("expected cooldown violation")
				}
				var cv *ErrConstraintViolation
				if !errors.As(err, &cv) {
					t.Fatalf("expected ErrConstraintViolation, got %T", err)
				}
				if cv.ConstraintType != "cooldown" {
					t.Fatalf("expected constraint type cooldown, got %s", cv.ConstraintType)
				}
			}
		}

		// 如果有错误，验证错误类型
		if err != nil {
			var cv *ErrConstraintViolation
			if !errors.As(err, &cv) {
				t.Fatalf("expected ErrConstraintViolation, got %T", err)
			}
			validTypes := map[string]bool{"min_dwell_time": true, "cooldown": true, "hysteresis": true}
			if !validTypes[cv.ConstraintType] {
				t.Fatalf("unexpected constraint type: %s", cv.ConstraintType)
			}
		}
	})
}

// 验证 Policy trigger 始终绕过 cooldown 和 hysteresis
func TestProperty7_PolicyTriggerBypass(t *testing.T) {
	config := DefaultConstraintConfig
	constraint := NewTransitionConstraint(config)

	rapid.Check(t, func(t *rapid.T) {
		// 选择一个有足够 dwell time 的模式对
		current := orchestrator.SurvivalModeNormal // dwell time = 0
		target := genSurvivalMode(t)
		if current == target {
			return
		}

		triggers := []TriggerSignal{{
			Source:   TriggerSourcePolicy,
			Reason:   "policy override",
			Severity: ModeSeverity[target],
		}}

		err := constraint.Check(current, target, time.Now().Add(-1*time.Hour), time.Now(), triggers)
		if err != nil {
			t.Fatalf("policy trigger should bypass constraints from Normal, got: %v", err)
		}
	})
}
