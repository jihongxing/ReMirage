package audit

import (
	"context"

	"mirage-gateway/pkg/orchestrator"
	"mirage-gateway/pkg/orchestrator/commit"
	"mirage-gateway/pkg/orchestrator/survival"
)

// DiagnosticAggregator 诊断聚合器
type DiagnosticAggregator interface {
	// GetSessionDiagnostic 获取 Session 诊断视图
	GetSessionDiagnostic(ctx context.Context, sessionID string) (*SessionDiagnostic, error)

	// GetSystemDiagnostic 获取系统诊断视图
	GetSystemDiagnostic(ctx context.Context) (*SystemDiagnostic, error)

	// GetTransactionDiagnostic 获取事务诊断视图
	GetTransactionDiagnostic(ctx context.Context, txID string) (*TransactionDiagnostic, error)
}

// SessionProvider provides session state queries
type SessionProvider interface {
	Get(ctx context.Context, sessionID string) (*orchestrator.SessionState, error)
	ListByFilter(ctx context.Context, filter orchestrator.SessionFilter) ([]*orchestrator.SessionState, error)
}

// LinkProvider provides link state queries
type LinkProvider interface {
	Get(ctx context.Context, linkID string) (*orchestrator.LinkState, error)
}

// ControlProvider provides control state queries
type ControlProvider interface {
	GetOrCreate(ctx context.Context, gatewayID string) (*orchestrator.ControlState, error)
}

// TxProvider provides transaction queries for diagnostics
type TxProvider interface {
	GetTransaction(ctx context.Context, txID string) (*commit.CommitTransaction, error)
	GetActiveTransactions(ctx context.Context) ([]*commit.CommitTransaction, error)
}

// SurvivalProvider provides survival mode queries
type SurvivalProvider interface {
	GetCurrentMode() orchestrator.SurvivalMode
	GetTransitionHistory(n int) []*survival.TransitionRecord
}
