package events

import (
	"time"

	"github.com/google/uuid"
)

func newEvent(et EventType, source, payloadRef string, epoch uint64) *ControlEvent {
	sem := GetSemantics(et)
	if sem == nil {
		return nil
	}
	ev := &ControlEvent{
		EventID:     uuid.New().String(),
		EventType:   et,
		Source:      source,
		TargetScope: sem.DefaultScope,
		Priority:    sem.DefaultPriority,
		RequiresAck: sem.RequiresAck,
		PayloadRef:  payloadRef,
		CreatedAt:   time.Now(),
	}
	if sem.CarriesEpoch {
		ev.Epoch = epoch
	}
	return ev
}

// NewSessionMigrateRequestEvent 创建会话迁移请求事件
func NewSessionMigrateRequestEvent(source, payloadRef string, epoch uint64) *ControlEvent {
	return newEvent(EventSessionMigrateRequest, source, payloadRef, epoch)
}

// NewSessionMigrateAckEvent 创建会话迁移确认事件
func NewSessionMigrateAckEvent(source, payloadRef string, epoch uint64) *ControlEvent {
	return newEvent(EventSessionMigrateAck, source, payloadRef, epoch)
}

// NewPersonaPrepareEvent 创建 Persona 准备事件
func NewPersonaPrepareEvent(source, payloadRef string, epoch uint64) *ControlEvent {
	return newEvent(EventPersonaPrepare, source, payloadRef, epoch)
}

// NewPersonaFlipEvent 创建 Persona 切换事件
func NewPersonaFlipEvent(source, payloadRef string, epoch uint64) *ControlEvent {
	return newEvent(EventPersonaFlip, source, payloadRef, epoch)
}

// NewSurvivalModeChangeEvent 创建 Survival Mode 变更事件
func NewSurvivalModeChangeEvent(source, payloadRef string, epoch uint64) *ControlEvent {
	return newEvent(EventSurvivalModeChange, source, payloadRef, epoch)
}

// NewRollbackRequestEvent 创建回滚请求事件
func NewRollbackRequestEvent(source, payloadRef string, epoch uint64) *ControlEvent {
	return newEvent(EventRollbackRequest, source, payloadRef, epoch)
}

// NewRollbackDoneEvent 创建回滚完成事件
func NewRollbackDoneEvent(source, payloadRef string, epoch uint64) *ControlEvent {
	return newEvent(EventRollbackDone, source, payloadRef, epoch)
}

// NewBudgetRejectEvent 创建预算拒绝事件
func NewBudgetRejectEvent(source, payloadRef string, epoch uint64) *ControlEvent {
	return newEvent(EventBudgetReject, source, payloadRef, epoch)
}

// NewEvent 通用工厂函数，根据 EventType 创建事件
func NewEvent(eventType EventType, source, payloadRef string, epoch uint64) (*ControlEvent, error) {
	sem := GetSemantics(eventType)
	if sem == nil {
		return nil, &ErrInvalidEventType{Value: string(eventType)}
	}
	return newEvent(eventType, source, payloadRef, epoch), nil
}
