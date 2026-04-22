package budget

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

// GormBudgetProfileStore GORM 实现的 BudgetProfileStore
type GormBudgetProfileStore struct {
	db *gorm.DB
}

// NewGormBudgetProfileStore 创建 GormBudgetProfileStore 实例
func NewGormBudgetProfileStore(db *gorm.DB) *GormBudgetProfileStore {
	return &GormBudgetProfileStore{db: db}
}

// Get 获取指定 session 的 BudgetProfile，不存在返回 DefaultBudgetProfile
func (s *GormBudgetProfileStore) Get(ctx context.Context, sessionID string) (*BudgetProfile, error) {
	var profile BudgetProfile
	err := s.db.WithContext(ctx).Where("session_id = ?", sessionID).First(&profile).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return DefaultBudgetProfile(), nil
		}
		return nil, err
	}
	return &profile, nil
}

// Save 创建或更新 BudgetProfile（upsert）
func (s *GormBudgetProfileStore) Save(ctx context.Context, profile *BudgetProfile) error {
	return s.db.WithContext(ctx).Save(profile).Error
}

// LoadAll 从数据库加载所有 BudgetProfile
func (s *GormBudgetProfileStore) LoadAll(ctx context.Context) ([]*BudgetProfile, error) {
	var profiles []*BudgetProfile
	err := s.db.WithContext(ctx).Find(&profiles).Error
	if err != nil {
		return nil, err
	}
	return profiles, nil
}
