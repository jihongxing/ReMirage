package events

import (
	"fmt"
	"time"
)

// ControlEvent 控制事件基础对象
type ControlEvent struct {
	EventID     string     `json:"event_id"`
	EventType   EventType  `json:"event_type"`
	Source      string     `json:"source"`
	TargetScope EventScope `json:"target_scope"`
	Priority    int        `json:"priority"`
	Epoch       uint64     `json:"epoch"`
	PayloadRef  string     `json:"payload_ref"`
	RequiresAck bool       `json:"requires_ack"`
	CreatedAt   time.Time  `json:"created_at"`
}

// Validate 对 ControlEvent 执行完整性校验
func (e *ControlEvent) Validate() error {
	if e.EventID == "" {
		return &ErrValidation{Field: "event_id", Message: "must not be empty"}
	}
	if e.Source == "" {
		return &ErrValidation{Field: "source", Message: "must not be empty"}
	}

	// 校验 event_type 是否属于已定义枚举
	validType := false
	for _, et := range AllEventTypes {
		if e.EventType == et {
			validType = true
			break
		}
	}
	if !validType {
		return &ErrInvalidEventType{Value: string(e.EventType)}
	}

	// 校验 target_scope
	switch e.TargetScope {
	case EventScopeSession, EventScopeLink, EventScopeGlobal:
		// valid
	default:
		return &ErrInvalidScope{Value: string(e.TargetScope)}
	}

	// 校验 priority 范围
	if e.Priority < 0 || e.Priority > 10 {
		return &ErrValidation{
			Field:   "priority",
			Message: fmt.Sprintf("must be 0-10, got: %d", e.Priority),
		}
	}

	// 校验 requires_ack 一致性
	sem := GetSemantics(e.EventType)
	if sem != nil {
		if sem.RequiresAck && !e.RequiresAck {
			return &ErrValidation{
				Field:   "requires_ack",
				Message: fmt.Sprintf("event_type %s requires ack", e.EventType),
			}
		}
		// 校验 epoch 一致性
		if sem.CarriesEpoch && e.Epoch == 0 {
			return &ErrValidation{
				Field:   "epoch",
				Message: fmt.Sprintf("event_type %s requires epoch", e.EventType),
			}
		}
	}

	return nil
}
