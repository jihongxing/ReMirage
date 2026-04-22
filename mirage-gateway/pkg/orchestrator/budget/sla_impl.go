package budget

import "mirage-gateway/pkg/orchestrator"

// slaPolicies 三档服务等级策略常量表
var slaPolicies = map[orchestrator.ServiceClass]*SLAPolicy{
	orchestrator.ServiceClassStandard: {
		HardenedAllowed:    false,
		EscapeAllowed:      false,
		LastResortAllowed:  false,
		MaxSwitchPerHour:   5,
		MaxEntryBurnPerDay: 2,
	},
	orchestrator.ServiceClassPlatinum: {
		HardenedAllowed:    true,
		EscapeAllowed:      false,
		LastResortAllowed:  false,
		MaxSwitchPerHour:   15,
		MaxEntryBurnPerDay: 5,
	},
	orchestrator.ServiceClassDiamond: {
		HardenedAllowed:    true,
		EscapeAllowed:      true,
		LastResortAllowed:  true,
		MaxSwitchPerHour:   30,
		MaxEntryBurnPerDay: 10,
	},
}

// DefaultSLAPolicy 默认 SLA 策略实现
type DefaultSLAPolicy struct{}

// NewDefaultSLAPolicy 创建默认 SLA 策略实例
func NewDefaultSLAPolicy() *DefaultSLAPolicy {
	return &DefaultSLAPolicy{}
}

// GetPolicy 根据 ServiceClass 返回对应策略，未知 ServiceClass 返回 Standard 默认策略
func (d *DefaultSLAPolicy) GetPolicy(serviceClass orchestrator.ServiceClass) *SLAPolicy {
	if policy, ok := slaPolicies[serviceClass]; ok {
		return policy
	}
	return slaPolicies[orchestrator.ServiceClassStandard]
}
