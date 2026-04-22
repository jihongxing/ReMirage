// Package commit - CommitEngine 主体实现
package commit

import (
	"context"
	"fmt"
	"time"
)

// BeginTxRequest 创建事务请求
type BeginTxRequest struct {
	TxType             TxType
	TargetSessionID    string
	TargetLinkID       string
	TargetPersonaID    string
	TargetSurvivalMode string
}

// TxFilter 事务查询过滤条件
type TxFilter struct {
	TxType          *TxType
	TxPhase         *TxPhase
	TargetSessionID *string
	CreatedAfter    *time.Time
	CreatedBefore   *time.Time
}

// CommitEngine 事务化状态提交引擎接口
type CommitEngine interface {
	BeginTransaction(ctx context.Context, req *BeginTxRequest) (*CommitTransaction, error)
	ExecuteTransaction(ctx context.Context, txID string) error
	RollbackTransaction(ctx context.Context, txID string, reason string) error
	GetTransaction(ctx context.Context, txID string) (*CommitTransaction, error)
	ListTransactions(ctx context.Context, filter *TxFilter) ([]*CommitTransaction, error)
	GetActiveTransactions(ctx context.Context) ([]*CommitTransaction, error)
	RecoverOnStartup(ctx context.Context) error
}

// TxStore 事务持久化接口
type TxStore interface {
	Save(tx *CommitTransaction) error
	Update(tx *CommitTransaction) error
	GetByID(txID string) (*CommitTransaction, error)
	List(filter *TxFilter) ([]*CommitTransaction, error)
	GetActive() ([]*CommitTransaction, error)
	GetIncomplete() ([]*CommitTransaction, error)
}

// commitEngineImpl CommitEngine 实现
type commitEngineImpl struct {
	controlMgr  ControlStateManager
	executor    PhaseExecutor
	cooldownMgr CooldownManager
	conflictMgr ConflictManager
	store       TxStore
}

// NewCommitEngine 创建 CommitEngine
func NewCommitEngine(
	controlMgr ControlStateManager,
	executor PhaseExecutor,
	cooldownMgr CooldownManager,
	conflictMgr ConflictManager,
	store TxStore,
) CommitEngine {
	return &commitEngineImpl{
		controlMgr:  controlMgr,
		executor:    executor,
		cooldownMgr: cooldownMgr,
		conflictMgr: conflictMgr,
		store:       store,
	}
}

// BeginTransaction 创建并启动新事务
func (e *commitEngineImpl) BeginTransaction(ctx context.Context, req *BeginTxRequest) (*CommitTransaction, error) {
	tx := NewCommitTransaction(req.TxType, e.controlMgr.GetLastSuccessfulEpoch())
	tx.TargetSessionID = req.TargetSessionID
	tx.TargetLinkID = req.TargetLinkID
	tx.TargetPersonaID = req.TargetPersonaID
	tx.TargetSurvivalMode = req.TargetSurvivalMode

	// 冲突检查
	if err := e.conflictMgr.CheckConflict(ctx, tx); err != nil {
		return nil, err
	}

	// 冷却时间检查
	if err := e.cooldownMgr.CheckCooldown(ctx, tx.TxType); err != nil {
		return nil, err
	}

	// 持久化
	if err := e.store.Save(tx); err != nil {
		return nil, fmt.Errorf("save tx: %w", err)
	}

	// 设置 active_tx_id
	_ = e.controlMgr.SetActiveTxID(tx.TxID)
	e.conflictMgr.RegisterActive(tx)

	return tx, nil
}

