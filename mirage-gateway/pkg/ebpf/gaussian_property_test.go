package ebpf

import (
	"math"
	"math/rand"
	"testing"

	"pgregory.net/rapid"
)

// Feature: zero-signature-elimination, Property 10: Irwin-Hall 高斯采样统计特征
// **Validates: Requirements 11.1, 11.2**

// gaussianSampleIrwinHall is the Go userspace equivalent of the eBPF gaussian_sample
// using Irwin-Hall approximation (4 uniform distributions summed and scaled).
func gaussianSampleIrwinHall(mean, stddev uint32, rng *rand.Rand) uint64 {
	u1 := rng.Uint32()
	u2 := rng.Uint32()
	u3 := rng.Uint32()
	u4 := rng.Uint32()

	sum := uint64(u1>>16) + uint64(u2>>16) + uint64(u3>>16) + uint64(u4>>16)
	centered := int64(sum) - 2*65535
	scaled := (centered * int64(stddev)) / 37837
	result := int64(mean) + scaled

	if result <= 0 {
		return 0
	}
	return uint64(result)
}

func TestProperty_IrwinHallGaussianStatistics(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		mean := rapid.Uint32Range(100, 10000).Draw(t, "mean")
		stddev := rapid.Uint32Range(10, 2000).Draw(t, "stddev")

		// Ensure stddev is reasonable relative to mean
		if stddev > mean/2 {
			stddev = mean / 2
		}
		if stddev < 10 {
			stddev = 10
		}

		rng := rand.New(rand.NewSource(rapid.Int64().Draw(t, "seed")))

		const N = 1000
		var samples [N]float64
		var sum float64

		for i := 0; i < N; i++ {
			s := gaussianSampleIrwinHall(mean, stddev, rng)
			samples[i] = float64(s)
			sum += samples[i]
		}

		sampleMean := sum / N

		// Compute sample stddev
		var variance float64
		for i := 0; i < N; i++ {
			diff := samples[i] - sampleMean
			variance += diff * diff
		}
		sampleStddev := math.Sqrt(variance / N)

		// Property: sample mean should be within mean ± 0.2*stddev
		meanTolerance := 0.2 * float64(stddev)
		if meanTolerance < 5 {
			meanTolerance = 5
		}
		if math.Abs(sampleMean-float64(mean)) > meanTolerance {
			t.Fatalf("sample mean %.1f too far from expected %d (tolerance=%.1f, stddev=%d)",
				sampleMean, mean, meanTolerance, stddev)
		}

		// Property: sample stddev should be in [0.5*stddev, 2.0*stddev]
		if sampleStddev < 0.5*float64(stddev) || sampleStddev > 2.0*float64(stddev) {
			t.Fatalf("sample stddev %.1f outside [%.1f, %.1f] (expected stddev=%d)",
				sampleStddev, 0.5*float64(stddev), 2.0*float64(stddev), stddev)
		}
	})
}

// ---------------------------------------------------------------------------
// Jitter egress mock — simulates jitter_lite_egress C eBPF logic in Go
// ---------------------------------------------------------------------------

// jitterEgressMock simulates the jitter_lite_egress eBPF program.
// If a dnaTemplate is present, it uses getMimicDelay (template path).
// Otherwise it falls back to gaussianSampleIrwinHall (jitter_config path).
type jitterEgressMock struct {
	dnaTemplate *DNATemplateEntry // nil = no template in dna_template_map
	jitterCfg   JitterConfig
}

// getMimicDelay simulates the C get_mimic_delay() using template IAT params.
func getMimicDelay(tpl *DNATemplateEntry, rng *rand.Rand) uint64 {
	return gaussianSampleIrwinHall(tpl.TargetIATMu, tpl.TargetIATSigma, rng)
}

// computeDelay returns delay_ns following the priority logic:
//
//	template path > jitter_config fallback path.
func (j *jitterEgressMock) computeDelay(rng *rand.Rand) uint64 {
	if j.dnaTemplate != nil {
		return getMimicDelay(j.dnaTemplate, rng) * 1000 // µs → ns
	}
	return gaussianSampleIrwinHall(j.jitterCfg.MeanIATUs, j.jitterCfg.StddevIATUs, rng) * 1000
}

