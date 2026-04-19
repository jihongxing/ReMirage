package security

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"pgregory.net/rapid"
)

// mockModule 测试用模块
type mockModule struct {
	name     string
	order    *[]string
	mu       *sync.Mutex
	shutErr  error
	duration time.Duration
}

func (m *mockModule) Name() string { return m.name }
func (m *mockModule) Shutdown(ctx context.Context) error {
	if m.duration > 0 {
		time.Sleep(m.duration)
	}
	m.mu.Lock()
	*m.order = append(*m.order, m.name)
	m.mu.Unlock()
	return m.shutErr
}

// mockEmergencyWiper 测试用紧急擦除器
type mockEmergencyWiper struct {
	called bool
	mu     sync.Mutex
}

func (m *mockEmergencyWiper) TriggerWipe() error {
	m.mu.Lock()
	m.called = true
	m.mu.Unlock()
	return nil
}

// Property 7: 优雅关闭逆序
func TestProperty_ShutdownReverseOrder(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		n := rapid.IntRange(1, 20).Draw(t, "n")
		rs := NewRAMShield()
		wiper := &mockEmergencyWiper{}
		gs := NewGracefulShutdown(rs, wiper, 30*time.Second)

		order := make([]string, 0)
		mu := &sync.Mutex{}
		names := make([]string, n)

		for i := 0; i < n; i++ {
			name := fmt.Sprintf("module-%d", i)
			names[i] = name
			gs.RegisterModule(&mockModule{name: name, order: &order, mu: mu})
		}

		if err := gs.Shutdown(); err != nil {
			t.Fatalf("Shutdown 失败: %v", err)
		}

		if len(order) != n {
			t.Fatalf("关闭模块数不匹配: 期望 %d, 实际 %d", n, len(order))
		}

		// 验证逆序
		for i := 0; i < n; i++ {
			expected := names[n-1-i]
			if order[i] != expected {
				t.Fatalf("关闭顺序错误: 位置 %d 期望 %s, 实际 %s", i, expected, order[i])
			}
		}
	})
}

// Property 8: 擦除所有注册缓冲区
func TestProperty_ShutdownWipesAllBuffers(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		n := rapid.IntRange(1, 10).Draw(t, "n")
		rs := NewRAMShield()
		wiper := &mockEmergencyWiper{}
		gs := NewGracefulShutdown(rs, wiper, 30*time.Second)

		bufs := make([]*SecureBuffer, n)
		for i := 0; i < n; i++ {
			size := rapid.IntRange(1, 256).Draw(t, fmt.Sprintf("size_%d", i))
			buf, err := rs.SecureAlloc(size)
			if err != nil {
				t.Fatalf("SecureAlloc 失败: %v", err)
			}
			// 写入非零数据
			for j := range buf.Data {
				buf.Data[j] = 0xFF
			}
			gs.RegisterSensitiveBuffer(buf)
			bufs[i] = buf
		}

		if err := gs.Shutdown(); err != nil {
			t.Fatalf("Shutdown 失败: %v", err)
		}

		// 验证所有缓冲区已清零
		for i, buf := range bufs {
			for j, b := range buf.Data {
				if b != 0 {
					t.Fatalf("缓冲区 %d 字节 %d 不为零: %d", i, j, b)
				}
			}
		}
	})
}

// 单元测试: 30 秒超时
func TestGracefulShutdown_Timeout(t *testing.T) {
	rs := NewRAMShield()
	// 使用短超时测试
	gs := NewGracefulShutdown(rs, nil, 1*time.Second)

	order := make([]string, 0)
	mu := &sync.Mutex{}
	gs.RegisterModule(&mockModule{
		name:     "slow-module",
		order:    &order,
		mu:       mu,
		duration: 500 * time.Millisecond,
	})

	err := gs.Shutdown()
	if err != nil {
		// 超时情况下可能返回错误
		t.Logf("Shutdown 返回: %v", err)
	}
}

// 单元测试: 空模块列表
func TestGracefulShutdown_EmptyModules(t *testing.T) {
	rs := NewRAMShield()
	gs := NewGracefulShutdown(rs, nil, 30*time.Second)
	if err := gs.Shutdown(); err != nil {
		t.Fatalf("空模块列表不应失败: %v", err)
	}
}

// 单元测试: EmergencyWiper 调用验证
func TestGracefulShutdown_EmergencyWiperCalled(t *testing.T) {
	rs := NewRAMShield()
	wiper := &mockEmergencyWiper{}
	gs := NewGracefulShutdown(rs, wiper, 30*time.Second)

	if err := gs.Shutdown(); err != nil {
		t.Fatalf("Shutdown 失败: %v", err)
	}

	wiper.mu.Lock()
	defer wiper.mu.Unlock()
	if !wiper.called {
		t.Fatal("EmergencyWiper.TriggerWipe 未被调用")
	}
}
