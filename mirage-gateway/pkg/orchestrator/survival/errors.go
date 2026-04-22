package survival

import (
	"fmt"
	"time"

	orchestrator "mirage-gateway/pkg/orchestrator"
)

// ErrInvalidTransition 非法模式迁移
type ErrInvalidTransition struct {
	From orchestrator.SurvivalMode
	To   orchestrator.SurvivalMode
}

func (e *ErrInvalidTransition) Error() string {
	return fmt.Sprintf("invalid transition from %s to %s", e.From, e.To)
}

// ErrConstraintViolation 约束违反
type ErrConstraintViolation struct {
	ConstraintType string        // "min_dwell_time" / "cooldown" / "hysteresis"
	Remaining      time.Duration // 剩余等待时间
}

func (e *ErrConstraintViolation) Error() string {
	return fmt.Sprintf("constraint violation: %s, remaining %s", e.ConstraintType, e.Remaining)
}

// ErrAdmissionDenied 准入拒绝
type ErrAdmissionDenied struct {
	Policy       SessionAdmissionPolicy
	ServiceClass orchestrator.ServiceClass
}

func (e *ErrAdmissionDenied) Error() string {
	return fmt.Sprintf("admission denied: policy=%s, service_class=%s", e.Policy, e.ServiceClass)
}