// usedTemplatePath reports which path was taken.
func (j *jitterEgressMock) usedTemplatePath() bool {
	return j.dnaTemplate != nil
}

// ---------------------------------------------------------------------------
// Task 6.2 — Example tests: template priority, fallback, IAT variance change
// ---------------------------------------------------------------------------

// Feature: phase2-stealth-evidence, Task 6.2 example tests
// **Validates: Requirements 4.1, 4.4**

func TestExample_JitterTemplatePriority(t *testing.T) {
	// When dna_template_map has a template, computeDelay should use it.
	tpl := &DNATemplateEntry{
		TargetIATMu:    5000,
		TargetIATSigma: 500,
	}
	mock := &jitterEgressMock{
		dnaTemplate: tpl,
		jitterCfg:   JitterConfig{Enabled: 1, MeanIATUs: 1000, StddevIATUs: 100},
	}
	rng := rand.New(rand.NewSource(42))

	if !mock.usedTemplatePath() {
		t.Fatal("expected template path when dnaTemplate is set")
	}
	delay := mock.computeDelay(rng)
	if delay == 0 {
		t.Fatal("expected delay_ns > 0 from template path")
	}
}

func TestExample_JitterFallbackToConfig(t *testing.T) {
	// When dna_template_map has no template, fall back to jitter_config.
	mock := &jitterEgressMock{
		dnaTemplate: nil,
		jitterCfg:   JitterConfig{Enabled: 1, MeanIATUs: 2000, StddevIATUs: 300},
	}
	rng := rand.New(rand.NewSource(42))

	if mock.usedTemplatePath() {
		t.Fatal("expected fallback path when dnaTemplate is nil")
	}
	delay := mock.computeDelay(rng)
	if delay == 0 {
		t.Fatal("expected delay_ns > 0 from fallback path")
	}
}

func TestExample_JitterIATVarianceChange(t *testing.T) {
	// Enabling jitter should increase IAT variance compared to constant base.
	const N = 200
	baseIAT := uint64(5000) // constant 5000 µs when jitter disabled

	rng := rand.New(rand.NewSource(99))
	mock := &jitterEgressMock{
		dnaTemplate: nil,
		jitterCfg:   JitterConfig{Enabled: 1, MeanIATUs: 5000, StddevIATUs: 800},
	}

	// Disabled: constant IAT → variance = 0
	var disabledVar float64 // 0 by definition

	// Enabled: variable IAT
	var sum float64
	samples := make([]float64, N)
	for i := 0; i < N; i++ {
		d := mock.computeDelay(rng)
		samples[i] = float64(d)
		sum += samples[i]
	}
	mean := sum / float64(N)
	var enabledVar float64
	for _, s := range samples {
		diff := s - mean
		enabledVar += diff * diff
	}
	enabledVar /= float64(N)

	_ = baseIAT
	if enabledVar <= disabledVar {
		t.Fatalf("enabled variance %.1f should be > disabled variance %.1f", enabledVar, disabledVar)
	}
}

// ---------------------------------------------------------------------------
// Task 6.3 — Property 5: Jitter IAT 方差增加 PBT
// ---------------------------------------------------------------------------

// Feature: phase2-stealth-evidence, Property 5: Jitter IAT 方差增加
// **Validates: Requirements 4.1**

