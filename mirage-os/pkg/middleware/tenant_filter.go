// Package middleware - 多租户数据隔离中间件
package middleware

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

// TenantFilter 多租户过滤器
type TenantFilter struct {
	UserIDKey string
}

// NewTenantFilter 创建租户过滤器
func NewTenantFilter() *TenantFilter {
	return &TenantFilter{
		UserIDKey: "tenant_user_id",
	}
}

// Apply 应用租户过滤（GORM 插件）
func (tf *TenantFilter) Apply(db *gorm.DB) *gorm.DB {
	// 从上下文获取当前用户 ID
	userID, ok := db.Statement.Context.Value(tf.UserIDKey).(string)
	if !ok || userID == "" {
		return db
	}

	// 自动注入 WHERE user_id = ? 条件
	return db.Where("user_id = ?", userID)
}

// WithUserID 设置上下文中的用户 ID
func WithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, "tenant_user_id", userID)
}

// GetUserID 从上下文获取用户 ID
func GetUserID(ctx context.Context) (string, error) {
	userID, ok := ctx.Value("tenant_user_id").(string)
	if !ok || userID == "" {
		return "", fmt.Errorf("未找到用户 ID")
	}
	return userID, nil
}

// RegisterTenantFilter 注册租户过滤插件
func RegisterTenantFilter(db *gorm.DB) {
	filter := NewTenantFilter()
	
	// 注册查询回调
	db.Callback().Query().Before("gorm:query").Register("tenant_filter:query", func(db *gorm.DB) {
		filter.Apply(db)
	})
	
	// 注册更新回调
	db.Callback().Update().Before("gorm:update").Register("tenant_filter:update", func(db *gorm.DB) {
		filter.Apply(db)
	})
	
	// 注册删除回调
	db.Callback().Delete().Before("gorm:delete").Register("tenant_filter:delete", func(db *gorm.DB) {
		filter.Apply(db)
	})
}
