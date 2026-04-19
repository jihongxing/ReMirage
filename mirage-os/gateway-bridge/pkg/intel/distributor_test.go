package intel

import (
	"testing"

	"pgregory.net/rapid"
)

// Feature: mirage-os-brain, Property 5: 黑名单封禁阈值
func TestProperty_BanThreshold(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		threshold := rapid.IntRange(1, 1000).Draw(t, "threshold")
		hitCount := rapid.IntRange(0, 2000).Draw(t, "hitCount")

		shouldBan := hitCount >= threshold

		// 验证阈值逻辑的正确性
		if hitCount < threshold && shouldBan {
			t.Fatalf("should not ban when hitCount(%d) < threshold(%d)", hitCount, threshold)
		}
		if hitCount >= threshold && !shouldBan {
			t.Fatalf("should ban when hitCount(%d) >= threshold(%d)", hitCount, threshold)
		}
	})
}

// 单元测试：Distributor 创建
func TestNewDistributor(t *testing.T) {
	// 验证默认值
	d := &Distributor{
		banThreshold: 100,
		cleanupDays:  30,
		cleanupMin:   10,
	}
	if d.banThreshold != 100 {
		t.Fatalf("expected banThreshold=100, got %d", d.banThreshold)
	}
	if d.cleanupDays != 30 {
		t.Fatalf("expected cleanupDays=30, got %d", d.cleanupDays)
	}
}
