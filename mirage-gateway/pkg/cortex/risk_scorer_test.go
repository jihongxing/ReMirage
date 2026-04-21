package cortex

import (
	"testing"
	"time"

	"mirage-gateway/pkg/threat"
)

// mockBlacklistManager 用于测试的黑名单管理器
func newTestBlacklistManager() *threat.BlacklistManager {
	return threat.NewBlacklistManager(nil, 1000)
}

func TestRiskScorer_AddScore_Basic(t *testing.T) {
	bm := newTestBlacklistManager()
	rs := NewRiskScorer(bm)

	rs.AddScore("10.0.0.1", 20, "test")
	if got := rs.GetScore("10.0.0.1"); got != 20 {
		t.Fatalf("expected score 20, got %d", got)
	}
}

func TestRiskScorer_AddScore_Accumulation(t *testing.T) {
	bm := newTestBlacklistManager()
	rs := NewRiskScorer(bm)

	rs.AddScore("10.0.0.1", 20, "threat")
	rs.AddScore("10.0.0.1", 30, "honeypot")
	if got := rs.GetScore("10.0.0.1"); got != 50 {
		t.Fatalf("expected score 50, got %d", got)
	}
}

func TestRiskScorer_AddScore_MaxCap(t *testing.T) {
	bm := newTestBlacklistManager()
	rs := NewRiskScorer(bm)

	rs.AddScore("10.0.0.1", 60, "a")
	rs.AddScore("10.0.0.1", 60, "b")
	if got := rs.GetScore("10.0.0.1"); got != MaxScore {
		t.Fatalf("expected score capped at %d, got %d", MaxScore, got)
	}
}

func TestRiskScorer_AutoBan_AtThreshold(t *testing.T) {
	bm := newTestBlacklistManager()
	rs := NewRiskScorer(bm)

	// 蜜罐命中 30 + 高危指纹 40 = 70 >= AutoBanThreshold
	rs.AddScore("192.168.1.100", HoneypotHitScore, "honeypot")
	rs.AddScore("192.168.1.100", DangerousFPScore, "fingerprint")

	score := rs.GetScore("192.168.1.100")
	if score < AutoBanThreshold {
		t.Fatalf("expected score >= %d, got %d", AutoBanThreshold, score)
	}

	// 验证黑名单已添加
	entry := bm.Get("192.168.1.100/32")
	if entry == nil {
		t.Fatal("expected IP to be auto-banned in blacklist")
	}
	if entry.Source != threat.SourceLocal {
		t.Fatalf("expected source SourceLocal, got %v", entry.Source)
	}
}

func TestRiskScorer_NoBan_BelowThreshold(t *testing.T) {
	bm := newTestBlacklistManager()
	rs := NewRiskScorer(bm)

	// 只有蜜罐命中 30 < 70
	rs.AddScore("192.168.1.200", HoneypotHitScore, "honeypot")

	entry := bm.Get("192.168.1.200/32")
	if entry != nil {
		t.Fatal("expected IP NOT to be banned below threshold")
	}
}

func TestRiskScorer_Decay(t *testing.T) {
	bm := newTestBlacklistManager()
	rs := NewRiskScorer(bm)

	rs.AddScore("10.0.0.5", 30, "test")
	rs.decay()

	expected := 30 - DecayPerHour
	if got := rs.GetScore("10.0.0.5"); got != expected {
		t.Fatalf("expected score %d after decay, got %d", expected, got)
	}
}

func TestRiskScorer_Decay_RemovesZero(t *testing.T) {
	bm := newTestBlacklistManager()
	rs := NewRiskScorer(bm)

	rs.AddScore("10.0.0.6", 5, "test")
	rs.decay() // 5 - 10 = -5 → removed

	if got := rs.GetScore("10.0.0.6"); got != 0 {
		t.Fatalf("expected score 0 (removed), got %d", got)
	}
}

func TestRiskScorer_Sources_Tracked(t *testing.T) {
	bm := newTestBlacklistManager()
	rs := NewRiskScorer(bm)

	rs.AddScore("10.0.0.7", HoneypotHitScore, "honeypot")
	rs.AddScore("10.0.0.7", DangerousFPScore, "fingerprint")

	ipScore := rs.GetIPScore("10.0.0.7")
	if ipScore == nil {
		t.Fatal("expected IPScore to exist")
	}
	if len(ipScore.Sources) != 2 {
		t.Fatalf("expected 2 sources, got %d", len(ipScore.Sources))
	}
}

