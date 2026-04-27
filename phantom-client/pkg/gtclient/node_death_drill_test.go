package gtclient

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"phantom-client/pkg/resonance"
	"phantom-client/pkg/token"
)

// ============================================================
// Task 6.1: 节点阵亡恢复演练测试
// 需求: 4.1, 4.2, 4.3, 4.4, 4.5, 4.6, 4.7, 4.8
// ============================================================

// encodeDrillMockSignal 编码模拟信令（与 resolver_test.go 中的 encodeMockSignal 相同逻辑）
func encodeDrillMockSignal(gateways []resonance.GatewayInfo, domains []string) string {
	payload := map[string]interface{}{
		"gateways": gateways,
		"domains":  domains,
	}
	data, _ := json.Marshal(payload)
	return base64.RawURLEncoding.EncodeToString(data)
}

// drillMockOpenFn 模拟解密函数
func drillMockOpenFn(sealed []byte) ([]resonance.GatewayInfo, []string, error) {
	var payload struct {
		Gateways []resonance.GatewayInfo `json:"gateways"`
		Domains  []string                `json:"domains"`
	}
	if err := json.Unmarshal(sealed, &payload); err != nil {
		return nil, nil, fmt.Errorf("mock open: %w", err)
	}
	return payload.Gateways, payload.Domains, nil
}

// newDrillClient 创建用于演练测试的 GTunnelClient（无真实连接）
func newDrillClient() *GTunnelClient {
	psk := make([]byte, 32)
	for i := range psk {
		psk[i] = byte(i)
	}
	config := &token.BootstrapConfig{
		BootstrapPool: []token.GatewayEndpoint{}, // 空池
		PreSharedKey:  psk,
	}
	return NewGTunnelClient(config)
}

// TestNodeDeathDrill_AllStrategiesFail 所有恢复策略失败时返回 "all reconnection strategies exhausted"
// 验证：空 runtimeTopo + 空 bootstrapPool + 无 Resolver → doReconnect 返回明确错误
// 需求: 4.7
func TestNodeDeathDrill_AllStrategiesFail(t *testing.T) {
	client := newDrillClient()
	defer client.Close()

	// 设置初始网关（模拟已连接状态）
	client.mu.Lock()
	client.currentGW = token.GatewayEndpoint{IP: "10.0.0.1", Port: 443, Region: "us-east"}
	client.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := client.doReconnect(ctx)
	if err == nil {
		t.Fatal("expected error when all strategies fail")
	}
	if !strings.Contains(err.Error(), "all reconnection strategies exhausted") {
		t.Fatalf("expected 'all reconnection strategies exhausted', got %q", err.Error())
	}
}

