package gtclient

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

// ClientOrchestrator 是 Client 侧的多协议编排器。
// 与 Gateway 侧的 Orchestrator 不同，Client 侧是主动拨号方：
// 默认走 QUIC 主路径，QUIC 不可用时降级到 WSS，WSS 恢复后回升到 QUIC。
//
// 它实现 Transport 接口，可以直接注入 GTunnelClient.SetTransport()。
// 这是 Client 侧"唯一运行时主链"的实现（审计 spec A-01 / S-01）。
type ClientOrchestrator struct {
	mu         sync.RWMutex
	active     Transport // 当前活跃传输
	activeType string    // "quic" | "wss"
	quicDial   func(ctx context.Context) (Transport, error)
	wssDial    func(ctx context.Context) (Transport, error)
	probeStop  chan struct{}
	closed     bool

	// 配置
	probeInterval    time.Duration // QUIC 探测间隔（降级后）
	promoteThreshold int           // 连续成功次数后回升
	fallbackTimeout  time.Duration // QUIC 拨号超时（超时后降级）
}

// ClientOrchestratorConfig 编排器配置。
type ClientOrchestratorConfig struct {
	// QUICDial 拨号 QUIC 主路径。返回的 Transport 必须实现 SendDatagram/ReceiveDatagram。
	QUICDial func(ctx context.Context) (Transport, error)
	// WSSDial 拨号 WSS 降级路径（可选，nil 表示不启用降级）。
	WSSDial func(ctx context.Context) (Transport, error)
	// FallbackTimeout QUIC 拨号超时，超时后尝试 WSS 降级。默认 3s。
	FallbackTimeout time.Duration
	// ProbeInterval 降级后 QUIC 探测间隔。默认 30s。
	ProbeInterval time.Duration
	// PromoteThreshold 连续成功探测次数后回升。默认 3。
	PromoteThreshold int
}

// NewClientOrchestrator 创建 Client 侧编排器。
func NewClientOrchestrator(cfg ClientOrchestratorConfig) *ClientOrchestrator {
	if cfg.FallbackTimeout == 0 {
		cfg.FallbackTimeout = 3 * time.Second
	}
	if cfg.ProbeInterval == 0 {
		cfg.ProbeInterval = 30 * time.Second
	}
	if cfg.PromoteThreshold == 0 {
		cfg.PromoteThreshold = 3
	}
	return &ClientOrchestrator{
		quicDial:         cfg.QUICDial,
		wssDial:          cfg.WSSDial,
		probeStop:        make(chan struct{}),
		probeInterval:    cfg.ProbeInterval,
		promoteThreshold: cfg.PromoteThreshold,
		fallbackTimeout:  cfg.FallbackTimeout,
	}
}

// Connect 建立连接：先尝试 QUIC，超时后降级到 WSS。
// 降级后后台持续探测 QUIC，恢复后自动回升。
func (co *ClientOrchestrator) Connect(ctx context.Context) error {
	// 尝试 QUIC
	quicCtx, quicCancel := context.WithTimeout(ctx, co.fallbackTimeout)
	defer quicCancel()

	if co.quicDial != nil {
		transport, err := co.quicDial(quicCtx)
		if err == nil {
			co.mu.Lock()
			co.active = transport
			co.activeType = "quic"
			co.mu.Unlock()
			log.Println("🚀 [ClientOrchestrator] QUIC 主路径已建立")
			return nil
		}
		log.Printf("⚠️ [ClientOrchestrator] QUIC 拨号失败: %v", err)
	}

	// QUIC 失败，尝试 WSS 降级
	if co.wssDial != nil {
		transport, err := co.wssDial(ctx)
		if err == nil {
			co.mu.Lock()
			co.active = transport
			co.activeType = "wss"
			co.mu.Unlock()
			log.Println("🦎 [ClientOrchestrator] WSS 降级路径已建立，启动 QUIC 探测...")
			go co.probeAndPromote(ctx)
			return nil
		}
		log.Printf("⚠️ [ClientOrchestrator] WSS 降级也失败: %v", err)
	}

	return fmt.Errorf("all transports failed")
}

// probeAndPromote 降级后后台探测 QUIC，恢复后回升。
func (co *ClientOrchestrator) probeAndPromote(ctx context.Context) {
	ticker := time.NewTicker(co.probeInterval)
	defer ticker.Stop()

	successCount := 0

	for {
		select {
		case <-co.probeStop:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		co.mu.RLock()
		if co.activeType == "quic" || co.closed {
			co.mu.RUnlock()
			return // 已经回升或已关闭
		}
		co.mu.RUnlock()

		if co.quicDial == nil {
			return
		}

		probeCtx, cancel := context.WithTimeout(ctx, co.fallbackTimeout)
		transport, err := co.quicDial(probeCtx)
		cancel()

		if err != nil {
			successCount = 0
			continue
		}

		successCount++
		log.Printf("🔍 [ClientOrchestrator] QUIC 探测成功 (%d/%d)", successCount, co.promoteThreshold)

		if successCount >= co.promoteThreshold {
			co.mu.Lock()
			old := co.active
			co.active = transport
			co.activeType = "quic"
			co.mu.Unlock()

			if old != nil {
				old.Close()
			}
			log.Println("⬆️ [ClientOrchestrator] 已回升到 QUIC 主路径")
			return
		}

		// 探测成功但未达阈值，关闭探测连接
		transport.Close()
	}
}

// --- Transport 接口实现 ---

// SendDatagram 通过当前活跃传输发送数据。
func (co *ClientOrchestrator) SendDatagram(data []byte) error {
	co.mu.RLock()
	t := co.active
	co.mu.RUnlock()
	if t == nil {
		return fmt.Errorf("not connected")
	}
	return t.SendDatagram(data)
}

// ReceiveDatagram 从当前活跃传输接收数据。
func (co *ClientOrchestrator) ReceiveDatagram(ctx context.Context) ([]byte, error) {
	co.mu.RLock()
	t := co.active
	co.mu.RUnlock()
	if t == nil {
		return nil, fmt.Errorf("not connected")
	}
	return t.ReceiveDatagram(ctx)
}

// IsConnected 返回当前是否有活跃连接。
func (co *ClientOrchestrator) IsConnected() bool {
	co.mu.RLock()
	defer co.mu.RUnlock()
	return co.active != nil && co.active.IsConnected()
}

// Close 关闭编排器和所有连接。
func (co *ClientOrchestrator) Close() error {
	co.mu.Lock()
	defer co.mu.Unlock()
	co.closed = true
	close(co.probeStop)
	if co.active != nil {
		return co.active.Close()
	}
	return nil
}

// ActiveType 返回当前活跃传输类型（"quic" / "wss" / ""）。
func (co *ClientOrchestrator) ActiveType() string {
	co.mu.RLock()
	defer co.mu.RUnlock()
	return co.activeType
}
