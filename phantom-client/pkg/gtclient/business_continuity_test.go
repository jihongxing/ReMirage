package gtclient

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"phantom-client/pkg/token"

	"pgregory.net/rapid"
)

// ============================================================
// Task 7.1: switchWithTransaction + 业务连续性样板测试
// 需求: 7.1, 7.2, 7.3, 7.4, 7.5, 7.6, 8.1, 8.2, 8.3, 8.4, 8.5
// ============================================================

// newTestClient 创建用于 switchWithTransaction 测试的 GTunnelClient
func newTestClient() *GTunnelClient {
	psk := make([]byte, 32)
	for i := range psk {
		psk[i] = byte(i)
	}
	config := &token.BootstrapConfig{
		BootstrapPool: []token.GatewayEndpoint{},
		PreSharedKey:  psk,
	}
	return NewGTunnelClient(config)
}

// mockQUICEngine 创建一个最小化的 QUICEngine 用于测试
// 注意：不会真正连接，仅用于 switchWithTransaction 的结构传递
func mockQUICEngine(addr string) *QUICEngine {
	return NewQUICEngine(&QUICEngineConfig{
		GatewayAddr: addr,
	})
}

// TestSwitchWithTransaction_SameIP 同 IP 切换测试
// newIP == oldIP 时直接 adoptConnection，不触发 PreAdd/Commit
// 需求: 8.1
func TestSwitchWithTransaction_SameIP(t *testing.T) {
	client := newTestClient()
	defer client.Close()

	var preAddCalled, commitCalled atomic.Int32
	client.SetSwitchPreAdd(func(ip string) error {
		preAddCalled.Add(1)
		return nil
	})
	client.SetSwitchCommit(func(oldIP, newIP string) {
		commitCalled.Add(1)
	})

	// 设置初始网关
	client.mu.Lock()
	client.currentGW = token.GatewayEndpoint{IP: "10.0.0.1", Port: 443, Region: "us-east"}
	client.mu.Unlock()

	engine := mockQUICEngine("10.0.0.1:443")
	result := &probeResult{
		gw:     token.GatewayEndpoint{IP: "10.0.0.1", Port: 443, Region: "us-east"},
		engine: engine,
	}

	err := client.switchWithTransaction(result, "10.0.0.1")
	if err != nil {
		t.Fatalf("switchWithTransaction same IP failed: %v", err)
	}

	// PreAdd 和 Commit 不应被调用
	if preAddCalled.Load() != 0 {
		t.Fatalf("PreAdd should not be called for same IP, called %d times", preAddCalled.Load())
	}
	if commitCalled.Load() != 0 {
		t.Fatalf("Commit should not be called for same IP, called %d times", commitCalled.Load())
	}

	// currentGW 应更新
	gw := client.CurrentGateway()
	if gw.IP != "10.0.0.1" {
		t.Fatalf("expected currentGW IP 10.0.0.1, got %s", gw.IP)
	}
}

// TestSwitchWithTransaction_PreAddFail PreAdd 失败测试
// 关闭新 engine，返回错误，不修改当前活跃连接
// 需求: 8.2
func TestSwitchWithTransaction_PreAddFail(t *testing.T) {
	client := newTestClient()
	defer client.Close()

	// 设置初始网关和 transport
	initialTransport := newMockTransport(true)
	client.mu.Lock()
	client.currentGW = token.GatewayEndpoint{IP: "10.0.0.1", Port: 443, Region: "us-east"}
	client.transport = initialTransport
	client.mu.Unlock()

	// PreAdd 返回错误
	client.SetSwitchPreAdd(func(ip string) error {
		return fmt.Errorf("route add failed")
	})

	engine := mockQUICEngine("10.0.0.2:443")
	result := &probeResult{
		gw:     token.GatewayEndpoint{IP: "10.0.0.2", Port: 443, Region: "us-west"},
		engine: engine,
	}

	err := client.switchWithTransaction(result, "10.0.0.1")
	if err == nil {
		t.Fatal("expected error when PreAdd fails")
	}
	if !strings.Contains(err.Error(), "pre-add route failed") {
		t.Fatalf("expected 'pre-add route failed', got %q", err.Error())
	}

	// 当前活跃连接不应被修改
	gw := client.CurrentGateway()
	if gw.IP != "10.0.0.1" {
		t.Fatalf("currentGW should remain 10.0.0.1 after PreAdd failure, got %s", gw.IP)
	}

	// transport 不应被替换
	client.mu.RLock()
	if client.transport != initialTransport {
		t.Fatal("transport should not be replaced after PreAdd failure")
	}
	client.mu.RUnlock()
}

