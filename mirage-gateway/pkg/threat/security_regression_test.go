package threat

import (
	"testing"
	"time"
)

// ============================================================
// 安全回归测试 — Gateway 侧（threat 包内）
// ============================================================

// --- 回归 2: 黑名单生效 ---

func TestSecurityRegression_Blacklist_AddAndGet(t *testing.T) {
	bm := NewBlacklistManager(nil, 65536)

	cidr := "192.168.1.100/32"
	expire := time.Now().Add(1 * time.Hour)

	err := bm.Add(cidr, expire, SourceLocal)
	if err != nil {
		t.Fatalf("添加黑名单失败: %v", err)
	}

	entry := bm.Get(cidr)
	if entry == nil {
		t.Fatal("黑名单条目应存在")
	}
	if entry.CIDR != cidr {
		t.Fatalf("CIDR 不匹配: 期望 %s，实际 %s", cidr, entry.CIDR)
	}
	if entry.Source != SourceLocal {
		t.Fatalf("Source 不匹配: 期望 SourceLocal，实际 %d", entry.Source)
	}
}

func TestSecurityRegression_Blacklist_SyncStatsConsistency(t *testing.T) {
	bm := NewBlacklistManager(nil, 65536)

	cidrs := []string{"10.0.0.1/32", "10.0.0.2/32", "172.16.0.0/24"}
	for _, cidr := range cidrs {
		if err := bm.Add(cidr, time.Now().Add(1*time.Hour), SourceLocal); err != nil {
			t.Fatalf("添加失败: %v", err)
		}
	}

	goCount, _ := bm.SyncStats()
	if goCount != len(cidrs) {
		t.Fatalf("Go 侧条目数不匹配: 期望 %d，实际 %d", len(cidrs), goCount)
	}

	if bm.Count() != len(cidrs) {
		t.Fatalf("Count() 不匹配: 期望 %d，实际 %d", len(cidrs), bm.Count())
	}
}

// --- 回归 3: 入口 Drop ---

func TestSecurityRegression_IngressDrop_BlacklistHit(t *testing.T) {
	rules := []PolicyRule{
		{Condition: "blacklist_hit", Action: ActionDrop, Priority: 100},
		{Condition: "threat_level_critical", Action: ActionDrop, Priority: 90},
		{Condition: "threat_level_high", Action: ActionThrottle, Params: map[string]int{"pps": 10}, Priority: 80},
	}
	policy := NewIngressPolicy(rules)

	ctx := &IngressContext{
		SourceIP:     "10.0.0.1",
		BlacklistHit: true,
	}

	action := policy.Evaluate(ctx)
	if action != ActionDrop {
		t.Fatalf("黑名单命中应返回 Drop，实际: %s", action)
	}
}

func TestSecurityRegression_IngressPass_NoMatch(t *testing.T) {
	rules := []PolicyRule{
		{Condition: "blacklist_hit", Action: ActionDrop, Priority: 100},
	}
	policy := NewIngressPolicy(rules)

	ctx := &IngressContext{
		SourceIP:     "10.0.0.1",
		BlacklistHit: false,
	}

	action := policy.Evaluate(ctx)
	if action != ActionPass {
		t.Fatalf("无匹配时应返回 Pass，实际: %s", action)
	}
}

// --- 回归 4: SecurityFSM 状态迁移 ---

func TestSecurityRegression_FSM_CriticalToHighPressure(t *testing.T) {
	policy := NewIngressPolicy(nil)
	fsm := NewSecurityFSM(policy, nil)

	fsm.Evaluate(&SecurityMetrics{ThreatLevel: LevelCritical})
	if fsm.CurrentState() != StateHighPressure {
		t.Fatalf("Critical 威胁应迁移到 HighPressure，实际: %d", fsm.CurrentState())
	}
}

func TestSecurityRegression_FSM_CooldownPreventsDowngrade(t *testing.T) {
	policy := NewIngressPolicy(nil)
	fsm := NewSecurityFSM(policy, nil)

	now := time.Now()
	fsm.nowFunc = func() time.Time { return now }

	fsm.Evaluate(&SecurityMetrics{ThreatLevel: LevelCritical})
	if fsm.CurrentState() != StateHighPressure {
		t.Fatalf("期望 HighPressure，实际: %d", fsm.CurrentState())
	}

	// 立即降级 → 应被冷却期阻止
	fsm.Evaluate(&SecurityMetrics{ThreatLevel: LevelLow})
	if fsm.CurrentState() != StateHighPressure {
		t.Fatalf("冷却期内不应降级，实际: %d", fsm.CurrentState())
	}

	// 前进 301 秒 → 冷却期结束
	now = now.Add(301 * time.Second)
	fsm.Evaluate(&SecurityMetrics{ThreatLevel: LevelLow})
	if fsm.CurrentState() != StateNormal {
		t.Fatalf("冷却期后应降级到 Normal，实际: %d", fsm.CurrentState())
	}
}
