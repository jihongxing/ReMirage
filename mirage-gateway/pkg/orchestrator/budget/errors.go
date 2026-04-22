package budget

import "fmt"

// ErrBudgetDenied 预算拒绝错误
type ErrBudgetDenied struct {
	Verdict BudgetVerdict
	Reason  string
}

func (e *ErrBudgetDenied) Error() string {
	return fmt.Sprintf("budget denied: verdict=%s, reason=%s", e.Verdict, e.Reason)
}

// ErrServiceClassDenied 服务等级拒绝错误
type ErrServiceClassDenied struct {
	ServiceClass string
	DeniedMode   string
}

func (e *ErrServiceClassDenied) Error() string {
	return fmt.Sprintf("service class denied: service_class=%s, denied_mode=%s", e.ServiceClass, e.DeniedMode)
}

// ErrInvalidBudgetProfile 无效预算配置错误
type ErrInvalidBudgetProfile struct {
	Field   string
	Message string
}

func (e *ErrInvalidBudgetProfile) Error() string {
	return fmt.Sprintf("invalid budget profile: field=%s, %s", e.Field, e.Message)
}
