package gtunnel

import (
	"testing"
	"time"

	"pgregory.net/rapid"
)

// Feature: multi-path-adaptive-transport, Property 5: 降格判定阈值正确性
func TestProperty_DemoteThreshold(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		maxLossRate := rapid.Float64Range(0.01, 0.99).Draw(t, "maxLossRate")
		maxRTTMultiple := rapid.Float64Range(1.1, 10.0).Draw(t, "maxRTTMultiple")

		lossRate := rapid.Float64Range(0.0, 1.0).Draw(t, "lossRate")
		baselineRTTMs := rapid.IntRange(1, 500).Draw(t, "baselineRTTMs")
		rttMs := rapid.IntRange(1, 2000).Draw(t, "rttMs")

		baselineRTT := time.Duration(baselineRTTMs) * time.Millisecond
		rtt := time.Duration(rttMs) * time.Millisecond

		m := &PathMetrics{
			LossRate:    lossRate,
			RTT:         rtt,
			BaselineRTT: baselineRTT,
		}

		la := &LinkAuditor{
			thresholds: AuditThresholds{
				MaxLossRate:    maxLossRate,
				MaxRTTMultiple: maxRTTMultiple,
			},
		}

		got := la.shouldDegradeMetrics(m)

		lossExceeded := lossRate > maxLossRate
		rttExceeded := baselineRTT > 0 && rtt > time.Duration(float64(baselineRTT)*maxRTTMultiple)
		expected := lossExceeded || rttExceeded

		if got != expected {
			t.Fatalf("ShouldDegrade mismatch: got=%v expected=%v loss=%.4f>%.4f=%v rtt=%v>%.1f*%v=%v",
				got, expected, lossRate, maxLossRate, lossExceeded, rtt, maxRTTMultiple, baselineRTT, rttExceeded)
		}
	})
}

// Feature: multi-path-adaptive-transport, Property 6: 升格判定连续成功计数
func TestProperty_PromoteConsecutiveSuccess(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		threshold := rapid.IntRange(1, 10).Draw(t, "threshold")
		seqLen := rapid.IntRange(0, 50).Draw(t, "seqLen")

		probeResults := make([]bool, seqLen)
		for i := range probeResults {
			probeResults[i] = rapid.Bool().Draw(t, "probe")
		}

		got := CheckShouldPromote(probeResults, threshold)

		// 手动验证：检查是否存在连续 >= threshold 个 true
		expected := false
		consecutive := 0
		for _, ok := range probeResults {
			if ok {
				consecutive++
				if consecutive >= threshold {
					expected = true
					break
				}
			} else {
				consecutive = 0
			}
		}

		if got != expected {
			t.Fatalf("CheckShouldPromote mismatch: got=%v expected=%v threshold=%d seq=%v",
				got, expected, threshold, probeResults)
		}
	})
}