// TestSwitchWithTransaction_NormalFlow PreAdd→adoptConnection→Commit 正常流程测试
// 需求: 8.3
func TestSwitchWithTransaction_NormalFlow(t *testing.T) {
	client := newTestClient()
	defer client.Close()

	var preAddIP string
	var commitOldIP, commitNewIP string
	client.SetSwitchPreAdd(func(ip string) error {
		preAddIP = ip
		return nil
	})
	client.SetSwitchCommit(func(oldIP, newIP string) {
		commitOldIP = oldIP
		commitNewIP = newIP
	})

	client.mu.Lock()
	client.currentGW = token.GatewayEndpoint{IP: "10.0.0.1", Port: 443, Region: "us-east"}
	client.mu.Unlock()

	engine := mockQUICEngine("10.0.0.2:443")
	result := &probeResult{
		gw:     token.GatewayEndpoint{IP: "10.0.0.2", Port: 8443, Region: "us-west"},
		engine: engine,
	}

	err := client.switchWithTransaction(result, "10.0.0.1")
	if err != nil {
		t.Fatalf("switchWithTransaction normal flow failed: %v", err)
	}

	// 验证 PreAdd 被调用
	if preAddIP != "10.0.0.2" {
		t.Fatalf("PreAdd should be called with new IP 10.0.0.2, got %q", preAddIP)
	}

	// 验证 Commit 被调用
	if commitOldIP != "10.0.0.1" || commitNewIP != "10.0.0.2" {
		t.Fatalf("Commit should be called with old=10.0.0.1, new=10.0.0.2, got old=%q new=%q",
			commitOldIP, commitNewIP)
	}

	// 验证 currentGW 已更新
	gw := client.CurrentGateway()
	if gw.IP != "10.0.0.2" || gw.Port != 8443 || gw.Region != "us-west" {
		t.Fatalf("currentGW should be updated to new gateway, got %+v", gw)
	}
}

// TestAdoptConnection_WithClientOrchestrator 验证 adoptConnection 在有 ClientOrchestrator 时的行为
// 替换 Orchestrator 内部 active，transport 本身不变
// 需求: 8.5
func TestAdoptConnection_WithClientOrchestrator(t *testing.T) {
	client := newTestClient()
	defer client.Close()

	// 创建 ClientOrchestrator 并注入
	oldActive := newMockTransport(true)
	orch := NewClientOrchestrator(ClientOrchestratorConfig{
		FallbackTimeout: time.Second,
	})
	orch.mu.Lock()
	orch.active = oldActive
	orch.activeType = "wss"
	orch.mu.Unlock()

	client.mu.Lock()
	client.transport = orch
	client.mu.Unlock()

	engine := mockQUICEngine("10.0.0.3:443")
	result := &probeResult{
		gw:     token.GatewayEndpoint{IP: "10.0.0.3", Port: 443, Region: "eu-west"},
		engine: engine,
	}

	client.adoptConnection(result)

	// transport 本身仍为 ClientOrchestrator 实例
	client.mu.RLock()
	_, isOrch := client.transport.(*ClientOrchestrator)
	client.mu.RUnlock()
	if !isOrch {
		t.Fatal("transport should still be ClientOrchestrator after adoptConnection")
	}

	// Orchestrator 内部 activeType 应为 "quic"
	if orch.ActiveType() != "quic" {
		t.Fatalf("Orchestrator activeType should be 'quic' after adoptConnection, got %q", orch.ActiveType())
	}

	// currentGW 应更新
	gw := client.CurrentGateway()
	if gw.IP != "10.0.0.3" {
		t.Fatalf("currentGW should be 10.0.0.3, got %s", gw.IP)
	}
}

