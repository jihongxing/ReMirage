package audit

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"mirage-gateway/pkg/orchestrator"
	"mirage-gateway/pkg/orchestrator/commit"
)

// timelineCollectorImpl TimelineCollector 实现
type timelineCollectorImpl struct {
	store TimelineStore
}

// NewTimelineCollector 创建 TimelineCollector 实例
func NewTimelineCollector(store TimelineStore) TimelineCollector {
	return &timelineCollectorImpl{store: store}
}

// OnSessionTransition Session 状态变更
func (c *timelineCollectorImpl) OnSessionTransition(ctx context.Context, sessionID string,
	from, to orchestrator.SessionPhase, reason, linkID, personaID string,
	mode orchestrator.SurvivalMode) error {

	entry := &SessionTimelineEntry{
		EntryID:      uuid.New().String(),
		SessionID:    sessionID,
		FromState:    from,
		ToState:      to,
		Reason:       reason,
		LinkID:       linkID,
		PersonaID:    personaID,
		SurvivalMode: mode,
		Timestamp:    time.Now().UTC(),
	}
	return c.store.SaveSessionEntry(ctx, entry)
}

// OnLinkHealthUpdate Link 健康更新
func (c *timelineCollectorImpl) OnLinkHealthUpdate(ctx context.Context, linkID string,
	score float64, rttMs int64, lossRate float64, jitterMs int64,
	phase orchestrator.LinkPhase) error {

	entry := &LinkHealthTimelineEntry{
		EntryID:     uuid.New().String(),
		LinkID:      linkID,
		HealthScore: score,
		RTTMs:       rttMs,
		LossRate:    lossRate,
		JitterMs:    jitterMs,
		Phase:       phase,
		EventType:   "health_update",
		Timestamp:   time.Now().UTC(),
	}
	return c.store.SaveLinkHealthEntry(ctx, entry)
}

// OnLinkPhaseTransition Link 阶段变更
func (c *timelineCollectorImpl) OnLinkPhaseTransition(ctx context.Context, linkID string,
	score float64, rttMs int64, lossRate float64, jitterMs int64,
	phase orchestrator.LinkPhase) error {

	entry := &LinkHealthTimelineEntry{
		EntryID:     uuid.New().String(),
		LinkID:      linkID,
		HealthScore: score,
		RTTMs:       rttMs,
		LossRate:    lossRate,
		JitterMs:    jitterMs,
		Phase:       phase,
		EventType:   "phase_transition",
		Timestamp:   time.Now().UTC(),
	}
	return c.store.SaveLinkHealthEntry(ctx, entry)
}

// OnPersonaSwitch Persona 切换
func (c *timelineCollectorImpl) OnPersonaSwitch(ctx context.Context, sessionID, personaID string,
	fromVersion, toVersion uint64) error {

	entry := &PersonaVersionTimelineEntry{
		EntryID:     uuid.New().String(),
		SessionID:   sessionID,
		PersonaID:   personaID,
		FromVersion: fromVersion,
		ToVersion:   toVersion,
		EventType:   "switch",
		Timestamp:   time.Now().UTC(),
	}
	return c.store.SavePersonaVersionEntry(ctx, entry)
}

// OnPersonaRollback Persona 回滚
func (c *timelineCollectorImpl) OnPersonaRollback(ctx context.Context, sessionID, personaID string,
	fromVersion, toVersion uint64) error {

	entry := &PersonaVersionTimelineEntry{
		EntryID:     uuid.New().String(),
		SessionID:   sessionID,
		PersonaID:   personaID,
		FromVersion: fromVersion,
		ToVersion:   toVersion,
		EventType:   "rollback",
		Timestamp:   time.Now().UTC(),
	}
	return c.store.SavePersonaVersionEntry(ctx, entry)
}

// OnModeTransition Survival Mode 迁移
func (c *timelineCollectorImpl) OnModeTransition(ctx context.Context,
	from, to orchestrator.SurvivalMode,
	triggers json.RawMessage, txID string) error {

	entry := &SurvivalModeTimelineEntry{
		EntryID:   uuid.New().String(),
		FromMode:  from,
		ToMode:    to,
		Triggers:  triggers,
		TxID:      txID,
		Timestamp: time.Now().UTC(),
	}
	return c.store.SaveSurvivalModeEntry(ctx, entry)
}

// OnTxPhaseTransition Transaction 阶段推进
func (c *timelineCollectorImpl) OnTxPhaseTransition(ctx context.Context, txID string,
	from, to commit.TxPhase, phaseData json.RawMessage) error {

	entry := &TransactionTimelineEntry{
		EntryID:   uuid.New().String(),
		TxID:      txID,
		FromPhase: from,
		ToPhase:   to,
		PhaseData: phaseData,
		Timestamp: time.Now().UTC(),
	}
	return c.store.SaveTransactionEntry(ctx, entry)
}
