package orchestrator

import (
	"context"

	"gorm.io/gorm"
)

// ConstraintChecker 关系约束执行器接口
type ConstraintChecker interface {
	ValidateLinkRef(ctx context.Context, linkID string) error
	OnLinkUnavailable(ctx context.Context, linkID string) error
	ValidateControlStateSingleton(ctx context.Context, gatewayID string) error
}

type constraintCheckerImpl struct {
	db *gorm.DB
}

// NewConstraintChecker 创建 ConstraintChecker 实例
func NewConstraintChecker(db *gorm.DB) ConstraintChecker {
	return &constraintCheckerImpl{db: db}
}

func (c *constraintCheckerImpl) ValidateLinkRef(ctx context.Context, linkID string) error {
	var ls LinkState
	if err := c.db.WithContext(ctx).Where("link_id = ?", linkID).First(&ls).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return &ErrLinkNotFound{LinkID: linkID}
		}
		return err
	}
	if !ls.Available {
		return &ErrLinkUnavailable{LinkID: linkID}
	}
	return nil
}

func (c *constraintCheckerImpl) OnLinkUnavailable(ctx context.Context, linkID string) error {
	var sessions []*SessionState
	if err := c.db.WithContext(ctx).Where("current_link_id = ?", linkID).Find(&sessions).Error; err != nil {
		return err
	}

	for _, ss := range sessions {
		if IsValidSessionTransition(ss.State, SessionPhaseDegraded) {
			ss.State = SessionPhaseDegraded
			if err := c.db.WithContext(ctx).Save(ss).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *constraintCheckerImpl) ValidateControlStateSingleton(ctx context.Context, gatewayID string) error {
	var count int64
	if err := c.db.WithContext(ctx).Model(&ControlState{}).Where("gateway_id = ?", gatewayID).Count(&count).Error; err != nil {
		return err
	}
	if count > 1 {
		return &ErrInvalidTransition{From: "multiple", To: "singleton"}
	}
	return nil
}
