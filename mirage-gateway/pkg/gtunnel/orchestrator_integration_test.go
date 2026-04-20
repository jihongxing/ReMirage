package gtunnel

import (
	"io"
	"net"
	"sync/atomic"
	"testing"
	"time"
)

// mockTransportConn 用于集成测试的 mock TransportConn
type mockTransportConn struct {
	transportType TransportType
	rtt           time.Duration
	maxDatagram   int
	sendErr       error
	recvData      chan []byte
	closed        int32
}

func newMockConn(t TransportType, rtt time.Duration) *mockTransportConn {
	return &mockTransportConn{
		transportType: t,
		rtt:           rtt,
		maxDatagram:   1200,
		recvData:      make(chan []byte, 16),
	}
}

func (m *mockTransportConn) Send(data []byte) error {
	if atomic.LoadInt32(&m.closed) == 1 {
		return io.ErrClosedPipe
	}
	return m.sendErr
}

func (m *mockTransportConn) Recv() ([]byte, error) {
	if atomic.LoadInt32(&m.closed) == 1 {
		return nil, io.EOF
	}
	data, ok := <-m.recvData
	if !ok {
		return nil, io.EOF
	}
	return data, nil
}

func (m *mockTransportConn) Close() error {
	atomic.StoreInt32(&m.closed, 1)
	return nil
}

func (m *mockTransportConn) Type() TransportType  { return m.transportType }
func (m *mockTransportConn) RTT() time.Duration   { return m.rtt }
func (m *mockTransportConn) RemoteAddr() net.Addr { return &net.IPAddr{IP: net.IPv4(1, 2, 3, 4)} }
func (m *mockTransportConn) MaxDatagramSize() int { return m.maxDatagram }

// TestOrchestrator_DemotePromote 测试降格/升格端到端流程
func TestOrchestrator_DemotePromote(t *testing.T) {
	cfg := DefaultOrchestratorConfig()
	cfg.PromoteThreshold = 2

	o := NewOrchestrator(cfg)

	// 模拟两条路径
	quicConn := newMockConn(TransportQUIC, 20*time.Millisecond)
	wssConn := newMockConn(TransportWebSocket, 150*time.Millisecond)

	o.paths[TransportQUIC] = &ManagedPath{
		Conn:        quicConn,
		Priority:    PriorityQUIC,
		Type:        TransportQUIC,
		Enabled:     true,
		Available:   true,
		BaselineRTT: 20 * time.Millisecond,
		Phase:       1,
	}
	o.paths[TransportWebSocket] = &ManagedPath{
		Conn:        wssConn,
		Priority:    PriorityWSS,
		Type:        TransportWebSocket,
		Enabled:     true,
		Available:   true,
		BaselineRTT: 150 * time.Millisecond,
		Phase:       1,
	}
	o.activePath = o.paths[TransportQUIC]
	o.state = StateOrcActive

	// 验证初始状态
	if o.GetActiveType() != TransportQUIC {
		t.Fatalf("初始活跃路径应为 QUIC，got %d", o.GetActiveType())
	}

	// 模拟 QUIC 劣化 → 降格
	o.paths[TransportQUIC].Available = false
	err := o.demote()
	if err != nil {
		t.Fatalf("降格失败: %v", err)
	}

	if o.GetActiveType() != TransportWebSocket {
		t.Fatalf("降格后应切换到 WSS，got %d", o.GetActiveType())
	}

	// 验证 epoch 递增
	if o.GetEpoch() == 0 {
		t.Fatal("降格后 epoch 应递增")
	}

	// 模拟 QUIC 恢复 → 升格
	o.paths[TransportQUIC].Available = true
	o.paths[TransportQUIC].ProbeSuccess = 0

	err = o.promote(TransportQUIC)
	if err != nil {
		t.Fatalf("升格失败: %v", err)
	}

	if o.GetActiveType() != TransportQUIC {
		t.Fatalf("升格后应切换回 QUIC，got %d", o.GetActiveType())
	}

	// 验证 epoch 再次递增
	if o.GetEpoch() < 2 {
		t.Fatalf("升格后 epoch 应 >= 2，got %d", o.GetEpoch())
	}

	o.Close()
}

// TestOrchestrator_DemoteNoFallback 测试无可用降格路径
func TestOrchestrator_DemoteNoFallback(t *testing.T) {
	cfg := DefaultOrchestratorConfig()
	o := NewOrchestrator(cfg)

	quicConn := newMockConn(TransportQUIC, 20*time.Millisecond)
	o.paths[TransportQUIC] = &ManagedPath{
		Conn:      quicConn,
		Priority:  PriorityQUIC,
		Type:      TransportQUIC,
		Enabled:   true,
		Available: true,
		Phase:     1,
	}
	o.activePath = o.paths[TransportQUIC]
	o.state = StateOrcActive

	err := o.demote()
	if err == nil {
		t.Fatal("无可用降格路径时应返回错误")
	}

	o.Close()
}

// TestOrchestrator_PromoteFailsIfLowerPriority 测试升格到低优先级路径应失败
func TestOrchestrator_PromoteFailsIfLowerPriority(t *testing.T) {
	cfg := DefaultOrchestratorConfig()
	o := NewOrchestrator(cfg)

	quicConn := newMockConn(TransportQUIC, 20*time.Millisecond)
	wssConn := newMockConn(TransportWebSocket, 150*time.Millisecond)

	o.paths[TransportQUIC] = &ManagedPath{
		Conn:      quicConn,
		Priority:  PriorityQUIC,
		Type:      TransportQUIC,
		Enabled:   true,
		Available: true,
		Phase:     1,
	}
	o.paths[TransportWebSocket] = &ManagedPath{
		Conn:      wssConn,
		Priority:  PriorityWSS,
		Type:      TransportWebSocket,
		Enabled:   true,
		Available: true,
		Phase:     1,
	}
	o.activePath = o.paths[TransportQUIC]
	o.state = StateOrcActive

	// 尝试升格到 WSS（优先级更低），应失败
	err := o.promote(TransportWebSocket)
	if err == nil {
		t.Fatal("升格到低优先级路径应失败")
	}

	o.Close()
}

// TestOrchestrator_MTUNotification 测试路径切换时 FEC MTU 通知
func TestOrchestrator_MTUNotification(t *testing.T) {
	cfg := DefaultOrchestratorConfig()
	o := NewOrchestrator(cfg)

	// DNS 传输 MTU 很小
	dnsConn := &mockTransportConn{
		transportType: TransportDNS,
		rtt:           500 * time.Millisecond,
		maxDatagram:   MaxDNSDatagramSize, // 110
		recvData:      make(chan []byte, 16),
	}

	o.notifyFECMTU(dnsConn)

	// 验证 FEC 分片大小已调整
	expectedSize := MaxDNSDatagramSize - 24 // 110 - 24 = 86
	if o.fec.shardSize != expectedSize {
		t.Fatalf("FEC shardSize 应为 %d，got %d", expectedSize, o.fec.shardSize)
	}

	// 切换回 QUIC（大 MTU）
	quicConn := newMockConn(TransportQUIC, 20*time.Millisecond)
	o.notifyFECMTU(quicConn)

	// 应恢复到默认 ShardSize
	if o.fec.shardSize != ShardSize {
		t.Fatalf("FEC shardSize 应恢复为 %d，got %d", ShardSize, o.fec.shardSize)
	}

	o.Close()
}
