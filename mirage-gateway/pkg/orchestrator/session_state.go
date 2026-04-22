package orchestrator

import (
	"context"
	"time"

	"gorm.io/gorm"
)

// SessionFilter 会话过滤条件
type SessionFilter struct {
	GatewayID *string
	UserID    *string
	State     *SessionPhase
}

// SessionStateManager 会话状态管理器接口
type SessionStateManager interface {
	Create(ctx context.Context, session *SessionState) error
	Get(ctx context.Context, sessionID string) (*SessionState, error)
	ListByGateway(ctx context.Context, gatewayID string) ([]*SessionState, error)
	ListByUser(ctx context.Context, userID string) ([]*SessionState, error)
	ListByFilter(ctx context.Context, filter SessionFilter) ([]*SessionState, error)
	TransitionState(ctx context.Context, sessionID string, target SessionPhase, reason string) error
	UpdateLink(ctx context.Context, sessionID string, newLinkID string) error
	Delete(ctx context.Context, sessionID string) error
}

type sessionStateManagerImpl struct {
	db          *gorm.DB
	lock        *LockManager
	constraints ConstraintChecker
}

// NewSessionStateManager 创建 SessionStateManager 实例
func NewSessionStateManager(db *gorm.DB, lock *LockManager, constraints ConstraintChecker) SessionStateManager {
	return &sessionStateManagerImpl{db: db, lock: lock, constraints: constraints}
}

func (m *sessionStateManagerImpl) Create(ctx context.Context, session *SessionState) error {
	// 校验链路引用
	if m.constraints != nil && session.CurrentLinkID != "" {
		if err := m.constraints.ValidateLinkRef(ctx, session.CurrentLinkID); err != nil {
			return err
		}
	}
	session.State = SessionPhaseBootstrapping
	session.MigrationPending = false
	now := time.Now()
	session.CreatedAt = now
	session.UpdatedAt = now
	return m.db.WithContext(ctx).Create(session).Error
}

func (m *sessionStateManagerImpl) Get(ctx context.Context, sessionID string) (*SessionState, error) {
	var ss SessionState
	if err := m.db.WithContext(ctx).Where("session_id = ?", sessionID).First(&ss).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, &ErrSessionNotFound{SessionID: sessionID}
		}
		return nil, err
	}
	return &ss, nil
}

func (m *sessionStateManagerImpl) ListByGateway(ctx context.Context, gatewayID string) ([]*SessionState, error) {
	var list []*SessionState
	if err := m.db.WithContext(ctx).Where("gateway_id = ?", gatewayID).Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

func (m *sessionStateManagerImpl) ListByUser(ctx context.Context, userID string) ([]*SessionState, error) {
	var list []*SessionState
	if err := m.db.WithContext(ctx).Where("user_id = ?", userID).Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

func (m *sessionStateManagerImpl) ListByFilter(ctx context.Context, filter SessionFilter) ([]*SessionState, error) {
	q := m.db.WithContext(ctx).Model(&SessionState{})
	if filter.GatewayID != nil {
		q = q.Where("gateway_id = ?", *filter.GatewayID)
	}
	if filter.UserID != nil {
		q = q.Where("user_id = ?", *filter.UserID)
	}
	if filter.State != nil {
		q = q.Where("state = ?", *filter.State)
	}
	var list []*SessionState
	if err := q.Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

func (m *sessionStateManagerImpl) TransitionState(ctx context.Context, sessionID string, target SessionPhase, reason string) error {
	unlock, err := m.lock.Lock(ctx, "session:"+sessionID)
	if err != nil {
		return err
	}
	defer unlock()

	return m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var ss SessionState
		if err := tx.Where("session_id = ?", sessionID).First(&ss).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return &ErrSessionNotFound{SessionID: sessionID}
			}
			return err
		}

		if !IsValidSessionTransition(ss.State, target) {
			return &ErrInvalidTransition{From: string(ss.State), To: string(target)}
		}

		oldUpdatedAt := ss.UpdatedAt

		// 副作用：迁移标记
		if target == SessionPhaseMigrating {
			ss.MigrationPending = true
		}
		if ss.State == SessionPhaseMigrating && target != SessionPhaseMigrating {
			ss.MigrationPending = false
		}

		ss.State = target
		ss.UpdatedAt = time.Now()

		result := tx.Where("session_id = ? AND updated_at = ?", sessionID, oldUpdatedAt).Save(&ss)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrOptimisticLockConflict
		}
		return nil
	})
}

func (m *sessionStateManagerImpl) UpdateLink(ctx context.Context, sessionID string, newLinkID string) error {
	unlock, err := m.lock.Lock(ctx, "session:"+sessionID)
	if err != nil {
		return err
	}
	defer unlock()

	return m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var ss SessionState
		if err := tx.Where("session_id = ?", sessionID).First(&ss).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return &ErrSessionNotFound{SessionID: sessionID}
			}
			return err
		}

		oldUpdatedAt := ss.UpdatedAt
		ss.CurrentLinkID = newLinkID
		ss.UpdatedAt = time.Now()

		result := tx.Where("session_id = ? AND updated_at = ?", sessionID, oldUpdatedAt).Save(&ss)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrOptimisticLockConflict
		}
		return nil
	})
}

func (m *sessionStateManagerImpl) Delete(ctx context.Context, sessionID string) error {
	result := m.db.WithContext(ctx).Where("session_id = ?", sessionID).Delete(&SessionState{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return &ErrSessionNotFound{SessionID: sessionID}
	}
	return nil
}
