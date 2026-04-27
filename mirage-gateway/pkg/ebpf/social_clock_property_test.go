package ebpf

import (
	"math"
	"testing"

	"pgregory.net/rapid"
)

// Feature: zero-signature-elimination, Property 20: 社交时钟渐变连续性
// **Validates: Requirements 23.1, 23.2, 23.3**

// socialClockConfig mirrors the C struct social_clock_config.
type socialClockConfig struct {
	Enabled          uint32
	PeakHourStart    uint32 // hour
	PeakHourEnd      uint32 // hour
	PeakMultiplier   uint32 // 100 = 1x
	NightMultiplier  uint32
	TransitionWindow uint32 // minutes, default 30
}

// intSigmoid is the Go equivalent of the eBPF int_sigmoid.
// Input x in [-1000, 1000], output in [0, 1000].
func intSigmoid(x int32) uint32 {
	absX := x
	if absX < 0 {
		absX = -absX
	}
	return uint32(500 + int64(x)*500/(1000+int64(absX)))
}

// getSocialClockFactor is the Go userspace equivalent of the eBPF get_social_clock_factor
// with sigmoid gradual transition.
func getSocialClockFactor(cfg *socialClockConfig, minuteOfDay uint32) uint32 {
	if cfg == nil || cfg.Enabled == 0 {
		return 100
	}

	tw := cfg.TransitionWindow
	if tw == 0 {
		tw = 30
	}
	halfTW := int32(tw / 2)
	if halfTW == 0 {
		halfTW = 1
	}

	peakStartMin := cfg.PeakHourStart * 60
	peakEndMin := cfg.PeakHourEnd * 60
	nightStartMin := uint32(22 * 60)
	nightEndMin := uint32(6 * 60)

	distPeakStart := int32(minuteOfDay) - int32(peakStartMin)
	distPeakEnd := int32(minuteOfDay) - int32(peakEndMin)
	distNightStart := int32(minuteOfDay) - int32(nightStartMin)
	distNightEnd := int32(minuteOfDay) - int32(nightEndMin)

	// Entering peak transition
	if distPeakStart > -halfTW && distPeakStart < halfTW {
		x := distPeakStart * 1000 / halfTW
		sig := intSigmoid(x)
		return 100 + (cfg.PeakMultiplier-100)*sig/1000
	}

	// Leaving peak transition
	if distPeakEnd > -halfTW && distPeakEnd < halfTW {
		x := distPeakEnd * 1000 / halfTW
		sig := intSigmoid(x)
		return cfg.PeakMultiplier - (cfg.PeakMultiplier-100)*sig/1000
	}

	// Inside peak
	if minuteOfDay >= peakStartMin && minuteOfDay < peakEndMin {
		return cfg.PeakMultiplier
	}

	// Entering night transition
	if distNightStart > -halfTW && distNightStart < halfTW {
		x := distNightStart * 1000 / halfTW
		sig := intSigmoid(x)
		return 100 + (cfg.NightMultiplier-100)*sig/1000
	}

	// Leaving night transition (across midnight)
	if minuteOfDay < nightEndMin {
		if distNightEnd > -halfTW && distNightEnd < halfTW {
			x := distNightEnd * 1000 / halfTW
			sig := intSigmoid(x)
			return cfg.NightMultiplier - (cfg.NightMultiplier-100)*sig/1000
		}
	}

	// Inside night (22:00 - 06:00)
	if minuteOfDay >= nightStartMin || minuteOfDay < nightEndMin {
		return cfg.NightMultiplier
	}

	return 100
}

func TestProperty_SocialClockSigmoidContinuity(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		peakStart := rapid.Uint32Range(8, 12).Draw(t, "peakStart")
		peakEnd := rapid.Uint32Range(peakStart+2, 20).Draw(t, "peakEnd")
		peakMult := rapid.Uint32Range(120, 300).Draw(t, "peakMult")
		nightMult := rapid.Uint32Range(30, 90).Draw(t, "nightMult")
		tw := rapid.Uint32Range(10, 60).Draw(t, "transitionWindow")

		cfg := &socialClockConfig{
			Enabled:          1,
			PeakHourStart:    peakStart,
			PeakHourEnd:      peakEnd,
			PeakMultiplier:   peakMult,
			NightMultiplier:  nightMult,
			TransitionWindow: tw,
		}

		// Test continuity: sample every minute across the peak start boundary
		// and verify no step change exceeds a threshold
		halfTW := int32(tw / 2)
		if halfTW == 0 {
			halfTW = 1
		}
		boundaryMin := peakStart * 60

		// Sample across the transition window
		startMin := int32(boundaryMin) - halfTW - 2
		endMin := int32(boundaryMin) + halfTW + 2
		if startMin < 0 {
			startMin = 0
		}
		if endMin > 1440 {
			endMin = 1440
		}

		prevFactor := getSocialClockFactor(cfg, uint32(startMin))
		maxStep := float64(0)

		for m := startMin + 1; m <= endMin; m++ {
			factor := getSocialClockFactor(cfg, uint32(m))
			step := math.Abs(float64(factor) - float64(prevFactor))
			if step > maxStep {
				maxStep = step
			}
			prevFactor = factor
		}

		// The maximum per-minute step should be bounded.
		// Key property: no abrupt jump (hard switch would be the full range in 1 minute).
		// Sigmoid ensures gradual transition. Allow up to range/3 per minute as generous bound.
		rangeVal := float64(peakMult - 100)
		if rangeVal < 3 {
			return // too small to meaningfully test
		}
		maxAllowed := rangeVal/3.0 + 5.0 // generous bound for integer rounding
		if maxStep > maxAllowed {
			t.Fatalf("step too large at peak_start boundary: maxStep=%.1f, allowed=%.1f (tw=%d, range=%.0f)",
				maxStep, maxAllowed, tw, rangeVal)
		}
	})
}

func TestProperty_SocialClockNoStepAtBoundary(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		peakStart := rapid.Uint32Range(8, 12).Draw(t, "peakStart")
		peakEnd := rapid.Uint32Range(peakStart+2, 20).Draw(t, "peakEnd")
		peakMult := rapid.Uint32Range(150, 300).Draw(t, "peakMult")
		tw := rapid.Uint32Range(20, 60).Draw(t, "transitionWindow")

		cfg := &socialClockConfig{
			Enabled:          1,
			PeakHourStart:    peakStart,
			PeakHourEnd:      peakEnd,
			PeakMultiplier:   peakMult,
			NightMultiplier:  50,
			TransitionWindow: tw,
		}

		// At the exact boundary minute, the factor should be approximately
		// midway between the two levels (sigmoid(0) = 0.5)
		boundaryMin := peakStart * 60
		factorAtBoundary := getSocialClockFactor(cfg, boundaryMin)

		// sigmoid(0) = 500/1000, so factor ≈ 100 + (peakMult-100)*0.5
		expected := 100 + (peakMult-100)/2
		diff := math.Abs(float64(factorAtBoundary) - float64(expected))

		// Allow ±5 for integer rounding
		if diff > 5 {
			t.Fatalf("factor at boundary minute %d = %d, expected ~%d (diff=%.0f)",
				boundaryMin, factorAtBoundary, expected, diff)
		}
	})
}