// TestIntegration_HoneypotToFingerprint_AutoBan 联动链路集成测试：
// 蜜罐命中 → 评分累加 → 指纹命中 → 超阈值自动封禁
func TestIntegration_HoneypotToFingerprint_AutoBan(t *testing.T) {
	bm := newTestBlacklistManager()
	rs := NewRiskScorer(bm)
	bus := NewThreatBus(nil)

	targetIP := "203.0.113.50"

	// 订阅 ThreatBus 事件，模拟 Cortex 消费
	events := bus.Subscribe()

	// 步骤 1：蜜罐命中 → 上报 ThreatBus → RiskScorer 加分
	honeypotEvent := &HighSeverityEvent{
		ID:         "hp_test",
		Timestamp:  time.Now().UnixMilli(),
		ThreatType: EventHoneypot,
		Severity:   8,
		SourceIP:   targetIP,
	}
	bus.EmitHighSeverityEvent(honeypotEvent)

	// 消费事件
	select {
	case evt := <-events:
		if evt.ThreatType != EventHoneypot {
			t.Fatalf("expected honeypot event, got %s", evt.ThreatType)
		}
		// Cortex 处理：蜜罐命中 +30
		rs.AddScore(evt.SourceIP, HoneypotHitScore, "honeypot")
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for honeypot event")
	}

	// 验证中间状态：30 分，未封禁
	if score := rs.GetScore(targetIP); score != HoneypotHitScore {
		t.Fatalf("expected score %d, got %d", HoneypotHitScore, score)
	}
	if entry := bm.Get(targetIP + "/32"); entry != nil {
		t.Fatal("should not be banned yet")
	}

	// 步骤 2：高危指纹检测 → 上报 ThreatBus → RiskScorer 加分
	fpEvent := &HighSeverityEvent{
		ID:          "fp_test",
		Timestamp:   time.Now().UnixMilli(),
		ThreatType:  EventFingerprint,
		Severity:    8,
		SourceIP:    targetIP,
		Fingerprint: "scanner_nmap_v7",
	}
	bus.EmitHighSeverityEvent(fpEvent)

	// 消费事件
	select {
	case evt := <-events:
		if evt.ThreatType != EventFingerprint {
			t.Fatalf("expected fingerprint event, got %s", evt.ThreatType)
		}
		// Cortex 处理：高危指纹 +40
		rs.AddScore(evt.SourceIP, DangerousFPScore, "fingerprint")
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for fingerprint event")
	}

	// 步骤 3：验证最终状态 — 评分 >= 70，已自动封禁
	finalScore := rs.GetScore(targetIP)
	if finalScore < AutoBanThreshold {
		t.Fatalf("expected score >= %d, got %d", AutoBanThreshold, finalScore)
	}

	entry := bm.Get(targetIP + "/32")
	if entry == nil {
		t.Fatal("expected IP to be auto-banned after combined score >= threshold")
	}

	// 验证封禁 TTL
	expectedExpiry := time.Now().Add(AutoBanTTL)
	if entry.ExpireAt.Before(time.Now()) || entry.ExpireAt.After(expectedExpiry.Add(time.Second)) {
		t.Fatalf("unexpected expiry time: %v", entry.ExpireAt)
	}

	t.Logf("联动链路验证通过: IP=%s score=%d banned=true", targetIP, finalScore)
}

// TestCircuitBreaker_DropsLowSeverity 断路器测试：队列满时丢弃低优先级事件
func TestCircuitBreaker_DropsLowSeverity(t *testing.T) {
	bus := NewThreatBus(nil)
	bus.SetMinSeverity(1) // 允许所有事件通过

	// 订阅（通道容量 100）
	ch := bus.Subscribe()

	// 通过 Emit 填充队列到 > 80%（填 85 个高优先级事件）
	for i := 0; i < 85; i++ {
		bus.EmitHighSeverityEvent(&HighSeverityEvent{
			ID:       "fill",
			Severity: 7,
			SourceIP: "10.0.0.1",
		})
	}

	// 记录当前队列长度
	beforeLen := len(ch)

	// 发送低优先级事件（severity=3 < 5），应被断路器丢弃
	bus.EmitHighSeverityEvent(&HighSeverityEvent{
		ID:         "low_test",
		ThreatType: EventThreat,
		Severity:   3,
		SourceIP:   "10.0.0.1",
	})
	afterLen := len(ch)

	if afterLen != beforeLen {
		t.Fatalf("expected low severity event to be dropped by circuit breaker, queue grew from %d to %d",
			beforeLen, afterLen)
	}

	// 发送高优先级事件（severity=8 >= 5），应正常入队
	bus.EmitHighSeverityEvent(&HighSeverityEvent{
		ID:         "high_test",
		ThreatType: EventThreat,
		Severity:   8,
		SourceIP:   "10.0.0.2",
	})
	finalLen := len(ch)

	if finalLen != beforeLen+1 {
		t.Fatalf("expected high severity event to be enqueued, queue: %d → %d (expected %d)",
			beforeLen, finalLen, beforeLen+1)
	}
}