// ExecuteTransaction 执行完整六阶段提交流程
func (e *commitEngineImpl) ExecuteTransaction(ctx context.Context, txID string) error {
	tx, err := e.store.GetByID(txID)
	if err != nil {
		return err
	}

	// Prepare
	if err := e.advancePhase(ctx, tx, TxPhaseValidating, func() error {
		return e.executor.Prepare(ctx, tx)
	}); err != nil {
		return e.handleFailure(ctx, tx, "Prepare failed: "+err.Error())
	}

	// Validate
	if err := e.advancePhase(ctx, tx, TxPhaseShadowWriting, func() error {
		return e.executor.Validate(ctx, tx)
	}); err != nil {
		return e.handleFailure(ctx, tx, "Validate failed: "+err.Error())
	}

	// ShadowWrite
	if err := e.advancePhase(ctx, tx, TxPhaseFlipping, func() error {
		return e.executor.ShadowWrite(ctx, tx)
	}); err != nil {
		return e.handleRollback(ctx, tx, "ShadowWrite failed: "+err.Error())
	}

	// Flip
	if err := e.advancePhase(ctx, tx, TxPhaseAcknowledging, func() error {
		return e.executor.Flip(ctx, tx)
	}); err != nil {
		return e.handleRollback(ctx, tx, "Flip failed: "+err.Error())
	}

	// Acknowledge
	if err := e.advancePhase(ctx, tx, TxPhaseCommitted, func() error {
		return e.executor.Acknowledge(ctx, tx)
	}); err != nil {
		return e.handleRollback(ctx, tx, "Acknowledge failed: "+err.Error())
	}

	// Commit
	tx.TxPhase = TxPhaseCommitted
	if err := e.executor.Commit(ctx, tx); err != nil {
		return err
	}
	return e.store.Update(tx)
}

func (e *commitEngineImpl) advancePhase(_ context.Context, tx *CommitTransaction, nextPhase TxPhase, fn func() error) error {
	if err := fn(); err != nil {
		return err
	}
	_, err := TransitionPhase(tx.TxPhase, nextPhase)
	if err != nil {
		return err
	}
	tx.TxPhase = nextPhase
	return e.store.Update(tx)
}

func (e *commitEngineImpl) handleFailure(ctx context.Context, tx *CommitTransaction, reason string) error {
	tx.TxPhase = TxPhaseFailed
	now := time.Now().UTC()
	tx.FinishedAt = &now
	_ = e.controlMgr.SetActiveTxID("")
	e.conflictMgr.UnregisterActive(tx.TxID)
	_ = e.store.Update(tx)
	return fmt.Errorf("failure: %s", reason)
}

func (e *commitEngineImpl) handleRollback(ctx context.Context, tx *CommitTransaction, reason string) error {
	tx.TxPhase = TxPhaseRolledBack
	_ = e.executor.Rollback(ctx, tx, reason)
	_ = e.store.Update(tx)
	return fmt.Errorf("rollback: %s", reason)
}

// RollbackTransaction 手动回滚
func (e *commitEngineImpl) RollbackTransaction(ctx context.Context, txID string, reason string) error {
	tx, err := e.store.GetByID(txID)
	if err != nil {
		return err
	}
	tx.TxPhase = TxPhaseRolledBack
	return e.executor.Rollback(ctx, tx, reason)
}

// GetTransaction 查询事务详情
func (e *commitEngineImpl) GetTransaction(_ context.Context, txID string) (*CommitTransaction, error) {
	return e.store.GetByID(txID)
}

// ListTransactions 按条件查询
func (e *commitEngineImpl) ListTransactions(_ context.Context, filter *TxFilter) ([]*CommitTransaction, error) {
	return e.store.List(filter)
}

// GetActiveTransactions 查询活跃事务
func (e *commitEngineImpl) GetActiveTransactions(_ context.Context) ([]*CommitTransaction, error) {
	return e.store.GetActive()
}

// RecoverOnStartup 崩溃恢复
func (e *commitEngineImpl) RecoverOnStartup(ctx context.Context) error {
	incomplete, err := e.store.GetIncomplete()
	if err != nil {
		return err
	}

	for _, tx := range incomplete {
		tx.TxPhase = TxPhaseRolledBack
		_ = e.executor.Rollback(ctx, tx, "system restart recovery")
		_ = e.store.Update(tx)
	}

	// 恢复 epoch 到 rollback_marker
	marker := e.controlMgr.GetRollbackMarker()
	if err := e.controlMgr.RestoreEpoch(marker); err != nil {
		_ = e.controlMgr.SetControlHealth("Faulted")
		return fmt.Errorf("recovery failed: cannot restore epoch to %d: %w", marker, err)
	}

	_ = e.controlMgr.SetActiveTxID("")
	_ = e.controlMgr.SetControlHealth("Recovering")
	return nil
}
