package gtclient

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"pgregory.net/rapid"
)

// --- mockTransport: 实现 Transport 接口 ---

type mockTransport struct {
	mu        sync.Mutex
	connected bool
	sendErr   error
	recvData  []byte
	recvErr   error
	closed    bool
	sendCount int32
}

func newMockTransport(connected bool) *mockTransport {
	return &mockTransport{connected: connected}
}

func (m *mockTransport) SendDatagram(data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sendErr != nil {
		return m.sendErr
	}
	atomic.AddInt32(&m.sendCount, 1)
	return nil
}

func (m *mockTransport) ReceiveDatagram(ctx context.Context) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.recvErr != nil {
		return nil, m.recvErr
	}
	return m.recvData, nil
}

func (m *mockTransport) IsConnected() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.connected
}

func (m *mockTransport) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	m.connected = false
	return nil
}

// =============================================================================
// Task 2.1: Example-based / Integration Tests
// =============================================================================

// TestDegradationAndPromotion_FullChain 降级集成测试：
// QUIC 失败→WSS 降级→数据传输→QUIC 恢复→回升→数据传输，验证全链路不中断
// 需求: 2.4
func TestDegradationAndPromotion_FullChain(t *testing.T) {
	var quicFailAtomic atomic.Int32
	quicFailAtomic.Store(1) // 1 = fail

	wssTransport := newMockTransport(true)
	wssTransport.recvData = []byte("wss-data")

	quicTransport := newMockTransport(true)
	quicTransport.recvData = []byte("quic-data")

	co := NewClientOrchestrator(ClientOrchestratorConfig{
		QUICDial: func(ctx context.Context) (Transport, error) {
			if quicFailAtomic.Load() == 1 {
				return nil, fmt.Errorf("quic unavailable")
			}
			return quicTransport, nil
		},
		WSSDial: func(ctx context.Context) (Transport, error) {
			return wssTransport, nil
		},
		FallbackTimeout:  50 * time.Millisecond,
		ProbeInterval:    5 * time.Millisecond,
		PromoteThreshold: 2,
	})

	ctx := context.Background()

	// Step 1: Connect — QUIC fails, should degrade to WSS
	if err := co.Connect(ctx); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	if co.ActiveType() != "wss" {
		t.Fatalf("expected wss after degradation, got %q", co.ActiveType())
	}

	// Step 2: Data transfer on WSS
	if err := co.SendDatagram([]byte("hello")); err != nil {
		t.Fatalf("SendDatagram on WSS failed: %v", err)
	}
	data, err := co.ReceiveDatagram(ctx)
	if err != nil {
		t.Fatalf("ReceiveDatagram on WSS failed: %v", err)
	}
	if string(data) != "wss-data" {
		t.Fatalf("expected wss-data, got %q", string(data))
	}

	// Step 3: Restore QUIC — probeAndPromote should detect and promote
	quicFailAtomic.Store(0)

	// Wait for promotion (probeInterval=5ms, threshold=2, so ~10-20ms)
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if co.ActiveType() == "quic" {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	if co.ActiveType() != "quic" {
		t.Fatalf("expected quic after promotion, got %q", co.ActiveType())
	}

	// Step 4: Data transfer on QUIC
	if err := co.SendDatagram([]byte("world")); err != nil {
		t.Fatalf("SendDatagram on QUIC failed: %v", err)
	}
	data, err = co.ReceiveDatagram(ctx)
	if err != nil {
		t.Fatalf("ReceiveDatagram on QUIC failed: %v", err)
	}
	if string(data) != "quic-data" {
		t.Fatalf("expected quic-data, got %q", string(data))
	}

	co.Close()
}

// TestAllTransportsFailed QUIC+WSS 全失败测试：验证返回 "all transports failed" 错误
// 需求: 2.8
func TestAllTransportsFailed(t *testing.T) {
	co := NewClientOrchestrator(ClientOrchestratorConfig{
		QUICDial: func(ctx context.Context) (Transport, error) {
			return nil, fmt.Errorf("quic down")
		},
		WSSDial: func(ctx context.Context) (Transport, error) {
			return nil, fmt.Errorf("wss down")
		},
		FallbackTimeout: 50 * time.Millisecond,
	})

	err := co.Connect(context.Background())
	if err == nil {
		t.Fatal("expected error when all transports fail")
	}
	if !strings.Contains(err.Error(), "all transports failed") {
		t.Fatalf("expected 'all transports failed', got %q", err.Error())
	}
	co.Close()
}

// TestDegradationAndPromotionLogs 降级/回升日志事件验证测试
// 需求: 2.5, 2.6, 2.7
func TestDegradationAndPromotionLogs(t *testing.T) {
	// Capture log output
	var logBuf bytes.Buffer
	oldWriter := log.Writer()
	log.SetOutput(&logBuf)
	t.Cleanup(func() {
		log.SetOutput(oldWriter)
	})

	var quicFailAtomic atomic.Int32
	quicFailAtomic.Store(1)

	wssTransport := newMockTransport(true)
	quicTransport := newMockTransport(true)

	co := NewClientOrchestrator(ClientOrchestratorConfig{
		QUICDial: func(ctx context.Context) (Transport, error) {
			if quicFailAtomic.Load() == 1 {
				return nil, fmt.Errorf("quic unavailable")
			}
			return quicTransport, nil
		},
		WSSDial: func(ctx context.Context) (Transport, error) {
			return wssTransport, nil
		},
		FallbackTimeout:  50 * time.Millisecond,
		ProbeInterval:    5 * time.Millisecond,
		PromoteThreshold: 2,
	})

	ctx := context.Background()

	// Connect — triggers degradation logs
	if err := co.Connect(ctx); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	// Verify degradation logs
	logStr := logBuf.String()
	if !strings.Contains(logStr, "QUIC 拨号失败") {
		t.Fatalf("missing degradation log 'QUIC 拨号失败' in:\n%s", logStr)
	}
	if !strings.Contains(logStr, "WSS 降级路径已建立") {
		t.Fatalf("missing degradation log 'WSS 降级路径已建立' in:\n%s", logStr)
	}

	// Restore QUIC and wait for promotion
	quicFailAtomic.Store(0)

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if co.ActiveType() == "quic" {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	if co.ActiveType() != "quic" {
		t.Fatalf("expected quic after promotion, got %q", co.ActiveType())
	}

	// Verify promotion logs
	logStr = logBuf.String()
	if !strings.Contains(logStr, "QUIC 探测成功") {
		t.Fatalf("missing promotion log 'QUIC 探测成功' in:\n%s", logStr)
	}
	if !strings.Contains(logStr, "已回升到 QUIC 主路径") {
		t.Fatalf("missing promotion log '已回升到 QUIC 主路径' in:\n%s", logStr)
	}

	// Verify probe count format (N/M)
	if !strings.Contains(logStr, "(1/2)") && !strings.Contains(logStr, "(2/2)") {
		t.Fatalf("missing probe count format in logs:\n%s", logStr)
	}

	co.Close()
}

// =============================================================================
// Task 2.2: Property 2 — ClientOrchestrator 降级正确性 PBT
// =============================================================================

// TestProperty_DegradationCorrectness
// Feature: phase1-link-continuity, Property 2: ClientOrchestrator degradation correctness
// 使用 rapid 生成随机 FallbackTimeout（1ms-5s），QUIC 始终失败 + WSS 成功时 ActiveType 必须为 "wss"
// **Validates: Requirements 2.1**
func TestProperty_DegradationCorrectness(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		fallbackMs := rapid.IntRange(1, 5000).Draw(t, "fallbackMs")
		fallbackTimeout := time.Duration(fallbackMs) * time.Millisecond

		wss := newMockTransport(true)

		co := NewClientOrchestrator(ClientOrchestratorConfig{
			QUICDial: func(ctx context.Context) (Transport, error) {
				return nil, fmt.Errorf("quic always fails")
			},
			WSSDial: func(ctx context.Context) (Transport, error) {
				return wss, nil
			},
			FallbackTimeout:  fallbackTimeout,
			ProbeInterval:    time.Hour, // no probing needed
			PromoteThreshold: 3,
		})

		ctx, cancel := context.WithTimeout(context.Background(), fallbackTimeout+5*time.Second)
		defer cancel()

		err := co.Connect(ctx)
		if err != nil {
			t.Fatalf("Connect should succeed via WSS, got: %v", err)
		}

		if co.ActiveType() != "wss" {
			t.Fatalf("expected ActiveType 'wss', got %q", co.ActiveType())
		}

		co.Close()
	})
}

