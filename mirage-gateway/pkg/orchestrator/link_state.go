package orchestrator

import (
	"context"
	"time"

	"gorm.io/gorm"
)

// LinkStateManager 链路状态管理器接口
type LinkStateManager interface {
	Create(ctx context.Context, link *LinkState) error
	Get(ctx context.Context, linkID string) (*LinkState, error)
	ListByGateway(ctx context.Context, gatewayID string) ([]*LinkState, error)
	TransitionPhase(ctx context.Context, linkID string, target LinkPhase, reason string) error
	UpdateHealth(ctx context.Context, linkID string, score float64, rtt int64, loss float64, jitter int64) error
	Delete(ctx context.Context, linkID string) error
}

// linkStateManagerImpl LinkStateManager 的 GORM 实现
type linkStateManagerImpl struct {
	db   *gorm.DB
	lock *LockManager
}

// NewLinkStateManager 创建 LinkStateManager 实例
func NewLinkStateManager(db *gorm.DB, lock *LockManager) LinkStateManager {
	return &linkStateManagerImpl{db: db, lock: lock}
}

func (m *linkStateManagerImpl) Create(ctx context.Context, link *LinkState) error {
	link.Phase = LinkPhaseProbing
	link.Available = false
	link.Degraded = false
	link.HealthScore = 0
	now := time.Now()
	link.CreatedAt = now
	link.UpdatedAt = now
	return m.db.WithContext(ctx).Create(link).Error
}

func (m *linkStateManagerImpl) Get(ctx context.Context, linkID string) (*LinkState, error) {
	var ls LinkState
	if err := m.db.WithContext(ctx).Where("link_id = ?", linkID).First(&ls).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, &ErrLinkNotFound{LinkID: linkID}
		}
		return nil, err
	}
	return &ls, nil
}

func (m *linkStateManagerImpl) ListByGateway(ctx context.Context, gatewayID string) ([]*LinkState, error) {
	var list []*LinkState
	if err := m.db.WithContext(ctx).Where("gateway_id = ?", gatewayID).Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

func (m *linkStateManagerImpl) TransitionPhase(ctx context.Context, linkID string, target LinkPhase, reason string) error {
	unlock, err := m.lock.Lock(ctx, "link:"+linkID)
	if err != nil {
		return err
	}
	defer unlock()

	return m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var ls LinkState
		if err := tx.Where("link_id = ?", linkID).First(&ls).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return &ErrLinkNotFound{LinkID: linkID}
			}
			return err
		}

		if !IsValidLinkTransition(ls.Phase, target) {
			return &ErrInvalidTransition{From: string(ls.Phase), To: string(target)}
		}

		oldUpdatedAt := ls.UpdatedAt
		ls.Phase = target
		ls.LastSwitchReason = reason
		ls.UpdatedAt = time.Now()

		// 副作用
		switch target {
		case LinkPhaseUnavailable:
			ls.Available = false
			ls.HealthScore = 0
		case LinkPhaseActive:
			ls.Available = true
			ls.Degraded = false
		case LinkPhaseDegrading:
			ls.Degraded = true
		}

		result := tx.Where("link_id = ? AND updated_at = ?", linkID, oldUpdatedAt).Save(&ls)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrOptimisticLockConflict
		}
		return nil
	})
}

func (m *linkStateManagerImpl) UpdateHealth(ctx context.Context, linkID string, score float64, rtt int64, loss float64, jitter int64) error {
	unlock, err := m.lock.Lock(ctx, "link:"+linkID)
	if err != nil {
		return err
	}
	defer unlock()

	return m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var ls LinkState
		if err := tx.Where("link_id = ?", linkID).First(&ls).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return &ErrLinkNotFound{LinkID: linkID}
			}
			return err
		}

		oldUpdatedAt := ls.UpdatedAt
		now := time.Now()
		ls.HealthScore = score
		ls.RttMs = rtt
		ls.LossRate = loss
		ls.JitterMs = jitter
		ls.LastProbeAt = &now
		ls.UpdatedAt = now

		result := tx.Where("link_id = ? AND updated_at = ?", linkID, oldUpdatedAt).Save(&ls)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrOptimisticLockConflict
		}
		return nil
	})
}

func (m *linkStateManagerImpl) Delete(ctx context.Context, linkID string) error {
	result := m.db.WithContext(ctx).Where("link_id = ?", linkID).Delete(&LinkState{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return &ErrLinkNotFound{LinkID: linkID}
	}
	return nil
}
