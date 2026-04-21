package threat

import (
	"testing"
)

// --- 6.6 IngressPolicy 单元测试 ---

func TestPolicy_BlacklistHit_Drop(t *testing.T) {
	rules := []PolicyRule{
		{Condition: "blacklist_hit", Action: ActionDrop, Priority: 100},
		{Condition: "threat_level_high", Action: ActionThrottle, Priority: 80},
	}
	policy := NewIngressPolicy(rules)

	ctx := &IngressContext{
		SourceIP:     "10.0.0.1",
		BlacklistHit: true,
		ThreatLevel:  LevelHigh,
	}

	action := policy.Evaluate(ctx)
	if action != ActionDrop {
		t.Fatalf("黑名单命中应返回 Drop，实际: %s", action)
	}
}

func TestPolicy_MultipleConditions_HighestPriorityWins(t *testing.T) {
	rules := []PolicyRule{
		{Condition: "blacklist_hit", Action: ActionDrop, Priority: 100},
		{Condition: "threat_level_critical", Action: ActionDrop, Priority: 90},
		{Condition: "threat_level_high", Action: ActionThrottle, Params: map[string]int{"pps": 10}, Priority: 80},
		{Condition: "honeypot_hit", Action: ActionTrap, Priority: 70},
		{Condition: "suspicious_fingerprint", Action: ActionObserve, Priority: 60},
		{Condition: "rate_exceeded", Action: ActionThrottle, Params: map[string]int{"pps": 50}, Priority: 50},
	}
	policy := NewIngressPolicy(rules)

	// 同时命中 threat_level_high(80) + honeypot_hit(70) → 应选 throttle(80)
	ctx := &IngressContext{
		SourceIP:    "10.0.0.2",
		ThreatLevel: LevelHigh,
		HoneypotHit: true,
	}
	action := policy.Evaluate(ctx)
	if action != ActionThrottle {
		t.Fatalf("多条件命中应选最高优先级 Throttle(80)，实际: %s", action)
	}

	// 同时命中 blacklist_hit(100) + threat_level_critical(90) + honeypot_hit(70) → 应选 drop(100)
	ctx2 := &IngressContext{
		SourceIP:     "10.0.0.3",
		BlacklistHit: true,
		ThreatLevel:  LevelCritical,
		HoneypotHit:  true,
	}
	action2 := policy.Evaluate(ctx2)
	if action2 != ActionDrop {
		t.Fatalf("三条件命中应选最高优先级 Drop(100)，实际: %s", action2)
	}
}

func TestPolicy_HoneypotHit_Trap(t *testing.T) {
	rules := []PolicyRule{
		{Condition: "honeypot_hit", Action: ActionTrap, Priority: 70},
	}
	policy := NewIngressPolicy(rules)

	ctx := &IngressContext{
		SourceIP:    "10.0.0.4",
		HoneypotHit: true,
	}
	action := policy.Evaluate(ctx)
	if action != ActionTrap {
		t.Fatalf("蜜罐命中应返回 Trap，实际: %s", action)
	}
}

func TestPolicy_NoConditions_Pass(t *testing.T) {
	rules := []PolicyRule{
		{Condition: "blacklist_hit", Action: ActionDrop, Priority: 100},
		{Condition: "threat_level_high", Action: ActionThrottle, Priority: 80},
		{Condition: "honeypot_hit", Action: ActionTrap, Priority: 70},
	}
	policy := NewIngressPolicy(rules)

	// 无任何条件命中
	ctx := &IngressContext{
		SourceIP:    "10.0.0.5",
		ThreatLevel: LevelLow,
	}
	action := policy.Evaluate(ctx)
	if action != ActionPass {
		t.Fatalf("无条件命中应返回 Pass，实际: %s", action)
	}
}

func TestPolicy_StateOverride_Alert(t *testing.T) {
	rules := []PolicyRule{
		{Condition: "rate_exceeded", Action: ActionThrottle, Params: map[string]int{"pps": 50}, Priority: 50},
	}
	policy := NewIngressPolicy(rules)

	// 覆盖为 Alert 状态
	policy.ApplyStateOverride(StateAlert)

	// Alert 状态下 rate_exceeded 阈值收紧到 pps=5，优先级提升到 95
	ctx := &IngressContext{
		SourceIP:       "10.0.0.6",
		ConnectionRate: 10, // 超过 pps=5
	}
	action := policy.Evaluate(ctx)
	if action != ActionThrottle {
		t.Fatalf("Alert 状态下超速应返回 Throttle，实际: %s", action)
	}
}

func TestPolicy_StateOverride_Normal_NoChange(t *testing.T) {
	rules := []PolicyRule{
		{Condition: "blacklist_hit", Action: ActionDrop, Priority: 100},
	}
	policy := NewIngressPolicy(rules)

	policy.ApplyStateOverride(StateNormal)

	// Normal 状态不覆盖，规则不变
	got := policy.GetRules()
	if len(got) != 1 || got[0].Condition != "blacklist_hit" {
		t.Fatalf("Normal 状态不应覆盖规则，实际规则数: %d", len(got))
	}
}