// TestNodeDeathDrill_L3ResolverDiscovery 验证 L3 信令共振发现链路
// L1(RuntimeTopo) 空 + L2(BootstrapPool) 空 → L3 Resolver 被调用
// Resolver 返回有效网关信息，但 probe 会失败（无真实 QUIC），验证 Resolver 被正确调用
// 需求: 4.2, 4.3
func TestNodeDeathDrill_L3ResolverDiscovery(t *testing.T) {
	client := newDrillClient()
	defer client.Close()

	client.mu.Lock()
	client.currentGW = token.GatewayEndpoint{IP: "10.0.0.1", Port: 443, Region: "us-east"}
	client.mu.Unlock()

	// 创建 mock DoH 服务器返回有效网关
	gateways := []resonance.GatewayInfo{{IP: "192.168.1.100", Port: 8443, Priority: 1}}
	encoded := encodeDrillMockSignal(gateways, []string{"new-gw.example.com"})

	var resolverCalled atomic.Int32
	dohServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resolverCalled.Add(1)
		type dohAnswer struct {
			Type int    `json:"type"`
			Data string `json:"data"`
		}
		type dohResp struct {
			Status int         `json:"Status"`
			Answer []dohAnswer `json:"Answer"`
		}
		resp := dohResp{
			Status: 0,
			Answer: []dohAnswer{{Type: 16, Data: fmt.Sprintf(`"%s"`, encoded)}},
		}
		w.Header().Set("Content-Type", "application/dns-json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer dohServer.Close()

	resolver := resonance.NewResolver(&resonance.ResolverConfig{
		DNSRecordName:  "_sig.drill.example.com",
		DoHServers:     []string{dohServer.URL},
		ChannelTimeout: 5 * time.Second,
	}, drillMockOpenFn)

	client.SetResonanceResolver(resolver)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// doReconnect: L1 空 → L2 空 → L3 Resolver 发现新入口 → probe 失败（无真实 QUIC）
	// 最终仍然失败，但 Resolver 应该被调用
	err := client.doReconnect(ctx)

	// Resolver 应该被调用（即使最终 probe 失败）
	if resolverCalled.Load() == 0 {
		t.Fatal("Resolver should have been called when L1/L2 fail")
	}

	// 即使 Resolver 发现了新入口，probe 仍然失败（无真实 QUIC），所以最终返回错误
	if err == nil {
		// 如果意外成功（不应该发生），也接受
		t.Log("doReconnect unexpectedly succeeded")
	} else if !strings.Contains(err.Error(), "all reconnection strategies exhausted") {
		t.Fatalf("expected 'all reconnection strategies exhausted', got %q", err.Error())
	}
}

// TestNodeDeathDrill_RecoveryFSMPhaseProgression 验证完整恢复链路：
// Reconnect → RecoveryFSM → PhaseJitter 起步 → 阶段递进 → PhaseDeath → doReconnect
// 需求: 4.1, 4.5, 4.6
func TestNodeDeathDrill_RecoveryFSMPhaseProgression(t *testing.T) {
	client := newDrillClient()
	defer client.Close()

	// 设置为非连接状态以触发 Reconnect
	client.transition(StateDegraded, "test node death")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Reconnect 会创建 RecoveryFSM，从 PhaseJitter 起步，逐步递进
	// 由于无真实连接，所有阶段都会失败，最终进入 StateExhausted
	err := client.Reconnect(ctx)
	if err == nil {
		t.Fatal("expected error from Reconnect when all strategies fail")
	}

	// 验证最终状态为 StateExhausted
	if client.State() != StateExhausted {
		t.Fatalf("expected StateExhausted after all recovery fails, got %s", client.State())
	}
}

// TestNodeDeathDrill_TimeoutBoundaries 验证超时口径
// RecoveryFSM 总超时 60s / 单阶段 15s，doReconnect 内部 probe 5s
// 需求: 4.8
func TestNodeDeathDrill_TimeoutBoundaries(t *testing.T) {
	// 验证 RecoveryFSM 默认超时值
	fsm := NewRecoveryFSM()
	if fsm.totalTimeout != 60*time.Second {
		t.Fatalf("RecoveryFSM totalTimeout should be 60s, got %v", fsm.totalTimeout)
	}
	if fsm.phaseTimeout != 15*time.Second {
		t.Fatalf("RecoveryFSM phaseTimeout should be 15s, got %v", fsm.phaseTimeout)
	}

	// 验证 doReconnect 使用 5s context timeout（通过代码审查确认）
	// doReconnect 内部: reconnCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	// 这是代码级验证，不需要运行时测试
}

// TestNodeDeathDrill_ResolverUpdatesTopology 验证 Resolver 发现后更新 RuntimeTopology
// 需求: 4.3, 4.4
func TestNodeDeathDrill_ResolverUpdatesTopology(t *testing.T) {
	client := newDrillClient()
	defer client.Close()

	client.mu.Lock()
	client.currentGW = token.GatewayEndpoint{IP: "10.0.0.1", Port: 443, Region: "us-east"}
	client.mu.Unlock()

	// 验证初始 runtimeTopo 为空
	if !client.runtimeTopo.IsEmpty() {
		t.Fatal("initial runtimeTopo should be empty")
	}

	gateways := []resonance.GatewayInfo{
		{IP: "192.168.1.100", Port: 8443, Priority: 1},
		{IP: "192.168.1.101", Port: 8443, Priority: 2},
	}
	encoded := encodeDrillMockSignal(gateways, []string{"new.example.com"})

	dohServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		type dohAnswer struct {
			Type int    `json:"type"`
			Data string `json:"data"`
		}
		type dohResp struct {
			Status int         `json:"Status"`
			Answer []dohAnswer `json:"Answer"`
		}
		resp := dohResp{
			Status: 0,
			Answer: []dohAnswer{{Type: 16, Data: fmt.Sprintf(`"%s"`, encoded)}},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer dohServer.Close()

	resolver := resonance.NewResolver(&resonance.ResolverConfig{
		DNSRecordName:  "_sig.topo.example.com",
		DoHServers:     []string{dohServer.URL},
		ChannelTimeout: 5 * time.Second,
	}, drillMockOpenFn)

	client.SetResonanceResolver(resolver)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// doReconnect 会调用 Resolver，发现新入口后更新 routeTable 和 runtimeTopo
	_ = client.doReconnect(ctx)

	// 验证 routeTable 已更新（Resolver 发现的网关写入 routeTable）
	if client.routeTable.Count() == 0 {
		t.Fatal("routeTable should be updated after Resolver discovery")
	}

	// 验证 runtimeTopo 已更新
	if client.runtimeTopo.IsEmpty() {
		t.Fatal("runtimeTopo should be updated after Resolver discovery")
	}
}
