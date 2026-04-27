package main

import (
	"testing"

	"pgregory.net/rapid"
)

// Feature: zero-signature-elimination, Property 17: 速率限制随机偏移范围
// **Validates: Requirements 20.3**

func TestProperty_RateLimitRandomOffset(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		base := rapid.Uint32Range(50, 10000).Draw(t, "base")
		const ratio = 0.15

		result := applyRateOffset(base, ratio)

		// Account for uint32 truncation: floor of lower bound, ceil of upper bound
		lo := uint32(float64(base) * (1.0 - ratio))
		hi := uint32(float64(base)*(1.0+ratio)) + 1

		if result < lo || result > hi {
			t.Fatalf("applyRateOffset(%d, %.2f)=%d outside [%d, %d]",
				base, ratio, result, lo, hi)
		}

		if result == 0 {
			t.Fatalf("applyRateOffset(%d, %.2f)=%d, expected > 0", base, ratio, result)
		}
	})
}
