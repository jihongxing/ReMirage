package gswitch

import (
	"sync"
	"time"
)

type gswitchAdapter struct {
	mu      sync.RWMutex
	manager *GSwitchManager
	// burnEvents 记录域名战死事件时间戳
	burnEvents []time.Time
}

// NewGSwitchAdapter 创建 GSwitchAdapter
func NewGSwitchAdapter(manager *GSwitchManager) GSwitchAdapter {
	adapter := &gswitchAdapter{
		manager:    manager,
		burnEvents: make([]time.Time, 0),
	}

	// 注册域名战死回调
	manager.SetBurnedCallback(func(domain *Domain) {
		adapter.mu.Lock()
		adapter.burnEvents = append(adapter.burnEvents, time.Now())
		adapter.mu.Unlock()
	})

	return adapter
}

func (a *gswitchAdapter) GetEntryBurnCount(window time.Duration) int {
	a.mu.RLock()
	defer a.mu.RUnlock()

	cutoff := time.Now().Add(-window)
	count := 0
	for _, t := range a.burnEvents {
		if t.After(cutoff) {
			count++
		}
	}
	return count
}

func (a *gswitchAdapter) GetPoolStats() map[string]int {
	return a.manager.GetPoolStats()
}

func (a *gswitchAdapter) TriggerEscape(reason string) error {
	return a.manager.TriggerEscape(reason)
}

func (a *gswitchAdapter) OnDomainBurned(callback func(domain string, reason string)) {
	a.manager.SetBurnedCallback(func(domain *Domain) {
		a.mu.Lock()
		a.burnEvents = append(a.burnEvents, time.Now())
		a.mu.Unlock()

		if callback != nil {
			callback(domain.Name, domain.BurnReason)
		}
	})
}

func (a *gswitchAdapter) IsStandbyPoolEmpty() bool {
	stats := a.manager.GetPoolStats()
	return stats["standby"] == 0
}
