package strategy

import (
	"testing"
	"time"

	"pgregory.net/rapid"
)

// Feature: zero-signature-elimination, Property 14: 策略引擎参数随机偏移范围
// **Validates: Requirements 17.1, 17.2, 17.3**

func TestProperty_StrategyEngineParamRandomOffset(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		level := DefenseLevel(rapid.IntRange(1, 5).Draw(t, "level"))
		base := levelToParams(level)

		engine := NewStrategyEngine(nil)
		// Force level and regenerate
		engine.mu.Lock()
		engine.currentLevel = level
		engine.cachedParams = engine.regenerateParams()
		engine.mu.Unlock()

		params := engine.GetParams()

		// Property: each param should be within [0.8*base, 1.2*base]
		checkRange := func(name string, val, baseVal uint32) {
			lo := float64(baseVal) * 0.8
			hi := float64(baseVal) * 1.2
			if float64(val) < lo || float64(val) > hi {
				t.Fatalf("%s=%d outside [%.0f, %.0f] (base=%d)", name, val, lo, hi, baseVal)
			}
		}

		checkRange("JitterMeanUs", params.JitterMeanUs, base.JitterMeanUs)
		checkRange("JitterStddevUs", params.JitterStddevUs, base.JitterStddevUs)
		checkRange("NoiseIntensity", params.NoiseIntensity, base.NoiseIntensity)
		checkRange("PaddingRate", params.PaddingRate, base.PaddingRate)

		// Property: same level, multiple GetParams calls return same value
		params2 := engine.GetParams()
		if params.JitterMeanUs != params2.JitterMeanUs ||
			params.JitterStddevUs != params2.JitterStddevUs ||
			params.NoiseIntensity != params2.NoiseIntensity ||
			params.PaddingRate != params2.PaddingRate {
			t.Fatalf("GetParams returned different values for same level")
		}

		// Property: Level field preserved
		if params.Level != level {
			t.Fatalf("Level=%d, expected %d", params.Level, level)
		}
	})
}

// Feature: zero-signature-elimination, Property 19: 策略调整间隔随机化范围
// **Validates: Requirements 22.1, 22.2**

func TestProperty_StrategyAdjustIntervalRange(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		interval := randomAdjustInterval()

		if interval < 8*time.Second || interval > 15*time.Second {
			t.Fatalf("randomAdjustInterval()=%v outside [8s, 15s]", interval)
		}
	})
}
