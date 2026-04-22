// Package commit - 事务冲突管理器
package commit

import (
	"context"
	"sync"
)

// ConflictManager 事务冲突管理器接口
type ConflictManager interface {
	CheckConflict(ctx context.Context, newTx *CommitTransaction) error
	RegisterActive(tx *CommitTransaction)
	UnregisterActive(txID string)
}

// conflictManagerImpl 冲突管理器实现
type conflictManagerImpl struct {
	mu     sync.RWMutex
	active map[TxScope]*CommitTransaction // scope -> active tx
	// rollbackFn 用于抢占时回滚低优先级事务
	rollbackFn func(txID string) error
}

// NewConflictManager 创建冲突管理器
func NewConflictManager(rollbackFn func(txID string) error) ConflictManager {
	return &conflictManagerImpl{
		active:     make(map[TxScope]*CommitTransaction),
		rollbackFn: rollbackFn,
	}
}

// CheckConflict 检查冲突，高优先级可抢占低优先级
func (m *conflictManagerImpl) CheckConflict(_ context.Context, newTx *CommitTransaction) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	newPriority := TxScopePriority[newTx.TxScope]

	// 检查所有活跃事务
	for scope, existing := range m.active {
		if existing.TxID == newTx.TxID {
			continue // 跳过自身
		}
		existingPriority := TxScopePriority[scope]

		if newPriority > existingPriority {
			// 高优先级抢占
			if m.rollbackFn != nil {
				_ = m.rollbackFn(existing.TxID)
			}
			delete(m.active, scope)
		} else {
			return &ErrTxConflict{
				ExistingTxID:   existing.TxID,
				ExistingTxType: existing.TxType,
			}
		}
	}

	return nil
}

// RegisterActive 注册活跃事务
func (m *conflictManagerImpl) RegisterActive(tx *CommitTransaction) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.active[tx.TxScope] = tx
}

// UnregisterActive 注销活跃事务
func (m *conflictManagerImpl) UnregisterActive(txID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for scope, tx := range m.active {
		if tx.TxID == txID {
			delete(m.active, scope)
			return
		}
	}
}
