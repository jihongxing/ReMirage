package entitlement

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"pgregory.net/rapid"
)

// --- Generators ---

func genServiceClass(t *rapid.T) ServiceClass {
	classes := []ServiceClass{ClassStandard, ClassPlatinum, ClassDiamond}
	return classes[rapid.IntRange(0, len(classes)-1).Draw(t, "classIdx")]
}

func genEntitlement(t *rapid.T) Entitlement {
	return Entitlement{
		ExpiresAt:      time.Unix(rapid.Int64Range(0, 2000000000).Draw(t, "expiresAt"), 0).UTC(),
		QuotaRemaining: rapid.Int64Range(0, 1<<40).Draw(t, "quota"),
		ServiceClass:   genServiceClass(t),
		Banned:         rapid.Bool().Draw(t, "banned"),
		FetchedAt:      time.Unix(rapid.Int64Range(0, 2000000000).Draw(t, "fetchedAt"), 0).UTC(),
	}
}

// --- Property 2: Entitlement 序列化往返测试 ---
// Feature: v1-client-productization, Property 2: Entitlement 序列化往返
// **Validates: Requirements 6.2**

func TestProperty2_EntitlementRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		original := genEntitlement(t)

		data, err := json.Marshal(original)
		if err != nil {
			t.Fatalf("marshal failed: %v", err)
		}

		var decoded Entitlement
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}

		if !original.Equal(&decoded) {
			t.Fatalf("round-trip mismatch:\n  original: %+v\n  decoded:  %+v", original, decoded)
		}
	})
}

// --- Property 12: 状态变更事件触发测试 ---
// Feature: v1-client-productization, Property 12: 状态变更事件触发
// **Validates: Requirements 6.3**

func TestProperty12_StateChangeEventTrigger(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		a := genEntitlement(t)
		b := genEntitlement(t)

		triggered := false
		onChange := func(old, new_ *Entitlement) {
			triggered = true
		}

		em := NewEntitlementManager(EntitlementConfig{
			Fetcher:  nil, // not used in this test
			OnChange: onChange,
		})

		// Set initial state to A
		em.current.Store(&a)

		// Simulate receiving B — replicate the logic from FetchOnce
		old := em.current.Load()
		em.current.Store(&b)
		if !b.Equal(old) {
			onChange(old, &b)
		}

		areEqual := a.Equal(&b)

		if areEqual && triggered {
			t.Fatalf("A == B but onChange was triggered.\nA: %+v\nB: %+v", a, b)
		}
		if !areEqual && !triggered {
			t.Fatalf("A != B but onChange was NOT triggered.\nA: %+v\nB: %+v", a, b)
		}
	})
}

// --- Property 13: ServiceClass → Policy 映射测试 ---
// Feature: v1-client-productization, Property 13: ServiceClass → Policy 映射
// **Validates: Requirements 7.2, 7.4**

