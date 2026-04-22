package gswitch

import "time"

// GSwitchAdapter G-Switch 适配层接口
type GSwitchAdapter interface {
	GetEntryBurnCount(window time.Duration) int
	GetPoolStats() map[string]int
	TriggerEscape(reason string) error
	OnDomainBurned(callback func(domain string, reason string))
	IsStandbyPoolEmpty() bool
}
