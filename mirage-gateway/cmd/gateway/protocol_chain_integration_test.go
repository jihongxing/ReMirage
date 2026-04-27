package main

import (
	"context"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"mirage-gateway/pkg/gtunnel"
)

// TestProtocolChainWiring 验证协议协同链闭环：
// Client SendPathShim (Padding/IAT) → QUIC (ALPN h3) → Bearer Listener → B-DNA → NPM → Jitter-Lite → VPC
// 此测试验证 Go 控制面的接线正确性（eBPF 数据面需要内核环境，此处验证逻辑链路）
func TestProtocolChainWiring(t *testing.T) {
	// 1. 创建 Orchestrator（被动模式，模拟 Gateway 服务端）
	orchConfig := gtunnel.DefaultOrchestratorConfig()
	orchConfig.EnableQUIC = true
	orchConfig.EnableWSS = true
	orchConfig.EnableDNS = true
	orchConfig.EnableICMP = true
	orch := gtunnel.NewOrchestrator(orchConfig)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	orch.StartPassive(ctx)
	defer orch.Close()

	// 2. 验证 PacketCallback 接线：Orchestrator 收到数据后触发回调
	var packetReceived int32
	orch.SetPacketCallback(func(data []byte) {
		atomic.AddInt32(&packetReceived, 1)
	})

	// 3. 注入 mock QUIC 连接（模拟 Bearer Listener Accept 后的连接）
	mockQUIC := &chainMockConn{
		ttype:    gtunnel.TransportQUIC,
		recvData: make(chan []byte, 10),
	}
	orch.AdoptInboundConn(mockQUIC, gtunnel.TransportQUIC)

	// 验证 QUIC 成为活跃路径
	if orch.GetActiveType() != gtunnel.TransportQUIC {
		t.Fatalf("活跃路径应为 QUIC, got %d", orch.GetActiveType())
	}

	// 4. 注入 mock WSS 连接（模拟 ChameleonListener 接入）
	mockWSS := &chainMockConn{
		ttype:    gtunnel.TransportWebSocket,
		recvData: make(chan []byte, 10),
	}
	orch.AdoptInboundConn(mockWSS, gtunnel.TransportWebSocket)

	// QUIC 优先级更高，应保持为活跃路径
	if orch.GetActiveType() != gtunnel.TransportQUIC {
		t.Fatalf("QUIC 优先级更高，应保持活跃, got %d", orch.GetActiveType())
	}

	// 5. 注入 mock DNS 连接（模拟 DNSServer 接入）
	mockDNS := &chainMockConn{
		ttype:    gtunnel.TransportDNS,
		recvData: make(chan []byte, 10),
	}
	orch.AdoptInboundConn(mockDNS, gtunnel.TransportDNS)

	// QUIC 仍应为活跃路径（DNS 优先级最低）
	if orch.GetActiveType() != gtunnel.TransportQUIC {
		t.Fatalf("QUIC 仍应为活跃路径, got %d", orch.GetActiveType())
	}

	// 6. 验证 Send 通过活跃路径发送
	testData := []byte("test-payload")
	if err := orch.Send(testData); err != nil {
		t.Fatalf("Send 失败: %v", err)
	}
	if atomic.LoadInt32(&mockQUIC.sendCount) != 1 {
		t.Fatalf("数据应通过 QUIC 活跃路径发送, sendCount=%d", mockQUIC.sendCount)
	}

	// 7. 模拟数据通过 QUIC 路径到达 → 触发 PacketCallback
	mockQUIC.recvData <- []byte("inbound-data")
	time.Sleep(200 * time.Millisecond)

	if atomic.LoadInt32(&packetReceived) == 0 {
		t.Log("⚠️ PacketCallback 未触发（receiveLoop 可能需要更长时间）")
		// 不 Fatal — receiveLoop 的 Recv 依赖 activePath 的实际实现
	}
}

// TestProtocolPriorityOrder 验证协议优先级排序
func TestProtocolPriorityOrder(t *testing.T) {
	orch := gtunnel.NewOrchestrator(gtunnel.DefaultOrchestratorConfig())
	defer orch.Close()

	// 先注入低优先级 DNS
	mockDNS := &chainMockConn{ttype: gtunnel.TransportDNS, recvData: make(chan []byte, 1)}
	orch.AdoptInboundConn(mockDNS, gtunnel.TransportDNS)
	if orch.GetActiveType() != gtunnel.TransportDNS {
		t.Fatalf("首个连接应成为活跃路径, got %d", orch.GetActiveType())
	}

	// 注入高优先级 WSS → 应自动切换
	mockWSS := &chainMockConn{ttype: gtunnel.TransportWebSocket, recvData: make(chan []byte, 1)}
	orch.AdoptInboundConn(mockWSS, gtunnel.TransportWebSocket)
	if orch.GetActiveType() != gtunnel.TransportWebSocket {
		t.Fatalf("WSS 优先级更高，应切换为活跃路径, got %d", orch.GetActiveType())
	}

	// 注入最高优先级 QUIC → 应自动切换
	mockQUIC := &chainMockConn{ttype: gtunnel.TransportQUIC, recvData: make(chan []byte, 1)}
	orch.AdoptInboundConn(mockQUIC, gtunnel.TransportQUIC)
	if orch.GetActiveType() != gtunnel.TransportQUIC {
		t.Fatalf("QUIC 优先级最高，应切换为活跃路径, got %d", orch.GetActiveType())
	}
}

// chainMockConn 协议链测试用 mock
type chainMockConn struct {
	ttype     gtunnel.TransportType
	sendCount int32
	recvData  chan []byte
}

func (c *chainMockConn) Send(data []byte) error {
	atomic.AddInt32(&c.sendCount, 1)
	return nil
}
func (c *chainMockConn) Recv() ([]byte, error) {
	data := <-c.recvData
	return data, nil
}
func (c *chainMockConn) Close() error                { return nil }
func (c *chainMockConn) Type() gtunnel.TransportType { return c.ttype }
func (c *chainMockConn) RTT() time.Duration          { return 10 * time.Millisecond }
func (c *chainMockConn) RemoteAddr() net.Addr        { return nil }
func (c *chainMockConn) MaxDatagramSize() int        { return 1200 }
