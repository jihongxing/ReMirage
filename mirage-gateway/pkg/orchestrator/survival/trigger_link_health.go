package survival

import (
	"context"
	"fmt"
	"time"

	orchestrator "mirage-gateway/pkg/orchestrator"
)

type linkHealthTrigger struct{}

// NewLinkHealthTrigger 创建 LinkHealthTrigger
func NewLinkHealthTrigger() LinkHealthTriggerIface {
	return &linkHealthTrigger{}
}

func (t *linkHealthTrigger) Evaluate(_ context.Context, links []*orchestrator.LinkState) *TriggerSignal {
	if len(links) == 0 {
		return nil
	}

	var sum float64
	for _, l := range links {
		sum += l.HealthScore
	}
	avg := sum / float64(len(links))

	var targetMode orchestrator.SurvivalMode
	var severity int

	switch {
	case avg < 10:
		targetMode = orchestrator.SurvivalModeEscape
		severity = ModeSeverity[orchestrator.SurvivalModeEscape]
	case avg < 30:
		targetMode = orchestrator.SurvivalModeDegraded
		severity = ModeSeverity[orchestrator.SurvivalModeDegraded]
	case avg < 60:
		targetMode = orchestrator.SurvivalModeHardened
		severity = ModeSeverity[orchestrator.SurvivalModeHardened]
	default:
		return nil
	}

	return &TriggerSignal{
		Source:    TriggerSourceLinkHealth,
		Reason:    fmt.Sprintf("avg link health %.2f → %s", avg, targetMode),
		Severity:  severity,
		Timestamp: time.Now(),
		Metadata: map[string]interface{}{
			"avg_health":  avg,
			"link_count":  len(links),
			"target_mode": string(targetMode),
		},
	}
}
