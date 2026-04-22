package api

import (
	"context"
	"fmt"
	"log"
	"time"

	"mirage-gateway/pkg/orchestrator/events"
	pb "mirage-proto/gen"
)

// V2CommandAdapter 将 legacy GatewayDownlink 命令转换为 V2 ControlEvent，
// 投递给 EventDispatcher 进入 V2 编排链路。
type V2CommandAdapter struct {
	dispatcher events.EventDispatcher
}

// NewV2CommandAdapter 创建 V2 命令适配器
func NewV2CommandAdapter(dispatcher events.EventDispatcher) *V2CommandAdapter {
	return &V2CommandAdapter{dispatcher: dispatcher}
}

// AdaptPushStrategy 将 StrategyPush 转换为 V2 ControlEvent
func (a *V2CommandAdapter) AdaptPushStrategy(ctx context.Context, req *pb.StrategyPush) error {
	event := &events.ControlEvent{
		EventID:     fmt.Sprintf("strategy-%d", time.Now().UnixNano()),
		EventType:   events.EventTypeStrategyUpdate,
		Source:      "legacy-downlink",
		TargetScope: events.EventScopeGlobal,
		Priority:    5,
		CreatedAt:   time.Now(),
		PayloadRef:  fmt.Sprintf("level=%d,jitter=%d", req.DefenseLevel, req.JitterMeanUs),
	}
	if err := a.dispatcher.Dispatch(ctx, event); err != nil {
		log.Printf("[V2Adapter] StrategyPush 分发失败: %v", err)
		return err
	}
	log.Printf("[V2Adapter] StrategyPush 已投递 V2 编排链路: level=%d", req.DefenseLevel)
	return nil
}

// AdaptPushQuota 将 QuotaPush 转换为 V2 ControlEvent
func (a *V2CommandAdapter) AdaptPushQuota(ctx context.Context, req *pb.QuotaPush) error {
	event := &events.ControlEvent{
		EventID:     fmt.Sprintf("quota-%d", time.Now().UnixNano()),
		EventType:   events.EventTypeQuotaUpdate,
		Source:      "legacy-downlink",
		TargetScope: events.EventScopeSession,
		Priority:    5,
		CreatedAt:   time.Now(),
		PayloadRef:  fmt.Sprintf("user=%s,remaining=%d", req.UserId, req.RemainingBytes),
	}
	if err := a.dispatcher.Dispatch(ctx, event); err != nil {
		log.Printf("[V2Adapter] QuotaPush 分发失败: %v", err)
		return err
	}
	log.Printf("[V2Adapter] QuotaPush 已投递 V2 编排链路: user=%s", req.UserId)
	return nil
}

// AdaptPushBlacklist 将 BlacklistPush 转换为 V2 ControlEvent
func (a *V2CommandAdapter) AdaptPushBlacklist(ctx context.Context, req *pb.BlacklistPush) error {
	event := &events.ControlEvent{
		EventID:     fmt.Sprintf("blacklist-%d", time.Now().UnixNano()),
		EventType:   events.EventTypeBlacklistUpdate,
		Source:      "legacy-downlink",
		TargetScope: events.EventScopeGlobal,
		Priority:    7,
		CreatedAt:   time.Now(),
		PayloadRef:  fmt.Sprintf("entries=%d", len(req.Entries)),
	}
	if err := a.dispatcher.Dispatch(ctx, event); err != nil {
		log.Printf("[V2Adapter] BlacklistPush 分发失败: %v", err)
		return err
	}
	log.Printf("[V2Adapter] BlacklistPush 已投递 V2 编排链路: %d entries", len(req.Entries))
	return nil
}

// AdaptPushReincarnation 将 ReincarnationPush 转换为 V2 ControlEvent
func (a *V2CommandAdapter) AdaptPushReincarnation(ctx context.Context, req *pb.ReincarnationPush) error {
	event := &events.ControlEvent{
		EventID:     fmt.Sprintf("reincarnation-%d", time.Now().UnixNano()),
		EventType:   events.EventTypeReincarnation,
		Source:      "legacy-downlink",
		TargetScope: events.EventScopeGlobal,
		Priority:    9,
		RequiresAck: true,
		CreatedAt:   time.Now(),
		PayloadRef:  fmt.Sprintf("domain=%s,ip=%s", req.NewDomain, req.NewIp),
	}
	if err := a.dispatcher.Dispatch(ctx, event); err != nil {
		log.Printf("[V2Adapter] ReincarnationPush 分发失败: %v", err)
		return err
	}
	log.Printf("[V2Adapter] ReincarnationPush 已投递 V2 编排链路: domain=%s", req.NewDomain)
	return nil
}
