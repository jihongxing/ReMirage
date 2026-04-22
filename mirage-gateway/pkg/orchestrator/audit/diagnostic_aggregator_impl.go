package audit

import (
	"context"
	"time"

	"mirage-gateway/pkg/orchestrator"
	"mirage-gateway/pkg/orchestrator/commit"
)

// diagnosticAggregatorImpl DiagnosticAggregator 实现
type diagnosticAggregatorImpl struct {
	sessions SessionProvider
	links    LinkProvider
	controls ControlProvider
	txs      TxProvider
	survival SurvivalProvider
	timeline TimelineStore
}

// NewDiagnosticAggregator 创建 DiagnosticAggregator 实例
func NewDiagnosticAggregator(
	sessions SessionProvider,
	links LinkProvider,
	controls ControlProvider,
	txs TxProvider,
	survivalProvider SurvivalProvider,
	timeline TimelineStore,
) DiagnosticAggregator {
	return &diagnosticAggregatorImpl{
		sessions: sessions,
		links:    links,
		controls: controls,
		txs:      txs,
		survival: survivalProvider,
		timeline: timeline,
	}
}

// GetSessionDiagnostic 获取 Session 诊断视图
func (a *diagnosticAggregatorImpl) GetSessionDiagnostic(ctx context.Context, sessionID string) (*SessionDiagnostic, error) {
	session, err := a.sessions.Get(ctx, sessionID)
	if err != nil {
		return nil, &ErrSessionNotFound{SessionID: sessionID}
	}

	diag := &SessionDiagnostic{
		SessionID:           session.SessionID,
		CurrentLinkID:       session.CurrentLinkID,
		CurrentPersonaID:    session.CurrentPersonaID,
		CurrentSurvivalMode: session.CurrentSurvivalMode,
		SessionState:        session.State,
	}

	// Get link info
	if session.CurrentLinkID != "" {
		link, err := a.links.Get(ctx, session.CurrentLinkID)
		if err == nil {
			diag.CurrentLinkPhase = link.Phase
			diag.LastSwitchReason = link.LastSwitchReason
		}
	}

	// Get control info
	control, err := a.controls.GetOrCreate(ctx, session.GatewayID)
	if err == nil {
		diag.CurrentPersonaVersion = control.PersonaVersion
		diag.LastRollbackReason = control.LastSwitchReason
	}

	return diag, nil
}

// GetSystemDiagnostic 获取系统诊断视图
func (a *diagnosticAggregatorImpl) GetSystemDiagnostic(ctx context.Context) (*SystemDiagnostic, error) {
	diag := &SystemDiagnostic{
		CurrentSurvivalMode: a.survival.GetCurrentMode(),
	}

	// Get last transition info
	history := a.survival.GetTransitionHistory(1)
	if len(history) > 0 {
		last := history[0]
		diag.LastModeSwitchReason = last.TxID
		ts := last.Timestamp
		diag.LastModeSwitchTime = &ts
	}

	// Count active sessions (non-Closed)
	allSessions, err := a.sessions.ListByFilter(ctx, orchestrator.SessionFilter{})
	if err == nil {
		activeCount := 0
		linkSet := make(map[string]struct{})
		for _, s := range allSessions {
			if s.State != orchestrator.SessionPhaseClosed {
				activeCount++
				if s.CurrentLinkID != "" {
					linkSet[s.CurrentLinkID] = struct{}{}
				}
			}
		}
		diag.ActiveSessionCount = activeCount
		diag.ActiveLinkCount = len(linkSet)
	}

	// Get active transactions
	activeTxs, err := a.txs.GetActiveTransactions(ctx)
	if err == nil && len(activeTxs) > 0 {
		tx := activeTxs[0]
		diag.ActiveTransaction = &ActiveTxInfo{
			TxID:    tx.TxID,
			TxType:  tx.TxType,
			TxPhase: tx.TxPhase,
		}
	}

	return diag, nil
}

// GetTransactionDiagnostic 获取事务诊断视图
func (a *diagnosticAggregatorImpl) GetTransactionDiagnostic(ctx context.Context, txID string) (*TransactionDiagnostic, error) {
	tx, err := a.txs.GetTransaction(ctx, txID)
	if err != nil {
		return nil, &ErrTransactionNotFound{TxID: txID}
	}

	diag := &TransactionDiagnostic{
		TxID:               tx.TxID,
		TxType:             tx.TxType,
		CurrentPhase:       tx.TxPhase,
		TargetSessionID:    tx.TargetSessionID,
		TargetSurvivalMode: orchestrator.SurvivalMode(tx.TargetSurvivalMode),
		PhaseDurations:     make(map[string]time.Duration),
	}

	// Get timeline entries for phase durations
	entries, err := a.timeline.ListTransactionEntries(ctx, txID)
	if err == nil && len(entries) > 0 {
		for i := 1; i < len(entries); i++ {
			phaseName := string(entries[i-1].ToPhase)
			duration := entries[i].Timestamp.Sub(entries[i-1].Timestamp)
			diag.PhaseDurations[phaseName] = duration
		}

		// Calculate stuck_duration
		if !commit.IsTerminal(tx.TxPhase) {
			lastEntry := entries[len(entries)-1]
			diag.StuckDuration = time.Since(lastEntry.Timestamp)
		}
	} else if !commit.IsTerminal(tx.TxPhase) {
		// No timeline entries but non-terminal: use CreatedAt
		diag.StuckDuration = time.Since(tx.CreatedAt)
	}

	return diag, nil
}
