// Package dataplane 定义 Gateway 侧数据面注入接口。
// 隧道解封后的完整 IP 包需要注入本机网络栈（TUN 设备或 NFQUEUE），
// 而非送入 TPROXY（TCP 流代理，语义不匹配）。
package dataplane

import (
	"fmt"
	"log"
	"sync/atomic"
)

// Injector 数据面注入器接口。
// Orchestrator 收到解隧后的 IP 包后，通过此接口注入本机网络栈。
type Injector interface {
	// InjectIPPacket 将完整 IP 包写入数据面设备（TUN fd / NFQUEUE）。
	InjectIPPacket(pkt []byte) error
	// Close 释放设备资源。
	Close() error
}

// --- NoopInjector: 设备未就绪时的降级占位 ---

// NoopInjector 在 TUN/NFQUEUE 未接入时使用。
// 不丢弃数据，而是记录结构化告警并计数，供监控采集。
type NoopInjector struct {
	dropped uint64
}

// NewNoopInjector 创建降级注入器。
func NewNoopInjector() *NoopInjector {
	return &NoopInjector{}
}

// InjectIPPacket 记录告警，返回错误（调用方可据此判断数据面未就绪）。
func (n *NoopInjector) InjectIPPacket(pkt []byte) error {
	cnt := atomic.AddUint64(&n.dropped, 1)
	// 每 100 包打一次日志，避免刷屏
	if cnt == 1 || cnt%100 == 0 {
		log.Printf("⚠️ [DataPlane] TUN/NFQUEUE 未接入，IP 包被丢弃: %d bytes (累计丢弃 %d 包)", len(pkt), cnt)
	}
	return fmt.Errorf("dataplane: no injector available (TUN/NFQUEUE not attached)")
}

// Close 无操作。
func (n *NoopInjector) Close() error { return nil }

// Dropped 返回累计丢弃包数。
func (n *NoopInjector) Dropped() uint64 {
	return atomic.LoadUint64(&n.dropped)
}
