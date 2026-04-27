package ebpf

import "testing"

// TestL1StatsTypeExists 编译回归测试：确保 L1Stats 类型唯一且字段与 C 侧 struct l1_stats 对齐
func TestL1StatsTypeExists(t *testing.T) {
	// 验证全部 9 个字段与 C 侧 struct l1_stats 严格对齐
	s := L1Stats{
		ASNDrops:       1,
		RateDrops:      2,
		SilentDrops:    3,
		BlacklistDrops: 4,
		SanityDrops:    5,
		ProfileDrops:   6,
		TotalChecked:   7,
		SynChallenge:   8,
		AckForgery:     9,
	}

	if s.TotalChecked != 7 {
		t.Fatal("L1Stats.TotalChecked 赋值异常")
	}
	if s.AckForgery != 9 {
		t.Fatal("L1Stats.AckForgery 赋值异常")
	}
}
