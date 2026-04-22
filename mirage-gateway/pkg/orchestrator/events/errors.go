package events

import "fmt"

// ErrValidation 校验错误
type ErrValidation struct {
	Field   string
	Message string
}

func (e *ErrValidation) Error() string {
	return fmt.Sprintf("validation error: field %q: %s", e.Field, e.Message)
}

// ErrInvalidEventType 非法事件类型
type ErrInvalidEventType struct {
	Value string
}

func (e *ErrInvalidEventType) Error() string {
	return fmt.Sprintf("invalid event type: %q", e.Value)
}

// ErrInvalidScope 非法作用域
type ErrInvalidScope struct {
	Value string
}

func (e *ErrInvalidScope) Error() string {
	return fmt.Sprintf("invalid event scope: %q", e.Value)
}

// ErrHandlerNotRegistered 处理器未注册
type ErrHandlerNotRegistered struct {
	EventType EventType
}

func (e *ErrHandlerNotRegistered) Error() string {
	return fmt.Sprintf("handler not registered for event type: %q", e.EventType)
}

// ErrDuplicateRegistration 重复注册
type ErrDuplicateRegistration struct {
	EventType EventType
}

func (e *ErrDuplicateRegistration) Error() string {
	return fmt.Sprintf("duplicate registration for event type: %q", e.EventType)
}

// ErrEpochStale epoch 过期
type ErrEpochStale struct {
	EventEpoch   uint64
	CurrentEpoch uint64
}

func (e *ErrEpochStale) Error() string {
	return fmt.Sprintf("epoch stale: event epoch %d < current epoch %d", e.EventEpoch, e.CurrentEpoch)
}

// ErrDispatchFailed 分发失败（包装 Handler 错误）
type ErrDispatchFailed struct {
	EventID   string
	EventType EventType
	Cause     error
}

func (e *ErrDispatchFailed) Error() string {
	return fmt.Sprintf("dispatch failed for event %q (type %q): %v", e.EventID, e.EventType, e.Cause)
}

func (e *ErrDispatchFailed) Unwrap() error {
	return e.Cause
}
