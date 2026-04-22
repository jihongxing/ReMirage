package events

import "context"

// EventHandler 事件处理器接口
type EventHandler interface {
	// Handle 处理控制事件
	Handle(ctx context.Context, event *ControlEvent) error
	// EventType 返回该处理器负责的事件类型
	EventType() EventType
}
