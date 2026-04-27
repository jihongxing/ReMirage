package main

import (
	"net"
	"testing"
	"time"

	"mirage-gateway/pkg/gtunnel"
)

// TestDNSServerStartupAndCallback 验证 DNSServer 创建、回调注册和启动流程
func TestDNSServerStartupAndCallback(t *testing.T) {
	srv, err := gtunnel.NewDNSServer("test.example.com", "127.0.0.1:15353")
	if err != nil {
		t.Fatalf("NewDNSServer 失败: %v", err)
	}

	srv.SetRecvCallback(func(clientID string, data []byte) {})

	// Start() 是阻塞的（ListenAndServe），需要在 goroutine 中运行
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start()
	}()

	// 等待服务器启动
	time.Sleep(200 * time.Millisecond)

	// 检查是否有启动错误
	select {
	case err := <-errCh:
		t.Fatalf("DNSServer.Start 失败: %v", err)
	default:
	}

	defer srv.Stop()

	// 验证服务器正在监听
	conn, err := net.Dial("udp", "127.0.0.1:15353")
	if err != nil {
		t.Fatalf("无法连接到 DNS 服务器: %v", err)
	}
	conn.Close()
}

// TestDNSServerCreationFailsWithEmptyDomain 验证空域名时创建失败
func TestDNSServerCreationFailsWithEmptyDomain(t *testing.T) {
	_, err := gtunnel.NewDNSServer("", ":15354")
	if err == nil {
		t.Fatal("空域名应返回错误")
	}
}

// TestWebRTCAnswererCreation 验证 WebRTCAnswerer 创建流程
func TestWebRTCAnswererCreation(t *testing.T) {
	config := gtunnel.WebRTCTransportConfig{}
	sendCtrl := func(ctrlType byte, payload []byte) error {
		return nil
	}

	answerer := gtunnel.NewWebRTCAnswerer(config, sendCtrl)
	if answerer == nil {
		t.Fatal("NewWebRTCAnswerer 返回 nil")
	}
	if answerer.IsConnected() {
		t.Fatal("新创建的 Answerer 不应处于已连接状态")
	}
	answerer.Close()
}

// TestCtrlFrameRouterControlFrameDetection 验证控制帧识别
func TestCtrlFrameRouterControlFrameDetection(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		expected bool
	}{
		{"有效控制帧", []byte{0xFE, 0x10, 0x01, 0x02}, true},
		{"数据帧", []byte{0x00, 0x10, 0x01, 0x02}, false},
		{"空数据", []byte{}, false},
		{"单字节", []byte{0xFE}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := gtunnel.IsControlFrame(tt.data)
			if result != tt.expected {
				t.Errorf("IsControlFrame(%v) = %v, want %v", tt.data, result, tt.expected)
			}
		})
	}
}

// TestOrchestratorAdoptInboundConn 验证 Orchestrator 入站连接注入
func TestOrchestratorAdoptInboundConn(t *testing.T) {
	config := gtunnel.DefaultOrchestratorConfig()
	config.EnableDNS = true
	config.EnableICMP = true
	config.EnableWebRTC = true
	orch := gtunnel.NewOrchestrator(config)
	defer orch.Close()

	if orch.GetState() != gtunnel.StateOrcProbing {
		t.Fatalf("初始状态应为 Probing, got %d", orch.GetState())
	}

	mock := &mockTransportConn{transportType: gtunnel.TransportDNS}
	orch.AdoptInboundConn(mock, gtunnel.TransportDNS)

	if orch.GetState() != gtunnel.StateOrcActive {
		t.Fatalf("注入连接后状态应为 Active, got %d", orch.GetState())
	}

	if orch.GetActiveType() != gtunnel.TransportDNS {
		t.Fatalf("活跃路径应为 DNS, got %d", orch.GetActiveType())
	}
}

// mockTransportConn 用于测试的 mock TransportConn
type mockTransportConn struct {
	transportType gtunnel.TransportType
}

func (m *mockTransportConn) Send(data []byte) error      { return nil }
func (m *mockTransportConn) Recv() ([]byte, error)       { return nil, nil }
func (m *mockTransportConn) Close() error                { return nil }
func (m *mockTransportConn) Type() gtunnel.TransportType { return m.transportType }
func (m *mockTransportConn) RTT() time.Duration          { return 0 }
func (m *mockTransportConn) RemoteAddr() net.Addr        { return nil }
func (m *mockTransportConn) MaxDatagramSize() int        { return 1200 }
