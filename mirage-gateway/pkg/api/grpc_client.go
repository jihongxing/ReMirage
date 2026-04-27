// Package api - gRPC 通信模块
package api

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log"
	"mirage-gateway/pkg/redact"
	"mirage-gateway/pkg/security"
	pb "mirage-proto/gen"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// GRPCClient 上行 gRPC 客户端
type GRPCClient struct {
	conn              *grpc.ClientConn
	uplinkClient      pb.GatewayUplinkClient
	gatewayID         string
	tlsConfig         *tls.Config
	endpoint          string
	connected         atomic.Bool
	degradedSince     time.Time
	eventBuffer       []*pb.ThreatEvent
	mu                sync.Mutex
	maxBuffer         int
	heartbeatCallback func() // 心跳成功回调（喂看门狗）
	certPin           *security.CertPin
}

// NewGRPCClient 创建客户端
func NewGRPCClient(endpoint, gatewayID string, tlsConfig *tls.Config) *GRPCClient {
	return &GRPCClient{
		endpoint:    endpoint,
		gatewayID:   gatewayID,
		tlsConfig:   tlsConfig,
		eventBuffer: make([]*pb.ThreatEvent, 0, 1000),
		maxBuffer:   1000,
	}
}

// Connect 建立连接（含指数退避重连）
func (c *GRPCClient) Connect(ctx context.Context) error {
	if c.tlsConfig == nil {
		return fmt.Errorf("mTLS 未配置，拒绝建立不安全连接")
	}

	var opts []grpc.DialOption
	// 注入证书钉扎校验
	tlsCfg := c.tlsConfig
	if c.certPin != nil {
		tlsCfg = c.tlsConfig.Clone()
		tlsCfg.VerifyPeerCertificate = func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
			if len(rawCerts) == 0 {
				return fmt.Errorf("no peer certificate")
			}
			cert, err := x509.ParseCertificate(rawCerts[0])
			if err != nil {
				return err
			}
			if !c.certPin.IsPinned() {
				// TOFU: 首次连接自动钉扎
				c.certPin.PinCertificate(cert)
				return nil
			}
			return c.certPin.VerifyPin(cert)
		}
	}
	opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(tlsCfg)))

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
			c.uplinkClient = pb.NewGatewayUplinkClient(conn)
			c.connected.Store(true)
			c.degradedSince = time.Time{}
			log.Printf("[gRPC Client] 已连接到 %s", c.endpoint)
			// 连接成功后 flush 缓存的威胁事件
			c.flushEventBuffer()
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
func (c *GRPCClient) StartHeartbeat(ctx context.Context, statusFn func() *pb.HeartbeatRequest) {
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
				resp, err := c.uplinkClient.SyncHeartbeat(callCtx, req)
				cancel()
				if err != nil {
					log.Printf("[gRPC Client] 心跳发送失败: %v", err)
					c.connected.Store(false)
				} else {
					// 检查是否需要全量同步
					if resp.NeedsFullSync {
						log.Printf("[gRPC Client] OS 要求全量状态同步 (desired_hash=%s)", resp.DesiredStateHash)
						// TODO: 拉取全量 Desired State 并对齐（通过 MotorDownlink）
					}
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
func (c *GRPCClient) StartTrafficReport(ctx context.Context, trafficFn func() *pb.TrafficRequest) {
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
func (c *GRPCClient) ReportThreat(events []*pb.ThreatEvent) error {
	if !c.connected.Load() {
		c.bufferEvents(events)
		return nil
	}

	req := &pb.ThreatRequest{
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
func (c *GRPCClient) bufferEvents(events []*pb.ThreatEvent) {
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

// flushEventBuffer 将缓存的威胁事件批量发送
func (c *GRPCClient) flushEventBuffer() {
	c.mu.Lock()
	if len(c.eventBuffer) == 0 {
		c.mu.Unlock()
		return
	}
	events := make([]*pb.ThreatEvent, len(c.eventBuffer))
	copy(events, c.eventBuffer)
	c.eventBuffer = c.eventBuffer[:0]
	c.mu.Unlock()

	log.Printf("[gRPC Client] 开始 flush 缓存威胁事件: %d 条", len(events))

	req := &pb.ThreatRequest{
		GatewayId: c.gatewayID,
		Events:    events,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := c.uplinkClient.ReportThreat(ctx, req)
	if err != nil {
		log.Printf("[gRPC Client] flush 缓存事件失败，重新缓存: %v", err)
		c.bufferEvents(events)
	} else {
		log.Printf("[gRPC Client] ✅ 缓存威胁事件已补发: %d 条", len(events))
	}
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
func (c *GRPCClient) ReportTrafficDirect(req *pb.TrafficRequest) {
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

// SetCertPin 设置证书钉扎管理器
func (c *GRPCClient) SetCertPin(pin *security.CertPin) {
	c.certPin = pin
}

// Register 向 OS 注册 Gateway
func (c *GRPCClient) Register(ctx context.Context, gatewayID, cellID, downlinkAddr, version string) error {
	if !c.connected.Load() {
		return fmt.Errorf("gRPC 未连接")
	}
	callCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	resp, err := c.uplinkClient.RegisterGateway(callCtx, &pb.RegisterRequest{
		GatewayId:    gatewayID,
		CellId:       cellID,
		DownlinkAddr: downlinkAddr,
		Version:      version,
		Capabilities: &pb.GatewayCapabilities{
			EbpfSupported:  true,
			MaxConnections: 10000,
			MaxSessions:    5000,
		},
	})
	if err != nil {
		return fmt.Errorf("register RPC failed: %w", err)
	}
	if !resp.Success {
		return fmt.Errorf("register rejected: %s", resp.Message)
	}
	log.Printf("[gRPC Client] Gateway 注册成功 (assigned_cell=%s)", resp.AssignedCellId)
	return nil
}

// ReportSessionEvent 上报会话事件（连接/断开）
func (c *GRPCClient) ReportSessionEvent(ctx context.Context, req *pb.SessionEventRequest) error {
	if !c.connected.Load() {
		return fmt.Errorf("gRPC 未连接")
	}
	callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err := c.uplinkClient.ReportSessionEvent(callCtx, req)
	if err != nil {
		log.Printf("[gRPC Client] 会话事件上报失败: %v", err)
	}
	return err
}

// StartTrafficReportByUser 启动按用户流量上报循环（60 秒间隔）
func (c *GRPCClient) StartTrafficReportByUser(ctx context.Context, flushFn func() []*TrafficStats, seqFn func() uint64) {
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
				stats := flushFn()
				for _, s := range stats {
					if s.BusinessBytes == 0 && s.DefenseBytes == 0 {
						continue
					}
					req := &pb.TrafficRequest{
						GatewayId:      c.gatewayID,
						Timestamp:      time.Now().Unix(),
						BusinessBytes:  s.BusinessBytes,
						DefenseBytes:   s.DefenseBytes,
						PeriodSeconds:  60,
						UserId:         s.UserID,
						SessionId:      s.SessionID,
						SequenceNumber: seqFn(),
					}
					callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
					_, err := c.uplinkClient.ReportTraffic(callCtx, req)
					cancel()
					if err != nil {
						log.Printf("[gRPC Client] 用户流量上报失败 (user=%s): %v", redact.RedactToken(s.UserID), err)
					}
				}
			}
		}
	}()
	log.Println("[gRPC Client] 按用户流量上报循环已启动 (60s)")
}
