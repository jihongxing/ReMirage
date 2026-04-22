package cortex

import (
	"math"
	"testing"
)

func TestDiscretize(t *testing.T) {
	tests := []struct {
		size     int
		expected int
	}{
		{0, 0}, {32, 0}, {64, 0},
		{65, 1}, {128, 1}, {256, 1},
		{257, 2}, {400, 2}, {512, 2},
		{513, 3}, {800, 3}, {1024, 3},
		{1025, 4}, {1500, 4}, {9000, 4},
	}
	for _, tt := range tests {
		if got := discretize(tt.size); got != tt.expected {
			t.Errorf("discretize(%d) = %d, want %d", tt.size, got, tt.expected)
		}
	}
}

func TestDefaultBaseline(t *testing.T) {
	m := DefaultBaseline()
	if len(m.States) != 5 {
		t.Fatalf("expected 5 states, got %d", len(m.States))
	}
	// 验证每行概率和为 1.0
	for from, row := range m.TransitionMatrix {
		var sum float64
		for _, p := range row {
			sum += p
		}
		if math.Abs(sum-1.0) > 1e-9 {
			t.Errorf("state %d row sum = %f, want 1.0", from, sum)
		}
	}
}

func TestDeviation_EmptySequence(t *testing.T) {
	m := DefaultBaseline()
	if got := m.Deviation(nil); got != 0.0 {
		t.Errorf("Deviation(nil) = %f, want 0.0", got)
	}
	if got := m.Deviation([]int{}); got != 0.0 {
		t.Errorf("Deviation([]) = %f, want 0.0", got)
	}
}

func TestDeviation_SingleElement(t *testing.T) {
	m := DefaultBaseline()
	if got := m.Deviation([]int{100}); got != 0.0 {
		t.Errorf("Deviation([100]) = %f, want 0.0", got)
	}
}

func TestDeviation_Range(t *testing.T) {
	m := DefaultBaseline()
	// 正常流量模式：混合包长
	normal := []int{64, 128, 512, 1024, 64, 256, 512, 128, 64, 1400, 64, 128}
	dev := m.Deviation(normal)
	if dev < 0.0 || dev > 1.0 {
		t.Errorf("Deviation should be in [0,1], got %f", dev)
	}
}

func TestDeviation_AnomalousPattern(t *testing.T) {
	m := DefaultBaseline()
	// 异常模式：全部相同大小的包（单一状态转移）
	anomalous := make([]int, 200)
	for i := range anomalous {
		anomalous[i] = 1500
	}
	dev := m.Deviation(anomalous)
	// 单一状态转移应产生较高偏离度
	if dev <= 0.0 {
		t.Errorf("expected positive deviation for anomalous pattern, got %f", dev)
	}
}

func TestDeviation_PerfectBaseline(t *testing.T) {
	// 构造一个完全匹配基线的观测序列
	// 使用基线自身的转移概率生成序列
	m := DefaultBaseline()
	// 大量均匀分布的转移应接近 0
	// 这里用一个简单的近似：生成符合基线分布的序列
	seq := []int{32, 128, 400, 800, 1400, 32, 128, 400, 800, 1400}
	dev := m.Deviation(seq)
	if dev < 0.0 || dev > 1.0 {
		t.Errorf("Deviation should be in [0,1], got %f", dev)
	}
}
