package events

import "sync"

// EventRegistry 事件注册表
type EventRegistry interface {
	// Register 注册事件处理器，同一 EventType 重复注册返回错误
	Register(handler EventHandler) error
	// GetHandler 获取指定 EventType 的处理器
	GetHandler(et EventType) (EventHandler, error)
	// ListRegistered 返回所有已注册的 EventType 列表
	ListRegistered() []EventType
	// IsRegistered 检查指定 EventType 是否已注册
	IsRegistered(et EventType) bool
}

// registryImpl 基于 sync.RWMutex 的 EventRegistry 实现
type registryImpl struct {
	mu       sync.RWMutex
	handlers map[EventType]EventHandler
}

// NewEventRegistry 创建 EventRegistry 实例
func NewEventRegistry() EventRegistry {
	return &registryImpl{
		handlers: make(map[EventType]EventHandler),
	}
}

func (r *registryImpl) Register(handler EventHandler) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	et := handler.EventType()
	if _, exists := r.handlers[et]; exists {
		return &ErrDuplicateRegistration{EventType: et}
	}
	r.handlers[et] = handler
	return nil
}

func (r *registryImpl) GetHandler(et EventType) (EventHandler, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	h, ok := r.handlers[et]
	if !ok {
		return nil, &ErrHandlerNotRegistered{EventType: et}
	}
	return h, nil
}

func (r *registryImpl) ListRegistered() []EventType {
	r.mu.RLock()
	defer r.mu.RUnlock()
	types := make([]EventType, 0, len(r.handlers))
	for et := range r.handlers {
		types = append(types, et)
	}
	return types
}

func (r *registryImpl) IsRegistered(et EventType) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.handlers[et]
	return ok
}
