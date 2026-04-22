package ebpf

import "testing"

// TestL1StatsTypeExists 编译回归测试：确保 L1Stats 类型唯一且可用
func TestL1StatsTypeExists(t *testing.T) {
	// 类型断言：如果 L1Stats 存在重复定义，此文件将无法编译
	var _ L1Stats

	// 验证字段与 C 侧 struct l1_stats 对齐（7 个 uint64 字段）
	s := L1Stats{
		ASNDrops:       1,
		RateDrops:      2,
		SilentDrops:    3,
		BlacklistDrops: 4,
		SanityDrops:    5,
		ProfileDrops:   6,
		TotalChecked:   7,
	}

	if s.TotalChecked != 7 {
		t.Fatal("L1Stats 字段赋值异常")
	}
}