// =============================================================================
// Task 2.3: Property 3 — ClientOrchestrator 回升正确性 PBT
// =============================================================================

// TestProperty_PromotionCorrectness
// Feature: phase1-link-continuity, Property 3: ClientOrchestrator promotion correctness
// 使用 rapid 生成随机 PromoteThreshold（1-10），WSS 降级后 QUIC 探测连续成功 N 次后 ActiveType 必须为 "quic"
// **Validates: Requirements 2.3**
func TestProperty_PromotionCorrectness(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		threshold := rapid.IntRange(1, 10).Draw(t, "promoteThreshold")

		var quicFailAtomic atomic.Int32
		quicFailAtomic.Store(1)

		wss := newMockTransport(true)

		co := NewClientOrchestrator(ClientOrchestratorConfig{
			QUICDial: func(ctx context.Context) (Transport, error) {
				if quicFailAtomic.Load() == 1 {
					return nil, fmt.Errorf("quic unavailable")
				}
				return newMockTransport(true), nil
			},
			WSSDial: func(ctx context.Context) (Transport, error) {
				return wss, nil
			},
			FallbackTimeout:  50 * time.Millisecond,
			ProbeInterval:    1 * time.Millisecond,
			PromoteThreshold: threshold,
		})

		ctx := context.Background()

		// Connect — degrades to WSS
		if err := co.Connect(ctx); err != nil {
			t.Fatalf("Connect failed: %v", err)
		}
		if co.ActiveType() != "wss" {
			t.Fatalf("expected wss after degradation, got %q", co.ActiveType())
		}

		// Enable QUIC
		quicFailAtomic.Store(0)

		// Wait for promotion: probeInterval=1ms, threshold=N, generous timeout
		deadline := time.Now().Add(time.Duration(threshold*50+500) * time.Millisecond)
		for time.Now().Before(deadline) {
			if co.ActiveType() == "quic" {
				break
			}
			time.Sleep(1 * time.Millisecond)
		}

		if co.ActiveType() != "quic" {
			t.Fatalf("expected quic after %d successful probes, got %q", threshold, co.ActiveType())
		}

		co.Close()
	})
}
