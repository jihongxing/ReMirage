// Package persona - V2 Persona Engine 错误类型定义
package persona

import "fmt"

// ErrMissingProfile 必填 profile_id 为空
type ErrMissingProfile struct {
	FieldName string
}

func (e *ErrMissingProfile) Error() string {
	return fmt.Sprintf("missing required profile: %s", e.FieldName)
}

// ErrChecksumMismatch checksum 不匹配
type ErrChecksumMismatch struct {
	Expected string
	Actual   string
}

func (e *ErrChecksumMismatch) Error() string {
	return fmt.Sprintf("checksum mismatch: expected %s, actual %s", e.Expected, e.Actual)
}

// ErrVersionConflict 版本冲突
type ErrVersionConflict struct {
	PersonaID   string
	ExistingMax uint64
	Attempted   uint64
}

func (e *ErrVersionConflict) Error() string {
	return fmt.Sprintf("version conflict for persona %s: existing max %d, attempted %d", e.PersonaID, e.ExistingMax, e.Attempted)
}

// ErrImmutableField 不可变字段修改
type ErrImmutableField struct {
	FieldName string
}

func (e *ErrImmutableField) Error() string {
	return fmt.Sprintf("cannot modify immutable field: %s", e.FieldName)
}

// ErrInvalidLifecycleTransition 非法生命周期转换
type ErrInvalidLifecycleTransition struct {
	From PersonaLifecycle
	To   PersonaLifecycle
}

func (e *ErrInvalidLifecycleTransition) Error() string {
	return fmt.Sprintf("invalid lifecycle transition from %s to %s", e.From, e.To)
}

// ErrShadowVerifyFailed Shadow Slot 回读校验失败
type ErrShadowVerifyFailed struct {
	MapName string
	Field   string
}

func (e *ErrShadowVerifyFailed) Error() string {
	return fmt.Sprintf("shadow verify failed: map=%s, field=%s", e.MapName, e.Field)
}

// ErrMapWriteFailed eBPF Map 写入失败
type ErrMapWriteFailed struct {
	MapName string
}

func (e *ErrMapWriteFailed) Error() string {
	return fmt.Sprintf("map write failed: %s", e.MapName)
}

// ErrFlipFailed Atomic Flip 失败
var ErrFlipFailed = fmt.Errorf("atomic flip failed")

// ErrSwitchInProgress 并发切换冲突
var ErrSwitchInProgress = fmt.Errorf("persona switch already in progress")

// ErrNoCoolingTarget 无 Cooling 可回滚
var ErrNoCoolingTarget = fmt.Errorf("no cooling persona available for rollback")

// ErrNoMatchingPersona 无匹配 Persona
type ErrNoMatchingPersona struct {
	Constraints string
}

func (e *ErrNoMatchingPersona) Error() string {
	return fmt.Sprintf("no matching persona for constraints: %s", e.Constraints)
}
