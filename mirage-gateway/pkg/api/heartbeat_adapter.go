package api

import (
	"context"
	"log"
	pb "mirage-proto/gen"
	"runtime"
	"time"
)

// HeartbeatAdapter 适配 GRPCClient 为 strategy.HeartbeatSender 接口
type HeartbeatAdapter struct {
	client     *GRPCClient
	gatewayID  string
	securityFn func() (bool, uint32) // 安全状态回调：(isUnderAttack, threatLevel)
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
	a.client.StartHeartbeat(ctx, func() *pb.HeartbeatRequest {
		var memStats runtime.MemStats
		runtime.ReadMemStats(&memStats)

		req := &pb.HeartbeatRequest{
			GatewayId:     a.gatewayID,
			Timestamp:     time.Now().Unix(),
			Status:        pb.GatewayStatus_ONLINE,
			EbpfLoaded:    true,
			ThreatLevel:   0,
			MemoryUsageMb: int32(memStats.Alloc / 1024 / 1024),
		}

		// 如果有安全状态回调，携带受攻击标记
		if a.securityFn != nil {
			isUnderAttack, threatLevel := a.securityFn()
			req.ThreatLevel = int32(threatLevel)
			if isUnderAttack {
				req.Status = pb.GatewayStatus_DEGRADED
			}
		}

		return req
	})

	log.Printf("[HeartbeatAdapter] 心跳循环已启动 (gateway=%s)", a.gatewayID)
}

// SetSecurityCallback 设置安全状态回调（返回 isUnderAttack, threatLevel）
func (a *HeartbeatAdapter) SetSecurityCallback(fn func() (bool, uint32)) {
	a.securityFn = fn
}

// IsConnected 是否已连接到 OS
func (a *HeartbeatAdapter) IsConnected() bool {
	return a.client.IsConnected()
}
