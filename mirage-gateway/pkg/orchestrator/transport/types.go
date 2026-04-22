// Package transport - Transport Fabric 传输织网层
package transport

import orchestrator "mirage-gateway/pkg/orchestrator"

// SwitchAggressiveness 切换激进度（transport 包本地定义，避免循环依赖）
type SwitchAggressiveness string

const (
	AggressivenessConservative SwitchAggressiveness = "Conservative"
	AggressivenessModerate     SwitchAggressiveness = "Moderate"
	AggressivenessAggressive   SwitchAggressiveness = "Aggressive"
)

// PathScore 路径评分结果
type PathScore struct {
	LinkID string  `json:"link_id"`
	Score  float64 `json:"score"`
}

// TransportPolicy 传输策略
type TransportPolicy struct {
	SwitchAggressiveness  SwitchAggressiveness `json:"switch_aggressiveness"`
	SwitchThreshold       float64              `json:"switch_threshold"`
	PreferHighPerformance bool                 `json:"prefer_high_performance"`
	AllowDegradedPath     bool                 `json:"allow_degraded_path"`
	MaxParallelPaths      int                  `json:"max_parallel_paths"`
	PrewarmBackup         bool                 `json:"prewarm_backup"`
}

// DefaultTransportPolicies 按 Survival Mode 分级的默认传输策略
var DefaultTransportPolicies = map[orchestrator.SurvivalMode]*TransportPolicy{
	orchestrator.SurvivalModeNormal: {
		SwitchAggressiveness:  AggressivenessConservative,
		SwitchThreshold:       40.0,
		PreferHighPerformance: true,
		AllowDegradedPath:     false,
		MaxParallelPaths:      1,
		PrewarmBackup:         false,
	},
	orchestrator.SurvivalModeLowNoise: {
		SwitchAggressiveness:  AggressivenessConservative,
		SwitchThreshold:       40.0,
		PreferHighPerformance: true,
		AllowDegradedPath:     false,
		MaxParallelPaths:      1,
		PrewarmBackup:         false,
	},
	orchestrator.SurvivalModeHardened: {
		SwitchAggressiveness:  AggressivenessModerate,
		SwitchThreshold:       60.0,
		PreferHighPerformance: false,
		AllowDegradedPath:     false,
		MaxParallelPaths:      2,
		PrewarmBackup:         true,
	},
	orchestrator.SurvivalModeDegraded: {
		SwitchAggressiveness:  AggressivenessConservative,
		SwitchThreshold:       40.0,
		PreferHighPerformance: false,
		AllowDegradedPath:     true,
		MaxParallelPaths:      1,
		PrewarmBackup:         false,
	},
	orchestrator.SurvivalModeEscape: {
		SwitchAggressiveness:  AggressivenessAggressive,
		SwitchThreshold:       80.0,
		PreferHighPerformance: false,
		AllowDegradedPath:     true,
		MaxParallelPaths:      3,
		PrewarmBackup:         true,
	},
	orchestrator.SurvivalModeLastResort: {
		SwitchAggressiveness:  AggressivenessAggressive,
		SwitchThreshold:       80.0,
		PreferHighPerformance: false,
		AllowDegradedPath:     true,
		MaxParallelPaths:      2,
		PrewarmBackup:         false,
	},
}
