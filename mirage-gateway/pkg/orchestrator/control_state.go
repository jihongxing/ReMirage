package orchestrator

import (
	"context"
	"time"

	"gorm.io/gorm"
)

// ControlStateManager 控制状态管理器接口
type ControlStateManager interface {
	GetOrCreate(ctx context.Context, gatewayID string) (*ControlState, error)
	IncrementEpoch(ctx context.Context, gatewayID string) (uint64, error)
	BeginTransaction(ctx context.Context, gatewayID string, txID string) error
	CommitTransaction(ctx context.Context, gatewayID string, reason string) error
	RecoverOnStartup(ctx context.Context, gatewayID string) error
}

type controlStateManagerImpl struct {
	db   *gorm.DB
	lock *LockManager
}

// NewControlStateManager 创建 ControlStateManager 实例
func NewControlStateManager(db *gorm.DB, lock *LockManager) ControlStateManager {
	return &controlStateManagerImpl{db: db, lock: lock}
}

func (m *controlStateManagerImpl) GetOrCreate(ctx context.Context, gatewayID string) (*ControlState, error) {
	var cs ControlState
	err := m.db.WithContext(ctx).Where("gateway_id = ?", gatewayID).First(&cs).Error
	if err == gorm.ErrRecordNotFound {
		cs = ControlState{
			GatewayID:     gatewayID,
			Epoch:         0,
			ControlHealth: ControlHealthHealthy,
			UpdatedAt:     time.Now(),
		}
		if err := m.db.WithContext(ctx).Create(&cs).Error; err != nil {
			return nil, err
		}
		return &cs, nil
	}
	if err != nil {
		return nil, err
	}
	return &cs, nil
}

func (m *controlStateManagerImpl) IncrementEpoch(ctx context.Context, gatewayID string) (uint64, error) {
	unlock, err := m.lock.Lock(ctx, "control:"+gatewayID)
	if err != nil {
		return 0, err
	}
	defer unlock()

	var newEpoch uint64
	err = m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var cs ControlState
		if err := tx.Where("gateway_id = ?", gatewayID).First(&cs).Error; err != nil {
			return err
		}

		oldUpdatedAt := cs.UpdatedAt
		cs.Epoch++
		newEpoch = cs.Epoch
		cs.UpdatedAt = time.Now()

		result := tx.Where("gateway_id = ? AND updated_at = ?", gatewayID, oldUpdatedAt).Save(&cs)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrOptimisticLockConflict
		}
		return nil
	})
	return newEpoch, err
}

func (m *controlStateManagerImpl) BeginTransaction(ctx context.Context, gatewayID string, txID string) error {
	unlock, err := m.lock.Lock(ctx, "control:"+gatewayID)
	if err != nil {
		return err
	}
	defer unlock()

	return m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var cs ControlState
		if err := tx.Where("gateway_id = ?", gatewayID).First(&cs).Error; err != nil {
			return err
		}

		oldUpdatedAt := cs.UpdatedAt
		cs.ActiveTxID = txID
		cs.UpdatedAt = time.Now()

		result := tx.Where("gateway_id = ? AND updated_at = ?", gatewayID, oldUpdatedAt).Save(&cs)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrOptimisticLockConflict
		}
		return nil
	})
}

func (m *controlStateManagerImpl) CommitTransaction(ctx context.Context, gatewayID string, reason string) error {
	unlock, err := m.lock.Lock(ctx, "control:"+gatewayID)
	if err != nil {
		return err
	}
	defer unlock()

	return m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var cs ControlState
		if err := tx.Where("gateway_id = ?", gatewayID).First(&cs).Error; err != nil {
			return err
		}

		oldUpdatedAt := cs.UpdatedAt
		cs.LastSuccessfulEpoch = cs.Epoch
		cs.RollbackMarker = cs.Epoch
		cs.ActiveTxID = ""
		cs.LastSwitchReason = reason
		cs.UpdatedAt = time.Now()

		result := tx.Where("gateway_id = ? AND updated_at = ?", gatewayID, oldUpdatedAt).Save(&cs)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrOptimisticLockConflict
		}
		return nil
	})
}

func (m *controlStateManagerImpl) RecoverOnStartup(ctx context.Context, gatewayID string) error {
	unlock, err := m.lock.Lock(ctx, "control:"+gatewayID)
	if err != nil {
		return err
	}
	defer unlock()

	return m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var cs ControlState
		if err := tx.Where("gateway_id = ?", gatewayID).First(&cs).Error; err != nil {
			return err
		}

		if cs.ActiveTxID == "" {
			return nil // 无未完成事务，无需恢复
		}

		oldUpdatedAt := cs.UpdatedAt
		cs.ControlHealth = ControlHealthRecovering
		cs.Epoch = cs.RollbackMarker
		cs.ActiveTxID = ""
		cs.UpdatedAt = time.Now()

		result := tx.Where("gateway_id = ? AND updated_at = ?", gatewayID, oldUpdatedAt).Save(&cs)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrOptimisticLockConflict
		}
		return nil
	})
}