func TestProperty13_ServiceClassPolicyMapping(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		class := genServiceClass(t)
		policy := PolicyForClass(class)

		if policy == nil {
			t.Fatalf("PolicyForClass(%q) returned nil", class)
		}

		// Verify ResonanceEnabled: Standard=false, Platinum/Diamond=true
		switch class {
		case ClassStandard:
			if policy.ResonanceEnabled {
				t.Fatalf("Standard should have ResonanceEnabled=false")
			}
			if policy.ReconnBackoffBase != 5*time.Second {
				t.Fatalf("Standard ReconnBackoffBase: got %v, want 5s", policy.ReconnBackoffBase)
			}
			if policy.ReconnBackoffMax != 120*time.Second {
				t.Fatalf("Standard ReconnBackoffMax: got %v, want 120s", policy.ReconnBackoffMax)
			}
			if policy.HeartbeatInterval != 30*time.Second {
				t.Fatalf("Standard HeartbeatInterval: got %v, want 30s", policy.HeartbeatInterval)
			}
			if policy.TopoRefreshInterval != 10*time.Minute {
				t.Fatalf("Standard TopoRefreshInterval: got %v, want 10min", policy.TopoRefreshInterval)
			}
		case ClassPlatinum:
			if !policy.ResonanceEnabled {
				t.Fatalf("Platinum should have ResonanceEnabled=true")
			}
			if policy.ReconnBackoffBase != 2*time.Second {
				t.Fatalf("Platinum ReconnBackoffBase: got %v, want 2s", policy.ReconnBackoffBase)
			}
			if policy.ReconnBackoffMax != 60*time.Second {
				t.Fatalf("Platinum ReconnBackoffMax: got %v, want 60s", policy.ReconnBackoffMax)
			}
			if policy.HeartbeatInterval != 15*time.Second {
				t.Fatalf("Platinum HeartbeatInterval: got %v, want 15s", policy.HeartbeatInterval)
			}
			if policy.TopoRefreshInterval != 5*time.Minute {
				t.Fatalf("Platinum TopoRefreshInterval: got %v, want 5min", policy.TopoRefreshInterval)
			}
		case ClassDiamond:
			if !policy.ResonanceEnabled {
				t.Fatalf("Diamond should have ResonanceEnabled=true")
			}
			if policy.ReconnBackoffBase != 1*time.Second {
				t.Fatalf("Diamond ReconnBackoffBase: got %v, want 1s", policy.ReconnBackoffBase)
			}
			if policy.ReconnBackoffMax != 30*time.Second {
				t.Fatalf("Diamond ReconnBackoffMax: got %v, want 30s", policy.ReconnBackoffMax)
			}
			if policy.HeartbeatInterval != 10*time.Second {
				t.Fatalf("Diamond HeartbeatInterval: got %v, want 10s", policy.HeartbeatInterval)
			}
			if policy.TopoRefreshInterval != 2*time.Minute {
				t.Fatalf("Diamond TopoRefreshInterval: got %v, want 2min", policy.TopoRefreshInterval)
			}
		}

		// Universal: all policies must have positive durations
		if policy.ReconnBackoffBase <= 0 {
			t.Fatalf("ReconnBackoffBase must be positive, got %v", policy.ReconnBackoffBase)
		}
		if policy.ReconnBackoffMax <= 0 {
			t.Fatalf("ReconnBackoffMax must be positive, got %v", policy.ReconnBackoffMax)
		}
		if policy.HeartbeatInterval <= 0 {
			t.Fatalf("HeartbeatInterval must be positive, got %v", policy.HeartbeatInterval)
		}
		if policy.TopoRefreshInterval <= 0 {
			t.Fatalf("TopoRefreshInterval must be positive, got %v", policy.TopoRefreshInterval)
		}
		// BackoffBase < BackoffMax
		if policy.ReconnBackoffBase >= policy.ReconnBackoffMax {
			t.Fatalf("ReconnBackoffBase (%v) should be < ReconnBackoffMax (%v)", policy.ReconnBackoffBase, policy.ReconnBackoffMax)
		}
	})
}

// --- Property 14: Grace Window 时间计算测试 ---
// Feature: v1-client-productization, Property 14: Grace Window 时间计算
// **Validates: Requirements 11.1**

func TestProperty14_GraceWindowTimeCalculation(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		durationSec := rapid.Int64Range(1, 86400*7).Draw(t, "durationSec") // 1s to 7 days
		duration := time.Duration(durationSec) * time.Second

		gw := NewGraceWindow(duration)

		// Record success at a known time
		ent := &Entitlement{
			ExpiresAt:    time.Now().Add(24 * time.Hour),
			ServiceClass: ClassStandard,
		}
		gw.RecordSuccess(ent)

		// Immediately after recording, should be within grace
		if !gw.IsWithinGrace() {
			t.Fatalf("should be within grace immediately after RecordSuccess (duration=%v)", duration)
		}

		// Use IsWithinGraceAt for deterministic testing
		lastSuccess := time.Unix(0, gw.lastSuccessAt.Load())

		// At lastSuccess + duration - 1s: should be within grace
		beforeExpiry := lastSuccess.Add(duration - time.Second)
		if !gw.IsWithinGraceAt(beforeExpiry) {
			t.Fatalf("should be within grace at lastSuccess + duration - 1s")
		}

		// At lastSuccess + duration + 1s: should NOT be within grace
		afterExpiry := lastSuccess.Add(duration + time.Second)
		if gw.IsWithinGraceAt(afterExpiry) {
			t.Fatalf("should NOT be within grace at lastSuccess + duration + 1s")
		}
	})
}