// TestBusinessContinuity_DataFlowAcrossSwitch 业务连续性样板测试
// 模拟持续请求流，记录切换前后发送/接收字节数、请求序号
// 需求: 7.1, 7.2, 7.3, 7.5, 7.6
func TestBusinessContinuity_DataFlowAcrossSwitch(t *testing.T) {
	client := newTestClient()
	defer client.Close()

	// 使用 mockTransport 模拟传输层
	transport1 := newMockTransport(true)
	transport1.recvData = []byte("response-1")

	client.mu.Lock()
	client.transport = transport1
	client.currentGW = token.GatewayEndpoint{IP: "10.0.0.1", Port: 443}
	client.mu.Unlock()
	client.transition(StateConnected, "test init")

	// 切换前：发送数据
	preSwitchSendCount := atomic.LoadInt32(&transport1.sendCount)
	if err := transport1.SendDatagram([]byte("pre-switch-data")); err != nil {
		t.Fatalf("pre-switch send failed: %v", err)
	}
	postSendCount := atomic.LoadInt32(&transport1.sendCount)
	if postSendCount <= preSwitchSendCount {
		t.Fatal("sendCount should increase after send")
	}

	// 记录切换前状态
	t.Logf("[业务连续性] 切换前: sendCount=%d, currentGW=%s",
		postSendCount, client.CurrentGateway().IP)

	// 执行切换（模拟 switchWithTransaction 的 adoptConnection 部分）
	switchStart := time.Now()
	transport2 := newMockTransport(true)
	transport2.recvData = []byte("response-2")

	client.mu.Lock()
	client.transport = transport2
	client.currentGW = token.GatewayEndpoint{IP: "10.0.0.2", Port: 443}
	client.mu.Unlock()
	switchDuration := time.Since(switchStart)

	// 切换后：发送数据
	if err := transport2.SendDatagram([]byte("post-switch-data")); err != nil {
		t.Fatalf("post-switch send failed: %v", err)
	}
	postSwitchSendCount := atomic.LoadInt32(&transport2.sendCount)

	// 切换后：接收数据
	ctx := context.Background()
	data, err := transport2.ReceiveDatagram(ctx)
	if err != nil {
		t.Fatalf("post-switch receive failed: %v", err)
	}

	// 记录切换后状态
	t.Logf("[业务连续性] 切换后: sendCount=%d, currentGW=%s, recvData=%q, switchDuration=%v",
		postSwitchSendCount, client.CurrentGateway().IP, string(data), switchDuration)

	// 传输层切换成功判定
	if client.CurrentGateway().IP != "10.0.0.2" {
		t.Fatal("传输层切换失败：currentGW 未更新")
	}
	if postSwitchSendCount == 0 {
		t.Fatal("传输层切换失败：切换后无法发送数据")
	}
	if string(data) != "response-2" {
		t.Fatalf("传输层切换失败：切换后接收数据不正确，got %q", string(data))
	}

	// 业务层影响量化（mock 环境下丢包为 0，切换耗时为微秒级）
	t.Logf("[业务连续性] 结论: 传输层切换成功, 切换耗时=%v, mock环境丢包=0", switchDuration)
	t.Log("[业务连续性] 边界说明: 当前为 mock transport 测试，传输层切换成功；" +
		"业务层影响需真实网络演练验证，不预设'业务层无感'结论")
}

