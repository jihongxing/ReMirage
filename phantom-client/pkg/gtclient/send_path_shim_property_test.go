package gtclient

import (
	"testing"
	"time"

	"pgregory.net/rapid"
)

// Feature: zero-signature-elimination, Property 6: SendPathShim Padding 不变量
// **Validates: Requirements 4.2, 4.5**

func TestProperty_SendPathShimPaddingInvariant(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate shim config with reasonable ranges
		paddingMean := rapid.IntRange(100, 1400).Draw(t, "paddingMean")
		paddingStddev := rapid.IntRange(1, 200).Draw(t, "paddingStddev")
		maxMTU := rapid.IntRange(500, 1500).Draw(t, "maxMTU")

		// Generate an encrypted datagram of random length
		encLen := rapid.IntRange(1, 1400).Draw(t, "encLen")
		encrypted := rapid.SliceOfN(rapid.Byte(), encLen, encLen).Draw(t, "encrypted")

		shim := NewSendPathShim(SendPathShimConfig{
			PaddingMean:   paddingMean,
			PaddingStddev: paddingStddev,
			MaxMTU:        maxMTU,
		}, func(data []byte) error { return nil })

		padded := shim.applyPadding(encrypted)

		// Invariant 1: padded length >= original length (padding never shrinks)
		if len(padded) < len(encrypted) {
			t.Fatalf("padding shrunk data: original=%d, padded=%d", len(encrypted), len(padded))
		}

		// Invariant 2: if original data fits within MTU, padded length <= maxMTU
		// (padding never pushes beyond MTU; but original data already > MTU is untouched)
		if len(encrypted) <= maxMTU && len(padded) > maxMTU {
			t.Fatalf("padded length %d exceeds maxMTU %d (original was %d)", len(padded), maxMTU, len(encrypted))
		}

		// Invariant 3: original data is preserved (prefix unchanged)
		for i := 0; i < len(encrypted); i++ {
			if padded[i] != encrypted[i] {
				t.Fatalf("original data corrupted at byte %d: expected %d, got %d", i, encrypted[i], padded[i])
			}
		}
	})
}

// Feature: zero-signature-elimination, Property 7: SendPathShim IAT 采样范围
// **Validates: Requirements 4.4**

func TestProperty_SendPathShimIATSamplingRange(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		iatMode := IATMode(rapid.IntRange(0, 1).Draw(t, "iatMode"))
		iatMeanUs := rapid.Int64Range(100, 50000).Draw(t, "iatMeanUs")
		iatStddevUs := rapid.Int64Range(10, 10000).Draw(t, "iatStddevUs")

		shim := NewSendPathShim(SendPathShimConfig{
			PaddingMean:   500,
			PaddingStddev: 50,
			MaxMTU:        1200,
			IATMode:       iatMode,
			IATMeanUs:     iatMeanUs,
			IATStddevUs:   iatStddevUs,
		}, func(data []byte) error { return nil })

		delay := shim.sampleIATDelay()

		// Invariant 1: delay is never negative (clamped to 0)
		if delay < 0 {
			t.Fatalf("IAT delay is negative: %v", delay)
		}

		// Invariant 2: delay is a valid time.Duration (not NaN/Inf equivalent)
		if delay > 10*time.Second {
			// Sanity bound: with mean up to 50ms and stddev up to 10ms,
			// 10s is an extreme outlier that indicates a bug
			t.Fatalf("IAT delay unreasonably large: %v (mean=%dμs, stddev=%dμs, mode=%d)",
				delay, iatMeanUs, iatStddevUs, iatMode)
		}
	})
}
