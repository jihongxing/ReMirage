package dispatch

import (
	"context"
	"fmt"
	"log"
	"time"

	pb "mirage-proto/gen"

	"mirage-os/gateway-bridge/pkg/topology"
)

// FanoutScope 下推范围
type FanoutScope int

const (
	ScopeSingle FanoutScope = iota // 单 Gateway
	ScopeCell                      // 按 Cell
	ScopeGlobal                    // 全局
)

// PushCommand 下推指令
type PushCommand struct {
	CommandType string      // strategy / quota / blacklist / reincarnation
	Payload     interface{} // *pb.StrategyPush / *pb.QuotaPush / *pb.BlacklistPush / *pb.ReincarnationPush
	Scope       FanoutScope
	TargetID    string // gateway_id（ScopeSingle）或 cell_id（ScopeCell）
}

// FanoutEngine 统一 Fan-out 引擎：支持 Single/Cell/Global 三种 scope
type FanoutEngine struct {
	registry   *topology.Registry
	dispatcher *StrategyDispatcher
	pushLog    *PushLog
}

// NewFanoutEngine 创建 Fan-out 引擎
func NewFanoutEngine(registry *topology.Registry, dispatcher *StrategyDispatcher, pushLog *PushLog) *FanoutEngine {
	return &FanoutEngine{registry: registry, dispatcher: dispatcher, pushLog: pushLog}
}

// Execute 执行下推：解析目标 → 逐个推送 → 失败重试 → 记录日志
func (fe *FanoutEngine) Execute(ctx context.Context, cmd *PushCommand) error {
	targets := fe.resolveTargets(cmd)
	if len(targets) == 0 {
		return fmt.Errorf("no online gateways for scope=%d target=%s", cmd.Scope, cmd.TargetID)
	}

	var lastErr error
	for _, gwID := range targets {
		err := fe.pushToGateway(ctx, gwID, cmd)
		result := "success"
		if err != nil {
			lastErr = err
			// 重试：最多 3 次，指数退避（1s, 4s, 9s）
			retried := false
			for attempt := 1; attempt <= 3; attempt++ {
				time.Sleep(time.Duration(attempt*attempt) * time.Second)
				if retryErr := fe.pushToGateway(ctx, gwID, cmd); retryErr == nil {
					result = fmt.Sprintf("success_after_retry_%d", attempt)
					lastErr = nil
					retried = true
					break
				}
			}
			if !retried {
				result = "failed_after_retries"
				log.Printf("[Fanout] ⚠️ 下推失败 gateway=%s cmd=%s: %v", gwID, cmd.CommandType, lastErr)
			}
		}
		if fe.pushLog != nil {
			fe.pushLog.Record(gwID, cmd.CommandType, result)
		}
	}
	return lastErr
}

// resolveTargets 从 Registry 查询目标 Gateway 列表
func (fe *FanoutEngine) resolveTargets(cmd *PushCommand) []string {
	switch cmd.Scope {
	case ScopeSingle:
		return []string{cmd.TargetID}
	case ScopeCell:
		gws := fe.registry.GetGatewaysByCell(cmd.TargetID)
		ids := make([]string, len(gws))
		for i, gw := range gws {
			ids[i] = gw.GatewayID
		}
		return ids
	case ScopeGlobal:
		gws := fe.registry.GetAllOnline()
		ids := make([]string, len(gws))
		for i, gw := range gws {
			ids[i] = gw.GatewayID
		}
		return ids
	}
	return nil
}

// pushToGateway 向单个 Gateway 推送指令，委托给 StrategyDispatcher 的连接管理
func (fe *FanoutEngine) pushToGateway(ctx context.Context, gwID string, cmd *PushCommand) error {
	fe.dispatcher.mu.RLock()
	gc, ok := fe.dispatcher.connections[gwID]
	fe.dispatcher.mu.RUnlock()

	if !ok {
		return fmt.Errorf("gateway %s not connected", gwID)
	}

	switch cmd.CommandType {
	case "strategy":
		if push, ok := cmd.Payload.(*pb.StrategyPush); ok {
			_, err := gc.client.PushStrategy(ctx, push)
			return err
		}
		return fmt.Errorf("invalid payload type for strategy push")
	case "blacklist":
		if push, ok := cmd.Payload.(*pb.BlacklistPush); ok {
			_, err := gc.client.PushBlacklist(ctx, push)
			return err
		}
		return fmt.Errorf("invalid payload type for blacklist push")
	case "quota":
		if push, ok := cmd.Payload.(*pb.QuotaPush); ok {
			_, err := gc.client.PushQuota(ctx, push)
			return err
		}
		return fmt.Errorf("invalid payload type for quota push")
	case "reincarnation":
		if push, ok := cmd.Payload.(*pb.ReincarnationPush); ok {
			_, err := gc.client.PushReincarnation(ctx, push)
			return err
		}
		return fmt.Errorf("invalid payload type for reincarnation push")
	}
	return fmt.Errorf("unsupported command type: %s", cmd.CommandType)
}
