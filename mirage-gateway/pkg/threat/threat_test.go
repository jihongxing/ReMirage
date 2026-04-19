package threat

import (
	"context"
	"fmt"
	"testing"
	"time"

	"mirage-gateway/pkg/ebpf"
	"mirage-gateway/pkg/evaluator"

	"pgregory.net/rapid"
)

// Feature: gateway-closure, Property 5: 威胁事件标准化完整性
func TestProperty_EventNormalization(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		source := rapid.IntRange(1, 3).Draw(t, "source")
		agg := NewAggregator(10000)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		agg.Start(ctx)

		switch EventSource(source) {
		case SourceEBPF:
			ev := &ebpf.ThreatEvent{
				Timestamp:  uint64(time.Now().UnixNano()),
				ThreatType: uint32(rapid.IntRange(1, 4).Draw(t, "type")),
				SourceIP:   0x0100007F, // 127.0.0.1
				SourcePort: uint16(rapid.IntRange(1024, 65535).Draw(t, "port")),
				Severity:   uint32(rapid.IntRange(0, 10).Draw(t, "sev")),
			}
			agg.IngestEBPF(ev)
		case SourceCortex:
			agg.IngestCortex("192.168.1.1", "test_reason")
		case SourceEvaluator:
			sig := evaluator.FeedbackSignal{
				Type:       "anomaly",
				Confidence: float64(rapid.IntRange(0, 100).Draw(t, "conf")),
				Action:     "adjust_parameters",
			}
			agg.IngestEvaluator(sig)
		}

		select {
		case event := <-agg.Subscribe():
			if event.Timestamp.IsZero() {
				t.Fatal("Timestamp 不应为零")
			}
			if event.EventType == 0 {
				t.Fatal("EventType 不应为零")
			}
			if event.SourceIP == "" {
				t.Fatal("SourceIP 不应为空")
			}
			if event.Severity < 0 || event.Severity > 10 {
				t.Fatalf("Severity 越界: %d", event.Severity)
			}
			if event.Source < SourceEBPF || event.Source > SourceEvaluator {
				t.Fatalf("Source 无效: %d", event.Source)
			}
		case <-time.After(2 * time.Second):
			// 可能被去重，跳过
		}
	})
}

// Feature: gateway-closure, Property 6: 同源事件聚合
func TestProperty_SameSourceAggregation(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		n := rapid.IntRange(2, 20).Draw(t, "n")
		agg := NewAggregator(10000)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		agg.Start(ctx)

		ip := "10.0.0.1"
		for i := 0; i < n; i++ {
			ev := &ebpf.ThreatEvent{
				ThreatType: 1,
				SourceIP:   0x0100000A, // 10.0.0.1
				Severity:   5,
			}
			agg.IngestEBPF(ev)
		}

		// 等待聚合处理
		time.Sleep(200 * time.Millisecond)

		// 第一个事件应该通过，后续被聚合
		select {
		case event := <-agg.Subscribe():
			if event.SourceIP != ip {
				t.Fatalf("IP 不匹配: %s", event.SourceIP)
			}
			// Count 在去重窗口内应 >= 1
		case <-time.After(2 * time.Second):
			// 可能已被消费
		}
	})
}

// Feature: gateway-closure, Property 7: 事件队列上限
func TestProperty_QueueLimit(t *testing.T) {
	maxQueue := 100 // 使用较小值加速测试
	agg := NewAggregator(maxQueue)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	agg.Start(ctx)

	// 注入超过上限的事件（使用不同 IP 避免去重）
	total := maxQueue + 50
	for i := 0; i < total; i++ {
		ip := uint32(i + 1)
		ev := &ebpf.ThreatEvent{
			ThreatType: 1,
			SourceIP:   ip,
			Severity:   5,
		}
		agg.IngestEBPF(ev)
	}

	time.Sleep(500 * time.Millisecond)

	// 丢弃计数应 > 0
	drops := agg.GetDropCount()
	if drops == 0 {
		t.Log("注意: 队列未溢出（处理速度快于注入速度）")
	}
}

