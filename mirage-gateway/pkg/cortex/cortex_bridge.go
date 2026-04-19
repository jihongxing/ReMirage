// Cortex 桥接 - 用户威胁事件聚合
package cortex

import (
	"sync"
	"time"
)

type CortexBridge struct {
	mu           sync.RWMutex
	userEvents   map[string][]*UserThreatEvent
	maxEvents    int
	eventTTL     time.Duration
}

type UserThreatEvent struct {
	ID          string    `json:"id"`
	UID         string    `json:"uid"`
	Timestamp   time.Time `json:"timestamp"`
	Type        string    `json:"type"`
	Severity    int       `json:"severity"`
	SourceIP    string    `json:"source_ip"`
	SourceGeo   GeoInfo   `json:"source_geo"`
	Action      string    `json:"action"`
	Details     string    `json:"details"`
	Mitigated   bool      `json:"mitigated"`
}

type GeoInfo struct {
	Country string  `json:"country"`
	City    string  `json:"city"`
	Lat     float64 `json:"lat"`
	Lng     float64 `json:"lng"`
}

type UserThreatSummary struct {
	UID              string `json:"uid"`
	TotalEvents      int    `json:"total_events"`
	HighSeverity     int    `json:"high_severity"`
	MediumSeverity   int    `json:"medium_severity"`
	LowSeverity      int    `json:"low_severity"`
	MitigatedCount   int    `json:"mitigated_count"`
	TopAttackTypes   map[string]int `json:"top_attack_types"`
	TopSourceCountries map[string]int `json:"top_source_countries"`
}

func NewCortexBridge(maxEvents int, eventTTL time.Duration) *CortexBridge {
	return &CortexBridge{
		userEvents: make(map[string][]*UserThreatEvent),
		maxEvents:  maxEvents,
		eventTTL:   eventTTL,
	}
}

// RecordEvent 记录用户威胁事件
func (b *CortexBridge) RecordEvent(event *UserThreatEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()
	
	events := b.userEvents[event.UID]
	events = append(events, event)
	
	// 限制数量
	if len(events) > b.maxEvents {
		events = events[len(events)-b.maxEvents:]
	}
	
	b.userEvents[event.UID] = events
}

// GetUserEvents 获取用户威胁事件
func (b *CortexBridge) GetUserEvents(uid string, limit int) []*UserThreatEvent {
	b.mu.RLock()
	defer b.mu.RUnlock()
	
	events := b.userEvents[uid]
	if events == nil {
		return nil
	}
	
	// 过滤过期事件
	cutoff := time.Now().Add(-b.eventTTL)
	filtered := make([]*UserThreatEvent, 0)
	for _, e := range events {
		if e.Timestamp.After(cutoff) {
			filtered = append(filtered, e)
		}
	}
	
	// 限制返回数量
	if limit > 0 && len(filtered) > limit {
		filtered = filtered[len(filtered)-limit:]
	}
	
	return filtered
}

// GetUserSummary 获取用户威胁摘要
func (b *CortexBridge) GetUserSummary(uid string) *UserThreatSummary {
	events := b.GetUserEvents(uid, 0)
	if len(events) == 0 {
		return nil
	}
	
	summary := &UserThreatSummary{
		UID:                uid,
		TopAttackTypes:     make(map[string]int),
		TopSourceCountries: make(map[string]int),
	}
	
	for _, e := range events {
		summary.TotalEvents++
		
		if e.Severity >= 7 {
			summary.HighSeverity++
		} else if e.Severity >= 4 {
			summary.MediumSeverity++
		} else {
			summary.LowSeverity++
		}
		
		if e.Mitigated {
			summary.MitigatedCount++
		}
		
		summary.TopAttackTypes[e.Type]++
		summary.TopSourceCountries[e.SourceGeo.Country]++
	}
	
	return summary
}

// CleanupExpired 清理过期事件
func (b *CortexBridge) CleanupExpired() {
	b.mu.Lock()
	defer b.mu.Unlock()
	
	cutoff := time.Now().Add(-b.eventTTL)
	
	for uid, events := range b.userEvents {
		filtered := make([]*UserThreatEvent, 0)
		for _, e := range events {
			if e.Timestamp.After(cutoff) {
				filtered = append(filtered, e)
			}
		}
		
		if len(filtered) == 0 {
			delete(b.userEvents, uid)
		} else {
			b.userEvents[uid] = filtered
		}
	}
}
