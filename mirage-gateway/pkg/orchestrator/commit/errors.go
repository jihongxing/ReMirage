// Package commit - 错误类型定义
package commit

import "fmt"

// ErrTxConflict 事务冲突
type ErrTxConflict struct {
	ExistingTxID   string
	ExistingTxType TxType
}

func (e *ErrTxConflict) Error() string {
	return fmt.Sprintf("tx conflict: existing tx %s (%s) is active", e.ExistingTxID, e.ExistingTxType)
}

// ErrCooldownActive 冷却时间未到
type ErrCooldownActive struct {
	TxType           TxType
	RemainingSeconds float64
}

func (e *ErrCooldownActive) Error() string {
	return fmt.Sprintf("cooldown active for %s: %.1f seconds remaining", e.TxType, e.RemainingSeconds)
}

// ErrInvalidPhaseTransition 非法阶段转换
type ErrInvalidPhaseTransition struct {
	From TxPhase
	To   TxPhase
}

func (e *ErrInvalidPhaseTransition) Error() string {
	return fmt.Sprintf("invalid phase transition from %s to %s", e.From, e.To)
}

// ErrTerminalPhase 终态后尝试转换
type ErrTerminalPhase struct {
	Phase TxPhase
}

func (e *ErrTerminalPhase) Error() string {
	return fmt.Sprintf("transaction in terminal phase: %s", e.Phase)
}

// ErrSessionNotFound 目标 Session 不存在
type ErrSessionNotFound struct {
	SessionID string
}

func (e *ErrSessionNotFound) Error() string {
	return fmt.Sprintf("session not found: %s", e.SessionID)
}

// ErrLinkNotFound 目标 Link 不存在
type ErrLinkNotFound struct {
	LinkID string
}

func (e *ErrLinkNotFound) Error() string {
	return fmt.Sprintf("link not found: %s", e.LinkID)
}
