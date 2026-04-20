package api

import (
	"context"
	"log"
	"runtime"
	"time"

	"mirage-gateway/pkg/api/proto"
)

// HeartbeatAdapter 适配 GRPCClient 为 strategy.HeartbeatSender 接口
type HeartbeatAdapter struct {
	client    *GRPCClient
	gatewayID string
}

// NewHeartbeatAdapter 创建适配器
func NewHeartbeatAdapter(client *GRPCClient, gatewayID string) *HeartbeatAdapter {
	return &HeartbeatAdapter{
		client:    client,
		gatewayID: gatewayID,
	}
}

// StartHeartbeatLoop 启动心跳循环，成功时调用 onSuccess 喂看门狗
func (a *HeartbeatAdapter) StartHeartbeatLoop(ctx context.Context, onSuccess func()) {
	// 注册心跳成功回调
	a.client.SetHeartbeatCallback(onSuccess)

	// 启动 GRPCClient 的心跳循环
	a.client.StartHeartbeat(ctx, func() *proto.HeartbeatRequest {
		var memStats runtime.MemStats
		runtime.ReadMemStats(&memStats)

		return &proto.HeartbeatRequest{
			GatewayId:     a.gatewayID,
			Timestamp:     time.Now().Unix(),
			Status:        proto.GatewayStatus_ONLINE,
			EbpfLoaded:    true,
			ThreatLevel:   0,
			MemoryUsageMb: int32(memStats.Alloc / 1024 / 1024),
		}
	})

	log.Printf("[HeartbeatAdapter] 心跳循环已启动 (gateway=%s)", a.gatewayID)
}

// IsConnected 是否已连接到 OS
func (a *HeartbeatAdapter) IsConnected() bool {
	return a.client.IsConnected()
}
