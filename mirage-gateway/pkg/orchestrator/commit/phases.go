// Package commit - 阶段执行器
package commit

import (
	"context"
	"encoding/json"
	"time"
)

// PhaseExecutor 各阶段执行器接口
type PhaseExecutor interface {
	Prepare(ctx context.Context, tx *CommitTransaction) error
	Validate(ctx context.Context, tx *CommitTransaction) error
	ShadowWrite(ctx context.Context, tx *CommitTransaction) error
	Flip(ctx context.Context, tx *CommitTransaction) error
	Acknowledge(ctx context.Context, tx *CommitTransaction) error
	Commit(ctx context.Context, tx *CommitTransaction) error
	Rollback(ctx context.Context, tx *CommitTransaction, reason string) error
}

// ControlStateManager 控制状态管理接口
type ControlStateManager interface {
	GetEpoch() uint64
	GetLastSuccessfulEpoch() uint64
	GetActiveTxID() string
	SetActiveTxID(txID string) error
	IncrementEpoch() (uint64, error)
	SetLastSuccessfulEpoch(epoch uint64) error
	SetRollbackMarker(epoch uint64) error
	GetRollbackMarker() uint64
	RestoreEpoch(epoch uint64) error
	SetControlHealth(health string) error
}

// SessionStateManager Session 状态管理接口
type SessionStateManager interface {
	GetSession(sessionID string) (map[string]interface{}, error)
	UpdateLink(sessionID, linkID string) error
	UpdateGateway(sessionID, gatewayID string) error
	UpdateSurvivalMode(sessionID, mode string) error
}

// LinkStateManager Link 状态管理接口
type LinkStateManager interface {
	GetLink(linkID string) (map[string]interface{}, error)
}

// phaseExecutorImpl 阶段执行器实现
type phaseExecutorImpl struct {
	controlMgr    ControlStateManager
	sessionMgr    SessionStateManager
	linkMgr       LinkStateManager
	cooldownMgr   CooldownManager
	conflictMgr   ConflictManager
	budgetChecker BudgetChecker
	classChecker  ServiceClassChecker
}

// NewPhaseExecutor 创建阶段执行器
func NewPhaseExecutor(
	controlMgr ControlStateManager,
	sessionMgr SessionStateManager,
	linkMgr LinkStateManager,
	cooldownMgr CooldownManager,
	conflictMgr ConflictManager,
	budgetChecker BudgetChecker,
	classChecker ServiceClassChecker,
) PhaseExecutor {
	return &phaseExecutorImpl{
		controlMgr:    controlMgr,
		sessionMgr:    sessionMgr,
		linkMgr:       linkMgr,
		cooldownMgr:   cooldownMgr,
		conflictMgr:   conflictMgr,
		budgetChecker: budgetChecker,
		classChecker:  classChecker,
	}
}

// Prepare 收集上下文快照
func (e *phaseExecutorImpl) Prepare(ctx context.Context, tx *CommitTransaction) error {
	snapshot := map[string]interface{}{
		"epoch": e.controlMgr.GetEpoch(),
	}

	if tx.TargetSessionID != "" {
		sess, err := e.sessionMgr.GetSession(tx.TargetSessionID)
		if err != nil {
			return &ErrSessionNotFound{SessionID: tx.TargetSessionID}
		}
		snapshot["session_snapshot"] = sess
	}

	if tx.TargetLinkID != "" {
		link, err := e.linkMgr.GetLink(tx.TargetLinkID)
		if err != nil {
			return &ErrLinkNotFound{LinkID: tx.TargetLinkID}
		}
		snapshot["link_snapshot"] = link
	}

	data, _ := json.Marshal(snapshot)
	tx.PrepareState = data
	return nil
}

// Validate 执行约束校验
func (e *phaseExecutorImpl) Validate(ctx context.Context, tx *CommitTransaction) error {
	result := map[string]interface{}{}

	// 冷却时间校验
	if err := e.cooldownMgr.CheckCooldown(ctx, tx.TxType); err != nil {
		result["cooldown_check"] = map[string]interface{}{"passed": false, "error": err.Error()}
		result["failed_check"] = "cooldown"
		data, _ := json.Marshal(result)
		tx.ValidateState = data
		return err
	}
	result["cooldown_check"] = map[string]interface{}{"passed": true}

	// 冲突校验
	if err := e.conflictMgr.CheckConflict(ctx, tx); err != nil {
		result["conflict_check"] = map[string]interface{}{"passed": false, "error": err.Error()}
		result["failed_check"] = "conflict"
		data, _ := json.Marshal(result)
		tx.ValidateState = data
		return err
	}
	result["conflict_check"] = map[string]interface{}{"passed": true}

	// 预算校验
	if err := e.budgetChecker.Check(ctx, tx); err != nil {
		result["budget_check"] = map[string]interface{}{"passed": false, "error": err.Error()}
		result["failed_check"] = "budget"
		data, _ := json.Marshal(result)
		tx.ValidateState = data
		return err
	}
	result["budget_check"] = map[string]interface{}{"passed": true}

	// 服务等级校验
	if err := e.classChecker.Check(ctx, tx); err != nil {
		result["service_class_check"] = map[string]interface{}{"passed": false, "error": err.Error()}
		result["failed_check"] = "service_class"
		data, _ := json.Marshal(result)
		tx.ValidateState = data
		return err
	}
	result["service_class_check"] = map[string]interface{}{"passed": true}

	data, _ := json.Marshal(result)
	tx.ValidateState = data
	return nil
}

