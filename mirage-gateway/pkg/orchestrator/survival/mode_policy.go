package survival

import orchestrator "mirage-gateway/pkg/orchestrator"

// TransportPolicyName 传输策略名称
type TransportPolicyName string

// PersonaPolicyName 人格策略名称
type PersonaPolicyName string

// BudgetPolicyName 预算策略名称
type BudgetPolicyName string

// ModePolicy 模式策略绑定
type ModePolicy struct {
	TransportPolicy        TransportPolicyName    `json:"transport_policy"`
	PersonaPolicy          PersonaPolicyName      `json:"persona_policy"`
	BudgetPolicy           BudgetPolicyName       `json:"budget_policy"`
	SwitchAggressiveness   SwitchAggressiveness   `json:"switch_aggressiveness"`
	SessionAdmissionPolicy SessionAdmissionPolicy `json:"session_admission_policy"`
}

// DefaultModePolicies 默认模式策略表
var DefaultModePolicies = map[orchestrator.SurvivalMode]*ModePolicy{
	orchestrator.SurvivalModeNormal: {
		TransportPolicy:        "normal",
		PersonaPolicy:          "normal",
		BudgetPolicy:           "normal",
		SwitchAggressiveness:   AggressivenessConservative,
		SessionAdmissionPolicy: AdmissionOpen,
	},
	orchestrator.SurvivalModeLowNoise: {
		TransportPolicy:        "low_noise",
		PersonaPolicy:          "low_noise",
		BudgetPolicy:           "conservative",
		SwitchAggressiveness:   AggressivenessConservative,
		SessionAdmissionPolicy: AdmissionOpen,
	},
	orchestrator.SurvivalModeHardened: {
		TransportPolicy:        "hardened",
		PersonaPolicy:          "hardened",
		BudgetPolicy:           "elevated",
		SwitchAggressiveness:   AggressivenessModerate,
		SessionAdmissionPolicy: AdmissionOpen,
	},
	orchestrator.SurvivalModeDegraded: {
		TransportPolicy:        "degraded",
		PersonaPolicy:          "degraded",
		BudgetPolicy:           "restricted",
		SwitchAggressiveness:   AggressivenessConservative,
		SessionAdmissionPolicy: AdmissionRestrictNew,
	},
	orchestrator.SurvivalModeEscape: {
		TransportPolicy:        "escape",
		PersonaPolicy:          "escape",
		BudgetPolicy:           "emergency",
		SwitchAggressiveness:   AggressivenessAggressive,
		SessionAdmissionPolicy: AdmissionHighPriorityOnly,
	},
	orchestrator.SurvivalModeLastResort: {
		TransportPolicy:        "last_resort",
		PersonaPolicy:          "last_resort",
		BudgetPolicy:           "emergency",
		SwitchAggressiveness:   AggressivenessAggressive,
		SessionAdmissionPolicy: AdmissionClosed,
	},
}
