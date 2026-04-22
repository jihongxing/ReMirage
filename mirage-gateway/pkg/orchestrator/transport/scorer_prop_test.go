package transport

import (
	"math"
	"testing"

	orchestrator "mirage-gateway/pkg/orchestrator"

	"pgregory.net/rapid"
)

// Property 8: 路径评分公式正确性
func TestProperty8_PathScorerFormulaCorrectness(t *testing.T) {
	scorer := NewPathScorer()

	rapid.Check(t, func(t *rapid.T) {
		healthScore := rapid.Float64Range(0, 100).Draw(t, "health_score")
		rttMs := rapid.Int64Range(0, 1000).Draw(t, "rtt_ms")
		lossRate := rapid.Float64Range(0, 1).Draw(t, "loss_rate")
		jitterMs := rapid.Int64Range(0, 500).Draw(t, "jitter_ms")

		link := &orchestrator.LinkState{
			HealthScore: healthScore,
			RttMs:       rttMs,
			LossRate:    lossRate,
			JitterMs:    jitterMs,
		}
		policy := &TransportPolicy{}

		actual := scorer.Score(link, policy)

		rttNorm := math.Min(float64(rttMs)/500.0, 1.0)
		jitterNorm := math.Min(float64(jitterMs)/200.0, 1.0)
		expected := healthScore*0.40 +
			(1.0-rttNorm)*25.0 +
			(1.0-lossRate)*25.0 +
			(1.0-jitterNorm)*10.0

		if math.Abs(actual-expected) > 0.01 {
			t.Fatalf("score mismatch: expected %.4f, got %.4f (health=%.2f rtt=%d loss=%.4f jitter=%d)",
				expected, actual, healthScore, rttMs, lossRate, jitterMs)
		}
	})
}