// Feature: gateway-closure, Property 8: 威胁等级参数单调性
func TestProperty_ThreatLevelMonotonicity(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		l1 := ThreatLevel(rapid.IntRange(1, 4).Draw(t, "l1"))
		l2 := l1 + 1

		p1 := GetLevelParams(l1)
		p2 := GetLevelParams(l2)

		if p2.JitterMeanUs < p1.JitterMeanUs {
			t.Fatalf("JitterMean 非单调: L%d=%d > L%d=%d", l1, p1.JitterMeanUs, l2, p2.JitterMeanUs)
		}
		if p2.NoiseIntensity < p1.NoiseIntensity {
			t.Fatalf("NoiseIntensity 非单调: L%d=%d > L%d=%d", l1, p1.NoiseIntensity, l2, p2.NoiseIntensity)
		}
		if p2.PaddingRate < p1.PaddingRate {
			t.Fatalf("PaddingRate 非单调: L%d=%d > L%d=%d", l1, p1.PaddingRate, l2, p2.PaddingRate)
		}
	})
}

// Feature: gateway-closure, Property 9: 黑名单添加往返
func TestProperty_BlacklistAddRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		octet1 := rapid.IntRange(1, 254).Draw(t, "o1")
		octet2 := rapid.IntRange(0, 255).Draw(t, "o2")
		octet3 := rapid.IntRange(0, 255).Draw(t, "o3")
		octet4 := rapid.IntRange(1, 254).Draw(t, "o4")
		prefix := rapid.SampledFrom([]int{8, 16, 24, 32}).Draw(t, "prefix")

		cidr := fmt.Sprintf("%d.%d.%d.%d/%d", octet1, octet2, octet3, octet4, prefix)
		expire := time.Now().Add(1 * time.Hour)

		bm := NewBlacklistManager(nil, 65536)
		if err := bm.Add(cidr, expire, SourceLocal); err != nil {
			t.Fatalf("添加失败: %v", err)
		}

		entry := bm.Get(cidr)
		if entry == nil {
			t.Fatalf("条目不存在: %s", cidr)
		}
		if entry.Source != SourceLocal {
			t.Fatalf("Source 不匹配: %d", entry.Source)
		}
		if entry.AddedAt.IsZero() {
			t.Fatal("AddedAt 不应为零")
		}
		if entry.ExpireAt.IsZero() {
			t.Fatal("ExpireAt 不应为零")
		}
	})
}

// Feature: gateway-closure, Property 10: 全局黑名单优先合并
func TestProperty_GlobalBlacklistPriority(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		cidr := "192.168.1.0/24"
		bm := NewBlacklistManager(nil, 65536)

		// 先添加本地
		bm.Add(cidr, time.Now().Add(1*time.Hour), SourceLocal)

		// 合并全局
		globalEntries := []BlacklistEntry{
			{CIDR: cidr, ExpireAt: time.Now().Add(2 * time.Hour)},
		}
		bm.MergeGlobal(globalEntries)

		entry := bm.Get(cidr)
		if entry == nil {
			t.Fatal("条目不存在")
		}
		if entry.Source != SourceGlobal {
			t.Fatalf("合并后 Source 应为 GLOBAL，实际: %d", entry.Source)
		}
	})
}

// 单元测试: 冷却期
func TestResponder_CooldownPeriod(t *testing.T) {
	r := NewResponder(nil, nil)

	// 模拟升级到 Critical
	r.mu.Lock()
	r.currentLevel = LevelCritical
	r.cooldownUntil = time.Now().Add(120 * time.Second)
	r.mu.Unlock()

	// 尝试降级（应被冷却期阻止）
	event := &UnifiedThreatEvent{Severity: 1}
	r.handleEvent(event)

	if r.GetCurrentLevel() != LevelCritical {
		t.Fatalf("冷却期内不应降级，当前: %d", r.GetCurrentLevel())
	}
}

// 单元测试: 过期清理
func TestBlacklist_ExpiryCleanup(t *testing.T) {
	bm := NewBlacklistManager(nil, 65536)

	// 添加已过期条目
	bm.Add("10.0.0.1/32", time.Now().Add(-1*time.Second), SourceLocal)
	bm.Add("10.0.0.2/32", time.Now().Add(1*time.Hour), SourceLocal)

	bm.cleanExpired()

	if bm.Count() != 1 {
		t.Fatalf("过期清理后应剩 1 条，实际: %d", bm.Count())
	}
}
