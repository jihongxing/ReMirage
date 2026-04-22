package survival

import (
	"time"

	orchestrator "mirage-gateway/pkg/orchestrator"
)

// TransitionConstraintConfig 迁移约束配置
type TransitionConstraintConfig struct {
	MinDwellTimes    map[orchestrator.SurvivalMode]time.Duration `json:"min_dwell_times"`
	UpgradeCooldown  time.Duration                               `json:"upgrade_cooldown"`
	HysteresisMargin float64                                     `json:"hysteresis_margin"`
}

// DefaultConstraintConfig 默认约束配置
var DefaultConstraintConfig = TransitionConstraintConfig{
	MinDwellTimes: map[orchestrator.SurvivalMode]time.Duration{
		orchestrator.SurvivalModeNormal:     0,
		orchestrator.SurvivalModeLowNoise:   30 * time.Second,
		orchestrator.SurvivalModeHardened:   60 * time.Second,
		orchestrator.SurvivalModeDegraded:   120 * time.Second,
		orchestrator.SurvivalModeEscape:     30 * time.Second,
		orchestrator.SurvivalModeLastResort: 60 * time.Second,
	},
	UpgradeCooldown:  60 * time.Second,
	HysteresisMargin: 0.20,
}

// TransitionConstraintIface 迁移约束检查器接口
type TransitionConstraintIface interface {
	Check(current orchestrator.SurvivalMode, target orchestrator.SurvivalMode,
		enteredCurrentAt time.Time, lastUpgradeAt time.Time,
		triggers []TriggerSignal) error
}
