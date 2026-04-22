// Package survival - V2 Survival Orchestrator 生存编排器
package survival

import "mirage-gateway/pkg/orchestrator"

// SwitchAggressiveness 切换激进度
type SwitchAggressiveness string

const (
	AggressivenessConservative SwitchAggressiveness = "Conservative"
	AggressivenessModerate     SwitchAggressiveness = "Moderate"
	AggressivenessAggressive   SwitchAggressiveness = "Aggressive"
)

// AllSwitchAggressiveness 所有合法值
var AllSwitchAggressiveness = []SwitchAggressiveness{
	AggressivenessConservative, AggressivenessModerate, AggressivenessAggressive,
}

// SessionAdmissionPolicy 会话准入策略
type SessionAdmissionPolicy string

const (
	AdmissionOpen             SessionAdmissionPolicy = "Open"
	AdmissionRestrictNew      SessionAdmissionPolicy = "RestrictNew"
	AdmissionHighPriorityOnly SessionAdmissionPolicy = "HighPriorityOnly"
	AdmissionClosed           SessionAdmissionPolicy = "Closed"
)

// AllSessionAdmissionPolicies 所有合法值
var AllSessionAdmissionPolicies = []SessionAdmissionPolicy{
	AdmissionOpen, AdmissionRestrictNew, AdmissionHighPriorityOnly, AdmissionClosed,
}

// TriggerSource 触发源类型
type TriggerSource string

const (
	TriggerSourceLinkHealth TriggerSource = "LinkHealth"
	TriggerSourceEntryBurn  TriggerSource = "EntryBurn"
	TriggerSourceBudget     TriggerSource = "Budget"
	TriggerSourcePolicy     TriggerSource = "Policy"
)

// AllTriggerSources 所有合法值
var AllTriggerSources = []TriggerSource{
	TriggerSourceLinkHealth, TriggerSourceEntryBurn, TriggerSourceBudget, TriggerSourcePolicy,
}

// AllSurvivalModes 所有合法 SurvivalMode 值
var AllSurvivalModes = []orchestrator.SurvivalMode{
	orchestrator.SurvivalModeNormal,
	orchestrator.SurvivalModeLowNoise,
	orchestrator.SurvivalModeHardened,
	orchestrator.SurvivalModeDegraded,
	orchestrator.SurvivalModeEscape,
	orchestrator.SurvivalModeLastResort,
}

// ValidTransitions 合法的 Survival Mode 迁移路径（6 key, 15 条路径）
var ValidTransitions = map[orchestrator.SurvivalMode][]orchestrator.SurvivalMode{
	orchestrator.SurvivalModeNormal:     {orchestrator.SurvivalModeLowNoise, orchestrator.SurvivalModeHardened, orchestrator.SurvivalModeDegraded},
	orchestrator.SurvivalModeLowNoise:   {orchestrator.SurvivalModeNormal, orchestrator.SurvivalModeHardened, orchestrator.SurvivalModeDegraded},
	orchestrator.SurvivalModeHardened:   {orchestrator.SurvivalModeNormal, orchestrator.SurvivalModeDegraded, orchestrator.SurvivalModeEscape},
	orchestrator.SurvivalModeDegraded:   {orchestrator.SurvivalModeNormal, orchestrator.SurvivalModeEscape},
	orchestrator.SurvivalModeEscape:     {orchestrator.SurvivalModeNormal, orchestrator.SurvivalModeHardened, orchestrator.SurvivalModeLastResort},
	orchestrator.SurvivalModeLastResort: {orchestrator.SurvivalModeNormal, orchestrator.SurvivalModeEscape},
}

// ModeSeverity 模式严重度排序（数值越大越严重）
var ModeSeverity = map[orchestrator.SurvivalMode]int{
	orchestrator.SurvivalModeNormal:     0,
	orchestrator.SurvivalModeLowNoise:   1,
	orchestrator.SurvivalModeHardened:   2,
	orchestrator.SurvivalModeDegraded:   3,
	orchestrator.SurvivalModeEscape:     4,
	orchestrator.SurvivalModeLastResort: 5,
}

// SeverityToMode 严重度到模式的反向映射
var SeverityToMode = map[int]orchestrator.SurvivalMode{
	0: orchestrator.SurvivalModeNormal,
	1: orchestrator.SurvivalModeLowNoise,
	2: orchestrator.SurvivalModeHardened,
	3: orchestrator.SurvivalModeDegraded,
	4: orchestrator.SurvivalModeEscape,
	5: orchestrator.SurvivalModeLastResort,
}
