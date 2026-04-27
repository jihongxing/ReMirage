package ebpf

import (
	"math"
	"math/rand"
	"testing"

	"pgregory.net/rapid"
)

// Feature: zero-signature-elimination, Property 13: 配额降级概率正确性
// **Validates: Requirements 16.1, 16.2, 16.3**

// quotaDegradationDecision is the Go userspace equivalent of the eBPF quota
// degradation logic in jitter_lite_egress. Returns true if the packet passes.
func quotaDegradationDecision(remaining, total uint64, randVal uint32) bool {
	if total == 0 {
		return true
	}
	ratio := (remaining * 100) / total
	r := randVal % 100

	if ratio < 1 {
		// 剩余 < 1%：10% 通过率
		return r < 10
	} else if ratio < 10 {
		// 剩余 < 10%：50% 通过率
		return r < 50
	}
	// >= 10%：100% 通过
	return true
}

func TestProperty_QuotaDegradationProbability(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		total := rapid.Uint64Range(10000, 1000000).Draw(t, "total")

		// Generate a ratio in (1%, 10%) range
		ratioPercent := rapid.Float64Range(1.5, 9.5).Draw(t, "ratioPercent")
		remaining := uint64(float64(total) * ratioPercent / 100.0)

		seed := rapid.Int64().Draw(t, "seed")
		rng := rand.New(rand.NewSource(seed))

		const N = 2000
		passCount := 0
		for i := 0; i < N; i++ {
			if quotaDegradationDecision(remaining, total, rng.Uint32()) {
				passCount++
			}
		}

		passRate := float64(passCount) / float64(N)

		// 剩余 (1%, 10%)：通过率应接近 50% (±15%)
		if math.Abs(passRate-0.50) > 0.15 {
			t.Fatalf("ratio=%.1f%% pass_rate=%.3f, expected ~0.50 (±0.15)",
				ratioPercent, passRate)
		}
	})
}

func TestProperty_QuotaDegradationCritical(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		total := rapid.Uint64Range(10000, 1000000).Draw(t, "total")

		// Generate a ratio in (0%, 1%) range — but > 0
		ratioPercent := rapid.Float64Range(0.01, 0.95).Draw(t, "ratioPercent")
		remaining := uint64(float64(total) * ratioPercent / 100.0)
		if remaining == 0 {
			remaining = 1
		}

		seed := rapid.Int64().Draw(t, "seed")
		rng := rand.New(rand.NewSource(seed))

		const N = 2000
		passCount := 0
		for i := 0; i < N; i++ {
			if quotaDegradationDecision(remaining, total, rng.Uint32()) {
				passCount++
			}
		}

		passRate := float64(passCount) / float64(N)

		// 剩余 < 1%：通过率应接近 10% (±10%)
		if math.Abs(passRate-0.10) > 0.10 {
			t.Fatalf("ratio=%.2f%% pass_rate=%.3f, expected ~0.10 (±0.10)",
				ratioPercent, passRate)
		}
	})
}

func TestProperty_QuotaDegradationNormal(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		total := rapid.Uint64Range(10000, 1000000).Draw(t, "total")

		// Generate a ratio clearly >= 10% (avoid boundary rounding)
		ratioPercent := rapid.Float64Range(11.0, 100.0).Draw(t, "ratioPercent")
		remaining := uint64(float64(total) * ratioPercent / 100.0)

		seed := rapid.Int64().Draw(t, "seed")
		rng := rand.New(rand.NewSource(seed))

		const N = 500
		for i := 0; i < N; i++ {
			if !quotaDegradationDecision(remaining, total, rng.Uint32()) {
				t.Fatalf("ratio=%.1f%% should always pass, but was dropped", ratioPercent)
			}
		}
	})
}