func TestPolicy_SuspiciousFingerprint_Observe(t *testing.T) {
	rules := []PolicyRule{
		{Condition: "suspicious_fingerprint", Action: ActionObserve, Priority: 60},
	}
	policy := NewIngressPolicy(rules)

	ctx := &IngressContext{
		SourceIP:        "10.0.0.7",
		FingerprintRisk: 80, // >= 70 触发
	}
	action := policy.Evaluate(ctx)
	if action != ActionObserve {
		t.Fatalf("高危指纹应返回 Observe，实际: %s", action)
	}
}

func TestPolicy_RateExceeded_Throttle(t *testing.T) {
	rules := []PolicyRule{
		{Condition: "rate_exceeded", Action: ActionThrottle, Params: map[string]int{"pps": 50}, Priority: 50},
	}
	policy := NewIngressPolicy(rules)

	ctx := &IngressContext{
		SourceIP:       "10.0.0.8",
		ConnectionRate: 100, // > 50
	}
	action := policy.Evaluate(ctx)
	if action != ActionThrottle {
		t.Fatalf("超速应返回 Throttle，实际: %s", action)
	}

	// 未超速
	ctx2 := &IngressContext{
		SourceIP:       "10.0.0.9",
		ConnectionRate: 30, // < 50
	}
	action2 := policy.Evaluate(ctx2)
	if action2 != ActionPass {
		t.Fatalf("未超速应返回 Pass，实际: %s", action2)
	}
}

func TestPolicy_EvaluateWithRule_ReturnsCondition(t *testing.T) {
	rules := []PolicyRule{
		{Condition: "blacklist_hit", Action: ActionDrop, Priority: 100},
		{Condition: "honeypot_hit", Action: ActionTrap, Priority: 70},
	}
	policy := NewIngressPolicy(rules)

	ctx := &IngressContext{
		SourceIP:     "10.0.0.10",
		BlacklistHit: true,
		HoneypotHit:  true,
	}
	action, condition := policy.EvaluateWithRule(ctx)
	if action != ActionDrop {
		t.Fatalf("应返回 Drop，实际: %s", action)
	}
	if condition != "blacklist_hit" {
		t.Fatalf("应返回条件 blacklist_hit，实际: %s", condition)
	}
}

func TestPolicy_UpdateRules(t *testing.T) {
	policy := NewIngressPolicy([]PolicyRule{
		{Condition: "blacklist_hit", Action: ActionDrop, Priority: 100},
	})

	// 热更新
	newRules := []PolicyRule{
		{Condition: "honeypot_hit", Action: ActionTrap, Priority: 70},
	}
	policy.UpdateRules(newRules)

	got := policy.GetRules()
	if len(got) != 1 || got[0].Condition != "honeypot_hit" {
		t.Fatalf("更新后规则应为 honeypot_hit，实际: %v", got)
	}
}

func TestPolicyConfig_LoadFromBytes(t *testing.T) {
	yamlData := []byte(`
ingress_policy:
  rules:
    - condition: blacklist_hit
      action: drop
      priority: 100
    - condition: threat_level_high
      action: throttle
      params:
        pps: 10
      priority: 80
    - condition: honeypot_hit
      action: trap
      priority: 70
`)
	policy, err := LoadPolicyFromBytes(yamlData)
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}

	rules := policy.GetRules()
	if len(rules) != 3 {
		t.Fatalf("应有 3 条规则，实际: %d", len(rules))
	}
	if rules[0].Action != ActionDrop || rules[0].Priority != 100 {
		t.Fatalf("第一条规则应为 Drop/100，实际: %s/%d", rules[0].Action, rules[0].Priority)
	}
	if rules[1].Action != ActionThrottle || rules[1].Params["pps"] != 10 {
		t.Fatalf("第二条规则应为 Throttle/pps=10，实际: %s/%v", rules[1].Action, rules[1].Params)
	}
	if rules[2].Action != ActionTrap {
		t.Fatalf("第三条规则应为 Trap，实际: %s", rules[2].Action)
	}
}

func TestPolicyConfig_InvalidAction(t *testing.T) {
	yamlData := []byte(`
ingress_policy:
  rules:
    - condition: blacklist_hit
      action: invalid_action
      priority: 100
`)
	_, err := LoadPolicyFromBytes(yamlData)
	if err == nil {
		t.Fatal("无效动作应返回错误")
	}
}

func TestPolicy_LogPolicyAction(t *testing.T) {
	logger := NewIngressLogger()
	ctx := &IngressContext{
		SourceIP:    "192.168.1.1",
		ThreatLevel: LevelHigh,
	}

	LogPolicyAction(logger, ctx, ActionDrop, "blacklist_hit")

	recent := logger.Recent(1)
	if len(recent) != 1 {
		t.Fatalf("应有 1 条日志，实际: %d", len(recent))
	}
	if recent[0].SourceIP != "192.168.1.1" {
		t.Fatalf("日志 IP 不匹配: %s", recent[0].SourceIP)
	}
	if recent[0].Action != ActionDrop {
		t.Fatalf("日志动作不匹配: %s", recent[0].Action)
	}
	if recent[0].Reason != "blacklist_hit" {
		t.Fatalf("日志原因不匹配: %s", recent[0].Reason)
	}
}

func TestPolicy_LogPolicyAction_NilLogger(t *testing.T) {
	ctx := &IngressContext{SourceIP: "10.0.0.1"}
	// 不应 panic
	LogPolicyAction(nil, ctx, ActionDrop, "test")
}
