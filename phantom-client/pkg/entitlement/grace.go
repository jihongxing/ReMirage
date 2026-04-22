package entitlement

import (
	"sync/atomic"
	"time"
)

// OfflineScenario 离线场景类型
type OfflineScenario int

const (
	ScenarioControlPlaneLost    OfflineScenario = iota // 控制面失联
	ScenarioQuotaExhausted                             // 配额耗尽
	ScenarioSubscriptionExpired                        // 订阅到期
	ScenarioAccountBanned                              // 账号停用
)

const defaultGraceDuration = 24 * time.Hour

// GraceWindow 离线宽限管理
type GraceWindow struct {
	duration      time.Duration
	lastSuccessAt atomic.Int64 // Unix nano timestamp
	cachedState   atomic.Pointer[Entitlement]
}

// NewGraceWindow creates a GraceWindow with the given duration. Defaults to 24h if duration <= 0.
func NewGraceWindow(duration time.Duration) *GraceWindow {
	if duration <= 0 {
		duration = defaultGraceDuration
	}
	return &GraceWindow{duration: duration}
}

// RecordSuccess records a successful entitlement fetch and caches the state.
func (gw *GraceWindow) RecordSuccess(ent *Entitlement) {
	gw.lastSuccessAt.Store(time.Now().UnixNano())
	if ent != nil {
		cp := *ent
		gw.cachedState.Store(&cp)
	}
}

// IsWithinGrace returns true if now - lastSuccessAt < duration.
// Returns false if no success has ever been recorded.
func (gw *GraceWindow) IsWithinGrace() bool {
	ts := gw.lastSuccessAt.Load()
	if ts == 0 {
		return false
	}
	lastSuccess := time.Unix(0, ts)
	return time.Since(lastSuccess) < gw.duration
}

// IsWithinGraceAt returns true if refTime - lastSuccessAt < duration.
// Useful for deterministic testing.
func (gw *GraceWindow) IsWithinGraceAt(refTime time.Time) bool {
	ts := gw.lastSuccessAt.Load()
	if ts == 0 {
		return false
	}
	lastSuccess := time.Unix(0, ts)
	return refTime.Sub(lastSuccess) < gw.duration
}

// DetermineScenario classifies the offline scenario based on entitlement state and control plane reachability.
// Priority: banned > expired > quotaExhausted > controlPlaneLost
func (gw *GraceWindow) DetermineScenario(ent *Entitlement, controlPlaneReachable bool) OfflineScenario {
	if ent != nil {
		if ent.Banned {
			return ScenarioAccountBanned
		}
		if !ent.ExpiresAt.IsZero() && time.Now().After(ent.ExpiresAt) {
			return ScenarioSubscriptionExpired
		}
		if ent.QuotaRemaining <= 0 {
			return ScenarioQuotaExhausted
		}
	}
	if !controlPlaneReachable {
		return ScenarioControlPlaneLost
	}
	// Default: control plane lost (shouldn't normally reach here if called correctly)
	return ScenarioControlPlaneLost
}

// DetermineScenarioAt is like DetermineScenario but uses refTime instead of time.Now().
func (gw *GraceWindow) DetermineScenarioAt(ent *Entitlement, controlPlaneReachable bool, refTime time.Time) OfflineScenario {
	if ent != nil {
		if ent.Banned {
			return ScenarioAccountBanned
		}
		if !ent.ExpiresAt.IsZero() && refTime.After(ent.ExpiresAt) {
			return ScenarioSubscriptionExpired
		}
		if ent.QuotaRemaining <= 0 {
			return ScenarioQuotaExhausted
		}
	}
	if !controlPlaneReachable {
		return ScenarioControlPlaneLost
	}
	return ScenarioControlPlaneLost
}

// CachedEntitlement returns the cached entitlement state from the last successful fetch.
func (gw *GraceWindow) CachedEntitlement() *Entitlement {
	return gw.cachedState.Load()
}
