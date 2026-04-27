package gtunnel

import (
	"io"
	"testing"
)

// TestTransportManager_DelegatesToOrchestrator 验证 TransportManager 内部持有 Orchestrator 实例，
// 所有操作委托到 Orchestrator，确保编排主链收敛（S-01）。
//
// **Validates: Requirements 2.5**
func TestTransportManager_DelegatesToOrchestrator(t *testing.T) {
	cfg := DefaultTransportConfig()
	tm := NewTransportManager(cfg)

	// 验证内部 Orchestrator 已创建
	if tm.orchestrator == nil {
		t.Fatal("NewTransportManager 应创建内部 Orchestrator 实例")
	}

	if tm.GetOrchestrator() == nil {
		t.Fatal("GetOrchestrator() 应返回非 nil 的 Orchestrator")
	}
}

// TestTransportManager_SendDelegatesToOrchestrator 验证 Send 委托到 Orchestrator。
// 当 Orchestrator 无活跃路径时，应返回 ErrClosedPipe（与 Orchestrator.Send 行为一致）。
func TestTransportManager_SendDelegatesToOrchestrator(t *testing.T) {
	cfg := DefaultTransportConfig()
	tm := NewTransportManager(cfg)

	// Orchestrator 无活跃路径时 Send 应返回 ErrClosedPipe
	err := tm.Send([]byte("test"))
	if err != io.ErrClosedPipe {
		t.Fatalf("Send 应返回 io.ErrClosedPipe，实际: %v", err)
	}
}

// TestTransportManager_CloseDelegatesToOrchestrator 验证 Close 委托到 Orchestrator。
func TestTransportManager_CloseDelegatesToOrchestrator(t *testing.T) {
	cfg := DefaultTransportConfig()
	tm := NewTransportManager(cfg)

	err := tm.Close()
	if err != nil {
		t.Fatalf("Close 应成功，实际: %v", err)
	}
}

// TestTransportManager_SetPacketCallbackDelegatesToOrchestrator 验证回调设置委托到 Orchestrator。
func TestTransportManager_SetPacketCallbackDelegatesToOrchestrator(t *testing.T) {
	cfg := DefaultTransportConfig()
	tm := NewTransportManager(cfg)

	called := false
	cb := func(data []byte) { called = true }
	tm.SetPacketCallback(cb)

	// 验证 Orchestrator 的回调也被设置
	orch := tm.GetOrchestrator()
	orch.mu.RLock()
	orchCb := orch.onPacketRecv
	orch.mu.RUnlock()

	if orchCb == nil {
		t.Fatal("SetPacketCallback 应同时设置 Orchestrator 的回调")
	}

	// 触发回调验证
	orchCb([]byte("test"))
	if !called {
		t.Fatal("Orchestrator 回调应触发 TransportManager 设置的回调函数")
	}
}

// TestTransportManager_OrchestratorConfigMapping 验证 TransportConfig 正确映射到 OrchestratorConfig。
func TestTransportManager_OrchestratorConfigMapping(t *testing.T) {
	cfg := TransportConfig{
		ProbeInterval:    45_000_000_000, // 45s
		PromoteThreshold: 5,
	}
	tm := NewTransportManager(cfg)
	orch := tm.GetOrchestrator()

	if orch.config.ProbeCycle != cfg.ProbeInterval {
		t.Fatalf("ProbeCycle 应为 %v，实际: %v", cfg.ProbeInterval, orch.config.ProbeCycle)
	}
	if orch.config.PromoteThreshold != cfg.PromoteThreshold {
		t.Fatalf("PromoteThreshold 应为 %d，实际: %d", cfg.PromoteThreshold, orch.config.PromoteThreshold)
	}
}
