package events

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestRegisterV2Handlers_AllRegistered(t *testing.T) {
	registry := NewEventRegistry()
	if err := RegisterV2Handlers(registry); err != nil {
		t.Fatalf("注册失败: %v", err)
	}

	expected := []EventType{
		EventTypeStrategyUpdate,
		EventTypeBlacklistUpdate,
		EventTypeQuotaUpdate,
		EventTypeReincarnation,
	}

	for _, et := range expected {
		if !registry.IsRegistered(et) {
			t.Fatalf("EventType %s 未注册", et)
		}
	}
}

func TestV2Dispatch_StrategyUpdate_Success(t *testing.T) {
	registry := NewEventRegistry()
	if err := RegisterV2Handlers(registry); err != nil {
		t.Fatalf("注册失败: %v", err)
	}

	dedup := NewDeduplicationStore()
	dispatcher := NewEventDispatcher(registry, dedup, nil)

	event := &ControlEvent{
		EventID:     fmt.Sprintf("test-strategy-%d", time.Now().UnixNano()),
		EventType:   EventTypeStrategyUpdate,
		Source:      "test",
		TargetScope: EventScopeGlobal,
		Priority:    5,
		CreatedAt:   time.Now(),
		PayloadRef:  "level=3",
	}

	err := dispatcher.Dispatch(context.Background(), event)
	if err != nil {
		t.Fatalf("strategy.update 分发不应失败: %v", err)
	}
}

func TestV2Dispatch_AllEventTypes_NoHandlerError(t *testing.T) {
	registry := NewEventRegistry()
	if err := RegisterV2Handlers(registry); err != nil {
		t.Fatalf("注册失败: %v", err)
	}

	dedup := NewDeduplicationStore()
	dispatcher := NewEventDispatcher(registry, dedup, nil)

	testCases := []struct {
		eventType   EventType
		scope       EventScope
		requiresAck bool
	}{
		{EventTypeStrategyUpdate, EventScopeGlobal, false},
		{EventTypeBlacklistUpdate, EventScopeGlobal, false},
		{EventTypeQuotaUpdate, EventScopeSession, false},
		{EventTypeReincarnation, EventScopeGlobal, true},
	}

	for _, tc := range testCases {
		event := &ControlEvent{
			EventID:     fmt.Sprintf("test-%s-%d", tc.eventType, time.Now().UnixNano()),
			EventType:   tc.eventType,
			Source:      "test",
			TargetScope: tc.scope,
			Priority:    5,
			RequiresAck: tc.requiresAck,
			CreatedAt:   time.Now(),
			PayloadRef:  "test-payload",
		}

		err := dispatcher.Dispatch(context.Background(), event)
		if err != nil {
			t.Fatalf("EventType %s 分发失败: %v", tc.eventType, err)
		}
	}
}
