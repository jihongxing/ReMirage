package audit

import (
	"context"
	"encoding/json"

	"mirage-gateway/pkg/orchestrator"
	"mirage-gateway/pkg/orchestrator/commit"
)

// TimelineCollector 时间线采集器
type TimelineCollector interface {
	// OnSessionTransition Session 状态变更
	OnSessionTransition(ctx context.Context, sessionID string,
		from, to orchestrator.SessionPhase, reason, linkID, personaID string,
		mode orchestrator.SurvivalMode) error

	// OnLinkHealthUpdate Link 健康更新
	OnLinkHealthUpdate(ctx context.Context, linkID string,
		score float64, rttMs int64, lossRate float64, jitterMs int64,
		phase orchestrator.LinkPhase) error

	// OnLinkPhaseTransition Link 阶段变更
	OnLinkPhaseTransition(ctx context.Context, linkID string,
		score float64, rttMs int64, lossRate float64, jitterMs int64,
		phase orchestrator.LinkPhase) error

	// OnPersonaSwitch Persona 切换
	OnPersonaSwitch(ctx context.Context, sessionID, personaID string,
		fromVersion, toVersion uint64) error

	// OnPersonaRollback Persona 回滚
	OnPersonaRollback(ctx context.Context, sessionID, personaID string,
		fromVersion, toVersion uint64) error

	// OnModeTransition Survival Mode 迁移
	OnModeTransition(ctx context.Context,
		from, to orchestrator.SurvivalMode,
		triggers json.RawMessage, txID string) error

	// OnTxPhaseTransition Transaction 阶段推进
	OnTxPhaseTransition(ctx context.Context, txID string,
		from, to commit.TxPhase, phaseData json.RawMessage) error
}