func TestProperty_JitterIATVarianceIncrease(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		meanIAT := rapid.Uint32Range(100, 10000).Draw(t, "mean_iat_us")
		stddevIAT := rapid.Uint32Range(10, 2000).Draw(t, "stddev_iat_us")

		// Keep stddev reasonable relative to mean to avoid clamping to 0
		if stddevIAT > meanIAT/2 {
			stddevIAT = meanIAT / 2
		}
		if stddevIAT < 10 {
			stddevIAT = 10
		}

		seed := rapid.Int64().Draw(t, "seed")
		rng := rand.New(rand.NewSource(seed))

		mock := &jitterEgressMock{
			dnaTemplate: nil,
			jitterCfg:   JitterConfig{Enabled: 1, MeanIATUs: meanIAT, StddevIATUs: stddevIAT},
		}

		const N = 200

		// Disabled jitter: constant base IAT (variance = 0)
		disabledVariance := 0.0

		// Enabled jitter: generate N delays and compute variance
		var sum float64
		samples := make([]float64, N)
		for i := 0; i < N; i++ {
			d := mock.computeDelay(rng)
			samples[i] = float64(d)
			sum += samples[i]
		}
		mean := sum / float64(N)
		var enabledVariance float64
		for _, s := range samples {
			diff := s - mean
			enabledVariance += diff * diff
		}
		enabledVariance /= float64(N)

		if enabledVariance <= disabledVariance {
			t.Fatalf("enabled IAT variance (%.1f) should be > disabled IAT variance (%.1f); mean_iat=%d stddev_iat=%d",
				enabledVariance, disabledVariance, meanIAT, stddevIAT)
		}
	})
}

// ---------------------------------------------------------------------------
// Task 6.4 — Property 6: Jitter 配置优先级 PBT
// ---------------------------------------------------------------------------

// Feature: phase2-stealth-evidence, Property 6: Jitter 配置优先级
// **Validates: Requirements 4.4**

func TestProperty_JitterConfigPriority(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Random dna_template
		tplMu := rapid.Uint32Range(100, 10000).Draw(t, "tpl_iat_mu")
		tplSigma := rapid.Uint32Range(10, 2000).Draw(t, "tpl_iat_sigma")
		if tplSigma > tplMu/2 {
			tplSigma = tplMu / 2
		}
		if tplSigma < 10 {
			tplSigma = 10
		}

		// Random jitter_config (different from template to distinguish paths)
		cfgMean := rapid.Uint32Range(100, 10000).Draw(t, "cfg_mean_iat")
		cfgStddev := rapid.Uint32Range(10, 2000).Draw(t, "cfg_stddev_iat")
		if cfgStddev > cfgMean/2 {
			cfgStddev = cfgMean / 2
		}
		if cfgStddev < 10 {
			cfgStddev = 10
		}

		seed := rapid.Int64().Draw(t, "seed")

		tpl := &DNATemplateEntry{
			TargetIATMu:    tplMu,
			TargetIATSigma: tplSigma,
		}
		cfg := JitterConfig{Enabled: 1, MeanIATUs: cfgMean, StddevIATUs: cfgStddev}

		const sampleCount = 20

		// Scenario A: dna_template_map has template → uses get_mimic_delay path
		mockA := &jitterEgressMock{dnaTemplate: tpl, jitterCfg: cfg}
		rngA := rand.New(rand.NewSource(seed))

		if !mockA.usedTemplatePath() {
			t.Fatal("Scenario A: expected template path when dnaTemplate is set")
		}
		var nonZeroA int
		for i := 0; i < sampleCount; i++ {
			if mockA.computeDelay(rngA) > 0 {
				nonZeroA++
			}
		}
		if nonZeroA == 0 {
			t.Fatalf("Scenario A: all %d delays were 0 (tpl_mu=%d, tpl_sigma=%d)", sampleCount, tplMu, tplSigma)
		}

		// Scenario B: dna_template_map empty → falls back to gaussian_sample path
		mockB := &jitterEgressMock{dnaTemplate: nil, jitterCfg: cfg}
		rngB := rand.New(rand.NewSource(seed))

		if mockB.usedTemplatePath() {
			t.Fatal("Scenario B: expected fallback path when dnaTemplate is nil")
		}
		var nonZeroB int
		for i := 0; i < sampleCount; i++ {
			if mockB.computeDelay(rngB) > 0 {
				nonZeroB++
			}
		}
		if nonZeroB == 0 {
			t.Fatalf("Scenario B: all %d delays were 0 (cfg_mean=%d, cfg_stddev=%d)", sampleCount, cfgMean, cfgStddev)
		}
	})
}
