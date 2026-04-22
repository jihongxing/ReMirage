package events

import (
	"context"
	"log"
)

// StrategyUpdateHandler 策略更新事件处理器
type StrategyUpdateHandler struct{}

func (h *StrategyUpdateHandler) Handle(ctx context.Context, event *ControlEvent) error {
	log.Printf("[V2Handler] strategy.update: id=%s payload=%s", event.EventID, event.PayloadRef)
	return nil
}

func (h *StrategyUpdateHandler) EventType() EventType {
	return EventTypeStrategyUpdate
}

// BlacklistUpdateHandler 黑名单更新事件处理器
type BlacklistUpdateHandler struct{}

func (h *BlacklistUpdateHandler) Handle(ctx context.Context, event *ControlEvent) error {
	log.Printf("[V2Handler] blacklist.update: id=%s payload=%s", event.EventID, event.PayloadRef)
	return nil
}

func (h *BlacklistUpdateHandler) EventType() EventType {
	return EventTypeBlacklistUpdate
}

// QuotaUpdateHandler 配额更新事件处理器
type QuotaUpdateHandler struct{}

func (h *QuotaUpdateHandler) Handle(ctx context.Context, event *ControlEvent) error {
	log.Printf("[V2Handler] quota.update: id=%s payload=%s", event.EventID, event.PayloadRef)
	return nil
}

func (h *QuotaUpdateHandler) EventType() EventType {
	return EventTypeQuotaUpdate
}

// ReincarnationHandler 转生触发事件处理器
type ReincarnationHandler struct{}

func (h *ReincarnationHandler) Handle(ctx context.Context, event *ControlEvent) error {
	log.Printf("[V2Handler] reincarnation.trigger: id=%s payload=%s", event.EventID, event.PayloadRef)
	return nil
}

func (h *ReincarnationHandler) EventType() EventType {
	return EventTypeReincarnation
}

// RegisterV2Handlers 注册所有 V2 生产 handler
func RegisterV2Handlers(registry EventRegistry) error {
	handlers := []EventHandler{
		&StrategyUpdateHandler{},
		&BlacklistUpdateHandler{},
		&QuotaUpdateHandler{},
		&ReincarnationHandler{},
	}
	for _, h := range handlers {
		if err := registry.Register(h); err != nil {
			return err
		}
	}
	return nil
}