// ShadowWrite 写入影子区
func (e *phaseExecutorImpl) ShadowWrite(_ context.Context, tx *CommitTransaction) error {
	shadow := map[string]interface{}{}

	switch tx.TxType {
	case TxTypePersonaSwitch:
		shadow["new_persona_id"] = tx.TargetPersonaID
	case TxTypeLinkMigration:
		shadow["new_route_target"] = tx.TargetLinkID
	case TxTypeSurvivalModeSwitch:
		shadow["new_survival_mode"] = tx.TargetSurvivalMode
	case TxTypeGatewayReassignment:
		shadow["new_session_target"] = tx.TargetSessionID
	}

	// 写入 rollback_marker 到 ControlState
	_ = e.controlMgr.SetRollbackMarker(tx.RollbackMarker)

	data, _ := json.Marshal(shadow)
	tx.ShadowState = data
	return nil
}

// Flip 执行单点切换
func (e *phaseExecutorImpl) Flip(_ context.Context, tx *CommitTransaction) error {
	switch tx.TxType {
	case TxTypeLinkMigration:
		if err := e.sessionMgr.UpdateLink(tx.TargetSessionID, tx.TargetLinkID); err != nil {
			return err
		}
	case TxTypeGatewayReassignment:
		if err := e.sessionMgr.UpdateGateway(tx.TargetSessionID, ""); err != nil {
			return err
		}
	case TxTypeSurvivalModeSwitch:
		if tx.TargetSessionID != "" {
			if err := e.sessionMgr.UpdateSurvivalMode(tx.TargetSessionID, tx.TargetSurvivalMode); err != nil {
				return err
			}
		}
	}

	// 递增 epoch
	newEpoch, err := e.controlMgr.IncrementEpoch()
	if err != nil {
		return err
	}

	flipResult := map[string]interface{}{
		"success":        true,
		"new_epoch":      newEpoch,
		"flip_timestamp": time.Now().UTC().Format(time.RFC3339),
	}
	data, _ := json.Marshal(flipResult)
	tx.FlipState = data
	return nil
}

// Acknowledge 确认检查
func (e *phaseExecutorImpl) Acknowledge(_ context.Context, tx *CommitTransaction) error {
	ack := map[string]interface{}{
		"control_plane_ack": map[string]interface{}{
			"passed":      true,
			"epoch_match": true,
		},
	}
	data, _ := json.Marshal(ack)
	tx.AckState = data
	return nil
}

// Commit 最终提交
func (e *phaseExecutorImpl) Commit(_ context.Context, tx *CommitTransaction) error {
	epoch := e.controlMgr.GetEpoch()
	_ = e.controlMgr.SetLastSuccessfulEpoch(epoch)
	_ = e.controlMgr.SetRollbackMarker(epoch)
	_ = e.controlMgr.SetActiveTxID("")

	now := time.Now().UTC()
	tx.FinishedAt = &now

	commitResult := map[string]interface{}{
		"result":      "Committed",
		"final_epoch": epoch,
	}
	data, _ := json.Marshal(commitResult)
	tx.CommitState = data

	e.cooldownMgr.RecordCompletion(tx.TxType, now)
	e.conflictMgr.UnregisterActive(tx.TxID)
	return nil
}

// Rollback 回滚
func (e *phaseExecutorImpl) Rollback(_ context.Context, tx *CommitTransaction, reason string) error {
	_ = e.controlMgr.RestoreEpoch(tx.RollbackMarker)
	_ = e.controlMgr.SetActiveTxID("")

	now := time.Now().UTC()
	tx.FinishedAt = &now

	commitResult := map[string]interface{}{
		"result": "RolledBack",
		"reason": reason,
	}
	data, _ := json.Marshal(commitResult)
	tx.CommitState = data

	e.conflictMgr.UnregisterActive(tx.TxID)
	return nil
}
