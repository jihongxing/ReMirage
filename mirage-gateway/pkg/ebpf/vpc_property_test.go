package ebpf

import (
	"math"
	"math/rand"
	"testing"

	"pgregory.net/rapid"
)

// Feature: zero-signature-elimination, Property 11: VPC 光缆抖动指数分布特征
// **Validates: Requirements 12.1, 12.3**

// simulateFiberJitterV2 is the Go userspace equivalent of the eBPF simulate_fiber_jitter_v2
// using piecewise linear approximation of exponential distribution.
func simulateFiberJitterV2(fiberBaseUs, fiberVarianceUs uint32, rng *rand.Rand) uint64 {
	random := rng.Uint32()
	u := uint64(random>>16) + 1 // [1, 65536]
	var negLn uint64

	if u > 32768 {
		negLn = (65536 - u) * 1000 / 32768
	} else if u > 8192 {
		negLn = 693 + (32768-u)*1000/24576
	} else {
		negLn = 2079 + (8192-u)*2000/8192
	}

	return uint64(fiberBaseUs) + (negLn*uint64(fiberVarianceUs))/1000
}

func TestProperty_VPCFiberJitterExponentialDistribution(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		fiberBase := rapid.Uint32Range(10, 500).Draw(t, "fiberBase")
		fiberVariance := rapid.Uint32Range(5, 200).Draw(t, "fiberVariance")
		seed := rapid.Int64().Draw(t, "seed")
		rng := rand.New(rand.NewSource(seed))

		const N = 1000
		var samples [N]float64
		var sum float64

		for i := 0; i < N; i++ {
			s := simulateFiberJitterV2(fiberBase, fiberVariance, rng)
			samples[i] = float64(s)
			sum += samples[i]
		}

		sampleMean := sum / N

		// Property 1: all values >= fiber_base_us
		for i := 0; i < N; i++ {
			if samples[i] < float64(fiberBase) {
				t.Fatalf("sample %d = %.0f < fiberBase %d", i, samples[i], fiberBase)
			}
		}

		// Property 2: distribution should be right-skewed (skewness > 0)
		var m2, m3 float64
		for i := 0; i < N; i++ {
			diff := samples[i] - sampleMean
			m2 += diff * diff
			m3 += diff * diff * diff
		}
		m2 /= N
		m3 /= N
		if m2 > 0 {
			skewness := m3 / math.Pow(m2, 1.5)
			if skewness <= 0 {
				t.Fatalf("distribution not right-skewed: skewness=%.3f (base=%d, var=%d)",
					skewness, fiberBase, fiberVariance)
			}
		}
	})
}

// Feature: zero-signature-elimination, Property 12: VPC 跨洋模拟非周期性
// **Validates: Requirements 12.2**

// simulateSubmarineCable is the Go userspace equivalent of the eBPF simulate_submarine_cable
// using 3-frequency superposition with pseudo-random components.
func simulateSubmarineCable(timestamp uint64, rng *rand.Rand) uint64 {
	r1 := rng.Uint32()
	r2 := rng.Uint32()
	tMs := timestamp / 1000000

	comp1 := ((tMs*7 + uint64(r1)) % 200)
	comp2 := ((tMs*31 + uint64(r2)) % 100)
	comp3 := uint64(rng.Uint32() % 50)

	return (comp1 + comp2 + comp3) / 3
}

func TestProperty_VPCSubmarineCableNonPeriodic(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		baseTimestamp := rapid.Uint64Range(1000000000, 1000000000000).Draw(t, "baseTs")
		seed := rapid.Int64().Draw(t, "seed")
		rng := rand.New(rand.NewSource(seed))

		const N = 200
		var samples [N]float64
		var sum float64

		// Generate consecutive millisecond samples
		for i := 0; i < N; i++ {
			ts := baseTimestamp + uint64(i)*1000000 // 1ms apart
			s := simulateSubmarineCable(ts, rng)
			samples[i] = float64(s)
			sum += samples[i]
		}

		mean := sum / N

		// Compute autocorrelation at various lags
		// No lag should have correlation > 0.5
		var variance float64
		for i := 0; i < N; i++ {
			diff := samples[i] - mean
			variance += diff * diff
		}
		variance /= N

		if variance == 0 {
			// All same values — this is periodic by definition, but extremely unlikely
			// with random components. Skip this edge case.
			return
		}

		for lag := 1; lag <= 50; lag++ {
			var autoCorr float64
			count := N - lag
			for i := 0; i < count; i++ {
				autoCorr += (samples[i] - mean) * (samples[i+lag] - mean)
			}
			autoCorr /= float64(count) * variance

			if autoCorr > 0.5 {
				t.Fatalf("significant autocorrelation at lag %d: %.3f > 0.5", lag, autoCorr)
			}
		}
	})
}
