package survival

import (
	"context"
	"fmt"
	"time"
)

type entryBurnTrigger struct{}

// NewEntryBurnTrigger 创建 EntryBurnTrigger
func NewEntryBurnTrigger() EntryBurnTriggerIface {
	return &entryBurnTrigger{}
}

func (t *entryBurnTrigger) Evaluate(_ context.Context, burnCount int, threshold int) *TriggerSignal {
	if burnCount <= threshold {
		return nil
	}
	return &TriggerSignal{
		Source:    TriggerSourceEntryBurn,
		Reason:    fmt.Sprintf("entry burn count %d exceeds threshold %d", burnCount, threshold),
		Severity:  ModeSeverity[SeverityToMode[4]], // Escape level
		Timestamp: time.Now(),
		Metadata: map[string]interface{}{
			"burn_count": burnCount,
			"threshold":  threshold,
		},
	}
}
