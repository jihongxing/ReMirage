// Package commit - 预留校验接口
package commit

import "context"

// BudgetChecker 预算校验接口（Spec 5-1 实现）
type BudgetChecker interface {
	Check(ctx context.Context, tx *CommitTransaction) error
}

// ServiceClassChecker 服务等级校验接口（Spec 5-1 实现）
type ServiceClassChecker interface {
	Check(ctx context.Context, tx *CommitTransaction) error
}

// DefaultBudgetChecker 默认预算校验（始终通过）
type DefaultBudgetChecker struct{}

func (d *DefaultBudgetChecker) Check(_ context.Context, _ *CommitTransaction) error { return nil }

// DefaultServiceClassChecker 默认服务等级校验（始终通过）
type DefaultServiceClassChecker struct{}

func (d *DefaultServiceClassChecker) Check(_ context.Context, _ *CommitTransaction) error {
	return nil
}
