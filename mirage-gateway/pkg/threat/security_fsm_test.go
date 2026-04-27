package threat

import (
	"sync"
	"testing"
	"time"
)

// --- 8.7 安全状态机单元测试 ---

func TestFSM_NormalToAlert_HighThreat(t *testing.T) {
	policy := NewIngressPolicy([]PolicyRule{
		{Condition: "rate_exceeded", Action: ActionThrottle, Params: map[string]int{"pps": 50}, Priority: 50},
	})
	var mu sync.Mutex
	var lastState SecurityState = -1
	fsm := NewSecurityFSM(policy, func(s SecurityState) {
		mu.Lock()
		lastState = s
		mu.Unlock()
	})

	// ThreatLevel High → Alert
	fsm.Evaluate(&SecurityMetrics{ThreatLevel: LevelHigh})

	if fsm.CurrentState() != StateAlert {
		t.Fatalf("期望 Alert，实际: %d", fsm.CurrentState())
	}
	// 等回调 goroutine
	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	got := lastState
	mu.Unlock()
	if got != StateAlert {
		t.Fatalf("回调状态期望 Alert，实际: %d", got)
	}
}

func TestFSM_NormalToHighPressure_CriticalThreat(t *testing.T) {
	policy := NewIngressPolicy(nil)
	fsm := NewSecurityFSM(policy, nil)

	fsm.Evaluate(&SecurityMetrics{ThreatLevel: LevelCritical})
	if fsm.CurrentState() != StateHighPressure {
		t.Fatalf("期望 HighPressure，实际: %d", fsm.CurrentState())
	}
}

func TestFSM_NormalToIsolated_ExtremeThreat(t *testing.T) {
	policy := NewIngressPolicy(nil)
	fsm := NewSecurityFSM(policy, nil)

	fsm.Evaluate(&SecurityMetrics{ThreatLevel: LevelExtreme})
	if fsm.CurrentState() != StateIsolated {
		t.Fatalf("期望 Isolated，实际: %d", fsm.CurrentState())
	}
}

func TestFSM_NormalToSilent_ControlPlaneDown(t *testing.T) {
	policy := NewIngressPolicy(nil)
	fsm := NewSecurityFSM(policy, nil)

	fsm.Evaluate(&SecurityMetrics{ControlPlaneDown: true})
	if fsm.CurrentState() != StateSilent {
		t.Fatalf("期望 Silent，实际: %d", fsm.CurrentState())
	}
}
func TestFSM_RejectRate_Triggers(t *testing.T) {
	policy := NewIngressPolicy(nil)
	fsm := NewSecurityFSM(policy, nil)

	// RejectRate > 0.2 → Alert
	fsm.Evaluate(&SecurityMetrics{RejectRate: 0.3})
	if fsm.CurrentState() != StateAlert {
		t.Fatalf("RejectRate 0.3 期望 Alert，实际: %d", fsm.CurrentState())
	}

	// RejectRate > 0.5 → HighPressure（升级立即执行）
	fsm.Evaluate(&SecurityMetrics{RejectRate: 0.6})
	if fsm.CurrentState() != StateHighPressure {
		t.Fatalf("RejectRate 0.6 期望 HighPressure，实际: %d", fsm.CurrentState())
	}

	// RejectRate > 0.8 → Isolated（升级立即执行）
	fsm.Evaluate(&SecurityMetrics{RejectRate: 0.9})
	if fsm.CurrentState() != StateIsolated {
		t.Fatalf("RejectRate 0.9 期望 Isolated，实际: %d", fsm.CurrentState())
	}
}

