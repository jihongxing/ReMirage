package audit

import (
	"context"

	"mirage-gateway/pkg/orchestrator/budget"
	"mirage-gateway/pkg/orchestrator/commit"
	"mirage-gateway/pkg/orchestrator/events"
)

// TransactionProvider 事务查询接口（依赖 Spec 4-3 CommitEngine）
type TransactionProvider interface {
	GetTransaction(ctx context.Context, txID string) (*commit.CommitTransaction, error)
}

// BudgetDecisionProvider 预算判定查询接口（依赖 Spec 5-1）
type BudgetDecisionProvider interface {
	GetLastDecision(ctx context.Context, txID string) (*budget.BudgetDecision, error)
}

// AuditCollector 审计采集器
// 实现 events.EventHandler 接口，处理 EventRollbackDone 和 EventBudgetReject
type AuditCollector interface {
	// OnTransactionFinished 当 CommitTransaction 到达终态时调用
	OnTransactionFinished(ctx context.Context, tx *commit.CommitTransaction) error

	// Handle 实现 EventHandler 接口
	Handle(ctx context.Context, event *events.ControlEvent) error

	// EventType 返回处理的事件类型（注册多个时需要多个实例）
	EventType() events.EventType
}
