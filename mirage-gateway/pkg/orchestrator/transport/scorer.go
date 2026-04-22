package transport

import (
	"math"

	orchestrator "mirage-gateway/pkg/orchestrator"
)

// PathScorerIface 路径评分器接口
type PathScorerIface interface {
	Score(link *orchestrator.LinkState, policy *TransportPolicy) float64
}

type defaultPathScorer struct{}

// NewPathScorer 创建 PathScorer
func NewPathScorer() PathScorerIface {
	return &defaultPathScorer{}
}

// Score 计算路径得分
// score = health_score * 0.40 + (1 - rtt_norm) * 25 + (1 - loss_rate) * 25 + (1 - jitter_norm) * 10
// rtt_norm = min(rtt_ms / 500.0, 1.0), jitter_norm = min(jitter_ms / 200.0, 1.0)
func (s *defaultPathScorer) Score(link *orchestrator.LinkState, _ *TransportPolicy) float64 {
	rttNorm := math.Min(float64(link.RttMs)/500.0, 1.0)
	jitterNorm := math.Min(float64(link.JitterMs)/200.0, 1.0)

	score := link.HealthScore*0.40 +
		(1.0-rttNorm)*25.0 +
		(1.0-link.LossRate)*25.0 +
		(1.0-jitterNorm)*10.0

	return score
}
