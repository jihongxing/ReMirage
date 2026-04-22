package tests

import (
	"context"
	"testing"

	"mirage-gateway/pkg/api"
	"mirage-gateway/pkg/orchestrator/events"
	pb "mirage-proto/gen"
)

// TestV2AdapterDispatchStrategy 验证 PushStrategy 命令通过 V2 adapter 投递到 EventDispatcher
func TestV2AdapterDispatchStrategy(t *testing.T) {
	registry := events.NewEventRegistry()
	dedup := events.NewDeduplicationStore()

	// 注册一个 handler 来捕获事件
	captured := make(chan *events.ControlEvent, 1)
	registry.Register(&captureHandler{
		eventType: events.EventTypeStrategyUpdate,
		ch:        captured,
	})

	dispatcher := events.NewEventDispatcher(registry, dedup, nil)
	adapter := api.NewV2CommandAdapter(dispatcher)

	ctx := context.Background()
	err := adapter.AdaptPushStrategy(ctx, &pb.StrategyPush{
		DefenseLevel: 2,
		JitterMeanUs: 50000,
	})
	if err != nil {
		t.Fatalf("AdaptPushStrategy 失败: %v", err)
	}

	select {
	case evt := <-captured:
		if evt.EventType != events.EventTypeStrategyUpdate {
			t.Errorf("期望 EventType=%s, 实际=%s", events.EventTypeStrategyUpdate, evt.EventType)
		}
		if evt.Source != "legacy-downlink" {
			t.Errorf("期望 Source=legacy-downlink, 实际=%s", evt.Source)
		}
	default:
		t.Error("未收到分发事件")
	}
}

// TestV2AdapterDispatchReincarnation 验证 PushReincarnation 命令闭环
func TestV2AdapterDispatchReincarnation(t *testing.T) {
	registry := events.NewEventRegistry()
	dedup := events.NewDeduplicationStore()

	captured := make(chan *events.ControlEvent, 1)
	registry.Register(&captureHandler{
		eventType: events.EventTypeReincarnation,
		ch:        captured,
	})

	dispatcher := events.NewEventDispatcher(registry, dedup, nil)
	adapter := api.NewV2CommandAdapter(dispatcher)

	ctx := context.Background()
	err := adapter.AdaptPushReincarnation(ctx, &pb.ReincarnationPush{
		NewDomain:       "new.example.com",
		NewIp:           "1.2.3.4",
		DeadlineSeconds: 60,
		Reason:          "test",
	})
	if err != nil {
		t.Fatalf("AdaptPushReincarnation 失败: %v", err)
	}

	select {
	case evt := <-captured:
		if evt.EventType != events.EventTypeReincarnation {
			t.Errorf("期望 EventType=%s, 实际=%s", events.EventTypeReincarnation, evt.EventType)
		}
	default:
		t.Error("未收到分发事件")
	}
}

// captureHandler 捕获事件的测试 handler
type captureHandler struct {
	eventType events.EventType
	ch        chan *events.ControlEvent
}

func (h *captureHandler) EventType() events.EventType { return h.eventType }
func (h *captureHandler) Handle(_ context.Context, event *events.ControlEvent) error {
	h.ch <- event
	return nil
}
