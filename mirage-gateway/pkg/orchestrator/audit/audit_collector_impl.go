package audit

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"mirage-gateway/pkg/orchestrator/budget"
	"mirage-gateway/pkg/orchestrator/commit"
	"mirage-gateway/pkg/orchestrator/events"
)

// auditCollectorImpl AuditCollector 实现
type auditCollectorImpl struct {
	store          AuditStore
	txProvider     TransactionProvider
	budgetProvider BudgetDecisionProvider
	eventType      events.EventType
}

// NewAuditCollector 创建 AuditCollector 实例
// 由于 EventHandler 接口要求单一 EventType()，需要为 EventRollbackDone 和 EventBudgetReject 各创建一个实例
func NewAuditCollector(
	store AuditStore,
	txProvider TransactionProvider,
	budgetProvider BudgetDecisionProvider,
	eventType events.EventType,
) AuditCollector {
	return &auditCollectorImpl{
		store:          store,
		txProvider:     txProvider,
		budgetProvider: budgetProvider,
		eventType:      eventType,
	}
}

// OnTransactionFinished 当 CommitTransaction 到达终态时生成 AuditRecord
func (c *auditCollectorImpl) OnTransactionFinished(ctx context.Context, tx *commit.CommitTransaction) error {
	// 1. 非终态直接返回
	if !commit.IsTerminal(tx.TxPhase) {
		return nil
	}

	// 2. 生成 UUID
	auditID := uuid.New().String()

	// 3. 映射基础字段
	finishedAt := time.Now().UTC()
	if tx.FinishedAt != nil {
		finishedAt = *tx.FinishedAt
	}

	record := &AuditRecord{
		AuditID:     auditID,
		TxID:        tx.TxID,
		TxType:      tx.TxType,
		InitiatedAt: tx.CreatedAt,
		FinishedAt:  finishedAt,
		TargetState: json.RawMessage(`{}`),
	}

	// 4. 根据 TxPhase 设置 flip_success / rollback_triggered
	switch tx.TxPhase {
	case commit.TxPhaseCommitted:
		record.FlipSuccess = true
		record.RollbackTriggered = false
	case commit.TxPhaseRolledBack:
		record.FlipSuccess = false
		record.RollbackTriggered = true
	case commit.TxPhaseFailed:
		record.FlipSuccess = false
		record.RollbackTriggered = false
	}

	// 5. 查询 BudgetDecisionProvider
	decision, err := c.budgetProvider.GetLastDecision(ctx, tx.TxID)
	if err == nil && decision != nil {
		record.BudgetVerdict = string(decision.Verdict)
		if decision.Verdict == budget.VerdictDenyAndHold || decision.Verdict == budget.VerdictDenyAndSuspend {
			record.DenyReason = decision.DenyReason
		}
	}
	// 查询失败时 budget_verdict 留空，不阻塞

	// 6. 保存
	return c.store.Save(ctx, record)
}

// Handle 实现 events.EventHandler 接口
func (c *auditCollectorImpl) Handle(ctx context.Context, event *events.ControlEvent) error {
	// 1. 从 PayloadRef 获取 tx_id
	txID := event.PayloadRef

	// 2. 查询事务
	tx, err := c.txProvider.GetTransaction(ctx, txID)
	if err != nil {
		return err
	}

	// 3. 调用 OnTransactionFinished
	return c.OnTransactionFinished(ctx, tx)
}

// EventType 返回处理的事件类型
func (c *auditCollectorImpl) EventType() events.EventType {
	return c.eventType
}
