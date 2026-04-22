// Package orchestrator - V2 三层状态模型编排器
package orchestrator

import "fmt"

// ErrInvalidTransition 非法状态转换
type ErrInvalidTransition struct {
	From string
	To   string
}

func (e *ErrInvalidTransition) Error() string {
	return fmt.Sprintf("invalid state transition from %s to %s", e.From, e.To)
}

// ErrLinkNotFound 链路不存在
type ErrLinkNotFound struct {
	LinkID string
}

func (e *ErrLinkNotFound) Error() string {
	return fmt.Sprintf("link not found: %s", e.LinkID)
}

// ErrLinkUnavailable 链路不可用
type ErrLinkUnavailable struct {
	LinkID string
}

func (e *ErrLinkUnavailable) Error() string {
	return fmt.Sprintf("link unavailable: %s", e.LinkID)
}

// ErrSessionNotFound 会话不存在
type ErrSessionNotFound struct {
	SessionID string
}

func (e *ErrSessionNotFound) Error() string {
	return fmt.Sprintf("session not found: %s", e.SessionID)
}

// ErrOptimisticLockConflict 乐观锁冲突
var ErrOptimisticLockConflict = fmt.Errorf("optimistic lock conflict: record was modified by another transaction")
