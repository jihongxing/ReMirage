package budget

import "mirage-gateway/pkg/orchestrator"

// SLAPolicy 服务等级策略
type SLAPolicy struct {
	HardenedAllowed    bool `json:"hardened_allowed"`
	EscapeAllowed      bool `json:"escape_allowed"`
	LastResortAllowed  bool `json:"last_resort_allowed"`
	MaxSwitchPerHour   int  `json:"max_switch_per_hour"`
	MaxEntryBurnPerDay int  `json:"max_entry_burn_per_day"`
}

// ExternalSLAPolicy 外部服务等级策略管理
type ExternalSLAPolicy interface {
	// GetPolicy 根据 ServiceClass 返回对应策略，不存在则返回 Standard 默认策略
	GetPolicy(serviceClass orchestrator.ServiceClass) *SLAPolicy
}