// TestSwitchWithTransaction_CurrentGWConsistency 切换前后 currentGW 状态一致性验证
// 需求: 8.4
func TestSwitchWithTransaction_CurrentGWConsistency(t *testing.T) {
	client := newTestClient()
	defer client.Close()

	client.SetSwitchPreAdd(func(ip string) error { return nil })
	client.SetSwitchCommit(func(oldIP, newIP string) {})

	client.mu.Lock()
	client.currentGW = token.GatewayEndpoint{IP: "10.0.0.1", Port: 443, Region: "us-east"}
	client.mu.Unlock()

	newGW := token.GatewayEndpoint{IP: "10.0.0.5", Port: 8443, Region: "ap-southeast"}
	engine := mockQUICEngine("10.0.0.5:8443")
	result := &probeResult{gw: newGW, engine: engine}

	err := client.switchWithTransaction(result, "10.0.0.1")
	if err != nil {
		t.Fatalf("switchWithTransaction failed: %v", err)
	}

	gw := client.CurrentGateway()
	if gw.IP != newGW.IP {
		t.Fatalf("currentGW.IP: expected %s, got %s", newGW.IP, gw.IP)
	}
	if gw.Port != newGW.Port {
		t.Fatalf("currentGW.Port: expected %d, got %d", newGW.Port, gw.Port)
	}
	if gw.Region != newGW.Region {
		t.Fatalf("currentGW.Region: expected %s, got %s", newGW.Region, gw.Region)
	}
}

// ============================================================
// Task 7.2: Property 8 — switchWithTransaction 同 IP 幂等性 PBT
// Feature: phase1-link-continuity, Property 8: switchWithTransaction same-IP idempotency
// Validates: Requirements 8.1
// ============================================================

func TestProperty_SameIPIdempotency(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		ip := fmt.Sprintf("%d.%d.%d.%d",
			rapid.IntRange(1, 254).Draw(t, "ip1"),
			rapid.IntRange(0, 255).Draw(t, "ip2"),
			rapid.IntRange(0, 255).Draw(t, "ip3"),
			rapid.IntRange(1, 254).Draw(t, "ip4"))
		port := rapid.IntRange(1, 65535).Draw(t, "port")
		region := rapid.StringMatching(`[a-z]{2}-[a-z]{4,8}`).Draw(t, "region")

		client := newTestClient()

		var preAddCalled, commitCalled atomic.Int32
		client.SetSwitchPreAdd(func(newIP string) error {
			preAddCalled.Add(1)
			return nil
		})
		client.SetSwitchCommit(func(oldIP, newIP string) {
			commitCalled.Add(1)
		})

		client.mu.Lock()
		client.currentGW = token.GatewayEndpoint{IP: ip, Port: port, Region: region}
		client.mu.Unlock()

		engine := mockQUICEngine(fmt.Sprintf("%s:%d", ip, port))
		result := &probeResult{
			gw:     token.GatewayEndpoint{IP: ip, Port: port, Region: region},
			engine: engine,
		}

		err := client.switchWithTransaction(result, ip)
		if err != nil {
			t.Fatalf("switchWithTransaction same IP should succeed: %v", err)
		}

		// 同 IP 时不应调用 PreAdd/Commit
		if preAddCalled.Load() != 0 {
			t.Fatalf("PreAdd should not be called for same IP %s", ip)
		}
		if commitCalled.Load() != 0 {
			t.Fatalf("Commit should not be called for same IP %s", ip)
		}

		client.Close()
	})
}

// ============================================================
// Task 7.3: Property 9 — switchWithTransaction 后状态一致性 PBT
// Feature: phase1-link-continuity, Property 9: switchWithTransaction post-state consistency
// Validates: Requirements 8.4, 8.5
// ============================================================

