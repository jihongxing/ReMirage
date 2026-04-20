// Package api - gRPC 通信模块
package api

import (
	"context"
	"crypto/tls"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"mirage-gateway/pkg/api/proto"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

// GRPCClient 上行 gRPC 客户端
type GRPCClient struct {
	conn              *grpc.ClientConn
	uplinkClient      proto.GatewayUplinkClient
	gatewayID         string
	tlsConfig         *tls.Config
	endpoint          string
	connected         atomic.Bool
	degradedSince     time.Time
	eventBuffer       []*proto.ThreatEvent
	mu                sync.Mutex
	maxBuffer         int
	heartbeatCallback func() // 心跳成功回调（喂看门狗）
}

// NewGRPCClient 创建客户端
func NewGRPCClient(endpoint, gatewayID string, tlsConfig *tls.Config) *GRPCClient {
	return &GRPCClient{
		endpoint:    endpoint,
		gatewayID:   gatewayID,
		tlsConfig:   tlsConfig,
		eventBuffer: make([]*proto.ThreatEvent, 0, 1000),
		maxBuffer:   1000,
	}
}

// Connect 建立连接（含指数退避重连）
func (c *GRPCClient) Connect(ctx context.Context) error {
	var opts []grpc.DialOption
	if c.tlsConfig != nil {
		opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(c.tlsConfig)))
	} else {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	backoff := time.Second
	maxBackoff := 60 * time.Second

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		conn, err := grpc.NewClient(c.endpoint, opts...)
		if err == nil {
			c.conn = conn
			c.uplinkClient = proto.NewGatewayUplinkClient(conn)
			c.connected.Store(true)
			c.degradedSince = time.Time{}
			log.Printf("[gRPC Client] 已连接到 %s", c.endpoint)
			return nil
		}

		log.Printf("[gRPC Client] 连接失败，%v 后重试: %v", backoff, err)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}

		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

// StartHeartbeat 启动心跳循环（30 秒间隔）
func (c *GRPCClient) StartHeartbeat(ctx context.Context, statusFn func() *proto.HeartbeatRequest) {
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if !c.connected.Load() {
					c.checkDegraded()
					continue
				}
				req := statusFn()
				callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
				_, err := c.uplinkClient.SyncHeartbeat(callCtx, req)
				cancel()
				if err != nil {
					log.Printf("[gRPC Client] 心跳发送失败: %v", err)
					c.connected.Store(false)
				} else {
					// 心跳成功，喂看门狗
					c.mu.Lock()
					cb := c.heartbeatCallback
					c.mu.Unlock()
					if cb != nil {
						cb()
					}
				}
			}
		}
	}()
	log.Println("[gRPC Client] 心跳循环已启动 (30s)")
}

// StartTrafficReport 启动流量上报循环（60 秒间隔）
func (c *GRPCClient) StartTrafficReport(ctx context.Context, trafficFn func() *proto.TrafficRequest) {
	go func() {
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if !c.connected.Load() {
					continue
				}
				req := trafficFn()
				callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
				_, err := c.uplinkClient.ReportTraffic(callCtx, req)
				cancel()
				if err != nil {
					log.Printf("[gRPC Client] 流量上报失败: %v", err)
				}
			}
		}
	}()
	log.Println("[gRPC Client] 流量上报循环已启动 (60s)")
}

// ReportThreat 上报威胁事件（5 秒内发送）
func (c *GRPCClient) ReportThreat(events []*proto.ThreatEvent) error {
	if !c.connected.Load() {
		c.bufferEvents(events)
		return nil
	}

	req := &proto.ThreatRequest{
		GatewayId: c.gatewayID,
		Events:    events,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := c.uplinkClient.ReportThreat(ctx, req)
	if err != nil {
		c.bufferEvents(events)
		log.Printf("[gRPC Client] 威胁上报失败，已缓存: %v", err)
	}
	return err
}

// bufferEvents 缓存事件（最多 1000 条）
func (c *GRPCClient) bufferEvents(events []*proto.ThreatEvent) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.eventBuffer = append(c.eventBuffer, events...)
	if len(c.eventBuffer) > c.maxBuffer {
		c.eventBuffer = c.eventBuffer[len(c.eventBuffer)-c.maxBuffer:]
	}
}

// GetBufferCount 获取缓存数量
func (c *GRPCClient) GetBufferCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.eventBuffer)
}

// checkDegraded 检查是否应标记为 DEGRADED
func (c *GRPCClient) checkDegraded() {
	if c.degradedSince.IsZero() {
		c.degradedSince = time.Now()
	} else if time.Since(c.degradedSince) > 300*time.Second {
		log.Println("[gRPC Client] ⚠️ OS 不可达超过 300s，标记 DEGRADED")
	}
}

// IsConnected 连接状态
func (c *GRPCClient) IsConnected() bool {
	return c.connected.Load()
}

// IsDegraded 是否处于降级状态
func (c *GRPCClient) IsDegraded() bool {
	return !c.degradedSince.IsZero() && time.Since(c.degradedSince) > 300*time.Second
}

// Close 关闭连接
func (c *GRPCClient) Close() error {
	c.connected.Store(false)
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// ReportTrafficDirect 直接上报流量（由 SensoryUplink 调用）
func (c *GRPCClient) ReportTrafficDirect(req *proto.TrafficRequest) {
	if !c.connected.Load() {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := c.uplinkClient.ReportTraffic(ctx, req); err != nil {
		log.Printf("[gRPC Client] 流量直报失败: %v", err)
	}
}

// SetHeartbeatCallback 设置心跳成功回调（用于喂看门狗）
func (c *GRPCClient) SetHeartbeatCallback(fn func()) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.heartbeatCallback = fn
}