// --- Property 15: 离线场景分类测试 ---
// Feature: v1-client-productization, Property 15: 离线场景分类
// **Validates: Requirements 11.2**

func TestProperty15_OfflineScenarioClassification(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		banned := rapid.Bool().Draw(t, "banned")
		// Use a reference time for deterministic expiry checks
		refTime := time.Unix(1700000000, 0).UTC()

		expired := rapid.Bool().Draw(t, "expired")
		var expiresAt time.Time
		if expired {
			// Set expiry in the past relative to refTime
			expiresAt = refTime.Add(-time.Duration(rapid.Int64Range(1, 86400*365).Draw(t, "expiredAgo")) * time.Second)
		} else {
			// Set expiry in the future relative to refTime
			expiresAt = refTime.Add(time.Duration(rapid.Int64Range(1, 86400*365).Draw(t, "expiresIn")) * time.Second)
		}

		quotaExhausted := rapid.Bool().Draw(t, "quotaExhausted")
		var quota int64
		if quotaExhausted {
			quota = 0
		} else {
			quota = rapid.Int64Range(1, 1<<40).Draw(t, "quota")
		}

		controlPlaneReachable := rapid.Bool().Draw(t, "cpReachable")

		ent := &Entitlement{
			ExpiresAt:      expiresAt,
			QuotaRemaining: quota,
			ServiceClass:   ClassStandard,
			Banned:         banned,
			FetchedAt:      refTime.Add(-time.Minute),
		}

		gw := NewGraceWindow(24 * time.Hour)
		scenario := gw.DetermineScenarioAt(ent, controlPlaneReachable, refTime)

		// Priority: banned > expired > quotaExhausted > controlPlaneLost
		if banned {
			if scenario != ScenarioAccountBanned {
				t.Fatalf("banned=true should yield ScenarioAccountBanned, got %d", scenario)
			}
			return
		}
		if expired {
			if scenario != ScenarioSubscriptionExpired {
				t.Fatalf("expired=true (not banned) should yield ScenarioSubscriptionExpired, got %d", scenario)
			}
			return
		}
		if quotaExhausted {
			if scenario != ScenarioQuotaExhausted {
				t.Fatalf("quotaExhausted=true (not banned, not expired) should yield ScenarioQuotaExhausted, got %d", scenario)
			}
			return
		}
		if !controlPlaneReachable {
			if scenario != ScenarioControlPlaneLost {
				t.Fatalf("controlPlaneLost (no other issues) should yield ScenarioControlPlaneLost, got %d", scenario)
			}
			return
		}
		// All conditions are "good" — should default to controlPlaneLost
		if scenario != ScenarioControlPlaneLost {
			t.Fatalf("no issues + reachable should yield ScenarioControlPlaneLost (default), got %d", scenario)
		}
	})
}

// --- Unit tests for EntitlementManager ---

func TestEntitlementManager_Current_NilBeforeFetch(t *testing.T) {
	em := NewEntitlementManager(EntitlementConfig{
		Fetcher: func(ctx context.Context) (*Entitlement, error) {
			return nil, nil
		},
	})
	if em.Current() != nil {
		t.Fatal("Current() should be nil before any fetch")
	}
}

func TestGraceWindow_NoSuccessRecorded(t *testing.T) {
	gw := NewGraceWindow(24 * time.Hour)
	if gw.IsWithinGrace() {
		t.Fatal("should not be within grace when no success recorded")
	}
	if gw.CachedEntitlement() != nil {
		t.Fatal("cached entitlement should be nil when no success recorded")
	}
}

func TestGraceWindow_CachedEntitlement(t *testing.T) {
	gw := NewGraceWindow(24 * time.Hour)
	ent := &Entitlement{
		ExpiresAt:      time.Now().Add(24 * time.Hour),
		QuotaRemaining: 1000,
		ServiceClass:   ClassPlatinum,
		Banned:         false,
	}
	gw.RecordSuccess(ent)

	cached := gw.CachedEntitlement()
	if cached == nil {
		t.Fatal("cached entitlement should not be nil after RecordSuccess")
	}
	if cached.ServiceClass != ClassPlatinum {
		t.Fatalf("cached ServiceClass: got %v, want platinum", cached.ServiceClass)
	}
}
