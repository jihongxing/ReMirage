package survival

import (
	"context"
	"fmt"
	"time"

	orchestrator "mirage-gateway/pkg/orchestrator"
)

type policyTrigger struct{}

// NewPolicyTrigger 创建 PolicyTrigger
func NewPolicyTrigger() PolicyTriggerIface {
	return &policyTrigger{}
}

func (t *policyTrigger) Evaluate(_ context.Context, targetMode orchestrator.SurvivalMode, reason string) *TriggerSignal {
	severity, ok := ModeSeverity[targetMode]
	if !ok {
		return nil
	}
	return &TriggerSignal{
		Source:    TriggerSourcePolicy,
		Reason:    fmt.Sprintf("policy directive: %s (%s)", targetMode, reason),
		Severity:  severity,
		Timestamp: time.Now(),
		Metadata: map[string]interface{}{
			"target_mode": string(targetMode),
			"reason":      reason,
		},
	}
}