func TestFSM_CooldownPreventsDowngrade(t *testing.T) {
	policy := NewIngressPolicy(nil)
	fsm := NewSecurityFSM(policy, nil)

	// 固定时间源
	now := time.Now()
	var mu sync.Mutex
	fsm.nowFunc = func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		return now
	}

	// 升级到 HighPressure
	fsm.Evaluate(&SecurityMetrics{ThreatLevel: LevelCritical})
	if fsm.CurrentState() != StateHighPressure {
		t.Fatalf("期望 HighPressure，实际: %d", fsm.CurrentState())
	}

	// 立即尝试降级 → 应被冷却期阻止
	fsm.Evaluate(&SecurityMetrics{ThreatLevel: LevelLow})
	if fsm.CurrentState() != StateHighPressure {
		t.Fatalf("冷却期内不应降级，期望 HighPressure，实际: %d", fsm.CurrentState())
	}

	// 前进 200 秒 → 仍在冷却期内
	mu.Lock()
	now = now.Add(200 * time.Second)
	mu.Unlock()
	fsm.Evaluate(&SecurityMetrics{ThreatLevel: LevelLow})
	if fsm.CurrentState() != StateHighPressure {
		t.Fatalf("200s 后仍在冷却期，期望 HighPressure，实际: %d", fsm.CurrentState())
	}

	// 前进到 301 秒 → 冷却期结束，允许降级
	mu.Lock()
	now = now.Add(101 * time.Second)
	mu.Unlock()
	fsm.Evaluate(&SecurityMetrics{ThreatLevel: LevelLow})
	if fsm.CurrentState() != StateNormal {
		t.Fatalf("冷却期后应降级到 Normal，实际: %d", fsm.CurrentState())
	}
}

func TestFSM_UpgradeBypassesCooldown(t *testing.T) {
	policy := NewIngressPolicy(nil)
	fsm := NewSecurityFSM(policy, nil)

	// 升级到 Alert
	fsm.Evaluate(&SecurityMetrics{ThreatLevel: LevelHigh})
	if fsm.CurrentState() != StateAlert {
		t.Fatalf("期望 Alert，实际: %d", fsm.CurrentState())
	}

	// 立即再升级到 HighPressure → 应立即执行（不受冷却期影响）
	fsm.Evaluate(&SecurityMetrics{ThreatLevel: LevelCritical})
	if fsm.CurrentState() != StateHighPressure {
		t.Fatalf("升级应立即执行，期望 HighPressure，实际: %d", fsm.CurrentState())
	}
}

func TestFSM_ForceState_BypassesCooldown(t *testing.T) {
	policy := NewIngressPolicy(nil)
	fsm := NewSecurityFSM(policy, nil)

	// 升级到 Isolated
	fsm.Evaluate(&SecurityMetrics{ThreatLevel: LevelExtreme})
	if fsm.CurrentState() != StateIsolated {
		t.Fatalf("期望 Isolated，实际: %d", fsm.CurrentState())
	}

	// ForceState 强制降级到 Normal → 应绕过冷却期
	fsm.ForceState(StateNormal)
	if fsm.CurrentState() != StateNormal {
		t.Fatalf("ForceState 应绕过冷却期，期望 Normal，实际: %d", fsm.CurrentState())
	}

	// ForceState 强制切换到 Silent
	fsm.ForceState(StateSilent)
	if fsm.CurrentState() != StateSilent {
		t.Fatalf("ForceState 期望 Silent，实际: %d", fsm.CurrentState())
	}
}

func TestFSM_PolicyOverrideApplied(t *testing.T) {
	rules := []PolicyRule{
		{Condition: "rate_exceeded", Action: ActionThrottle, Params: map[string]int{"pps": 50}, Priority: 50},
	}
	policy := NewIngressPolicy(rules)
	fsm := NewSecurityFSM(policy, nil)

	// 升级到 Alert → 策略应被覆盖
	fsm.Evaluate(&SecurityMetrics{ThreatLevel: LevelHigh})

	// Alert 状态下 rate_exceeded 阈值收紧到 pps=5
	ctx := &IngressContext{ConnectionRate: 10}
	action := policy.Evaluate(ctx)
	if action != ActionThrottle {
		t.Fatalf("Alert 状态下超速应返回 Throttle，实际: %s", action)
	}
}

func TestFSM_StaysNormal_LowMetrics(t *testing.T) {
	policy := NewIngressPolicy(nil)
	fsm := NewSecurityFSM(policy, nil)

	fsm.Evaluate(&SecurityMetrics{ThreatLevel: LevelLow, RejectRate: 0.1})
	if fsm.CurrentState() != StateNormal {
		t.Fatalf("低指标应保持 Normal，实际: %d", fsm.CurrentState())
	}
}