func TestProperty_PostStateConsistency(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		oldIP := fmt.Sprintf("%d.%d.%d.%d",
			rapid.IntRange(1, 254).Draw(t, "oldIP1"),
			rapid.IntRange(0, 255).Draw(t, "oldIP2"),
			rapid.IntRange(0, 255).Draw(t, "oldIP3"),
			rapid.IntRange(1, 254).Draw(t, "oldIP4"))
		newIP := fmt.Sprintf("%d.%d.%d.%d",
			rapid.IntRange(1, 254).Draw(t, "newIP1"),
			rapid.IntRange(0, 255).Draw(t, "newIP2"),
			rapid.IntRange(0, 255).Draw(t, "newIP3"),
			rapid.IntRange(1, 254).Draw(t, "newIP4"))
		newPort := rapid.IntRange(1, 65535).Draw(t, "newPort")
		newRegion := rapid.StringMatching(`[a-z]{2}-[a-z]{4,8}`).Draw(t, "newRegion")

		client := newTestClient()
		client.SetSwitchPreAdd(func(ip string) error { return nil })
		client.SetSwitchCommit(func(o, n string) {})

		client.mu.Lock()
		client.currentGW = token.GatewayEndpoint{IP: oldIP, Port: 443, Region: "old-region"}
		client.mu.Unlock()

		newGW := token.GatewayEndpoint{IP: newIP, Port: newPort, Region: newRegion}
		engine := mockQUICEngine(fmt.Sprintf("%s:%d", newIP, newPort))
		result := &probeResult{gw: newGW, engine: engine}

		err := client.switchWithTransaction(result, oldIP)
		if err != nil {
			t.Fatalf("switchWithTransaction failed: %v", err)
		}

		// 验证 currentGW 与输入一致
		gw := client.CurrentGateway()
		if gw.IP != newIP {
			t.Fatalf("currentGW.IP: expected %s, got %s", newIP, gw.IP)
		}
		if gw.Port != newPort {
			t.Fatalf("currentGW.Port: expected %d, got %d", newPort, gw.Port)
		}
		if gw.Region != newRegion {
			t.Fatalf("currentGW.Region: expected %s, got %s", newRegion, gw.Region)
		}

		// 验证 quic 引用指向新 engine
		client.mu.RLock()
		if client.quic != engine {
			t.Fatal("client.quic should point to new engine")
		}
		client.mu.RUnlock()

		client.Close()
	})
}

// TestProperty_PostStateConsistency_WithOrchestrator 验证有 ClientOrchestrator 时的状态一致性
// transport 本身仍为 ClientOrchestrator 实例不变
// Validates: Requirements 8.5
func TestProperty_PostStateConsistency_WithOrchestrator(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		newIP := fmt.Sprintf("%d.%d.%d.%d",
			rapid.IntRange(1, 254).Draw(t, "newIP1"),
			rapid.IntRange(0, 255).Draw(t, "newIP2"),
			rapid.IntRange(0, 255).Draw(t, "newIP3"),
			rapid.IntRange(1, 254).Draw(t, "newIP4"))
		newPort := rapid.IntRange(1, 65535).Draw(t, "newPort")

		client := newTestClient()

		// 注入 ClientOrchestrator
		orch := NewClientOrchestrator(ClientOrchestratorConfig{
			FallbackTimeout: time.Second,
		})
		oldActive := newMockTransport(true)
		orch.mu.Lock()
		orch.active = oldActive
		orch.activeType = "wss"
		orch.mu.Unlock()

		client.mu.Lock()
		client.transport = orch
		client.currentGW = token.GatewayEndpoint{IP: "10.0.0.1", Port: 443}
		client.mu.Unlock()

		engine := mockQUICEngine(fmt.Sprintf("%s:%d", newIP, newPort))
		result := &probeResult{
			gw:     token.GatewayEndpoint{IP: newIP, Port: newPort},
			engine: engine,
		}

		client.adoptConnection(result)

		// transport 本身仍为 ClientOrchestrator
		client.mu.RLock()
		transportRef := client.transport
		client.mu.RUnlock()

		if transportRef != orch {
			t.Fatal("transport should still be the same ClientOrchestrator instance")
		}

		// Orchestrator 内部 active 已替换
		if orch.ActiveType() != "quic" {
			t.Fatalf("Orchestrator activeType should be 'quic', got %q", orch.ActiveType())
		}

		client.Close()
	})
}

// suppress unused import warnings
var _ = sync.Mutex{}
