package budget

import "context"

// BudgetProfileStore 预算配置存储接口
type BudgetProfileStore interface {
	// Get 获取指定 session 的 BudgetProfile，不存在返回 DefaultBudgetProfile
	Get(ctx context.Context, sessionID string) (*BudgetProfile, error)
	// Save 创建或更新 BudgetProfile
	Save(ctx context.Context, profile *BudgetProfile) error
	// LoadAll 从数据库加载所有 BudgetProfile
	LoadAll(ctx context.Context) ([]*BudgetProfile, error)
}
