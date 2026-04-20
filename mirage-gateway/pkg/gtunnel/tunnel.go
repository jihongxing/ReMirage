// Package gtunnel - G-Tunnel 主控制器
package gtunnel

import (
	"context"
	"log"
	"net"
	"sync"
)

// Tunnel G-Tunnel 隧道
type Tunnel struct {
	scheduler    *PathScheduler
	orchestrator *Orchestrator
	fec          *FECProcessor
	packetID     uint64
	mu           sync.Mutex
	sendBuffer   chan []byte
	recvBuffer   map[uint64][]*Shard
	recvMu       sync.RWMutex
}

// NewTunnel 创建隧道
func NewTunnel(strategy string) *Tunnel {
	return &Tunnel{
		scheduler:  NewPathScheduler(strategy),
		fec:        NewFECProcessor(),
		sendBuffer: make(chan []byte, 256),
		recvBuffer: make(map[uint64][]*Shard),
	}
}

// NewTunnelWithOrchestrator 创建带 Orchestrator 的隧道
func NewTunnelWithOrchestrator(config OrchestratorConfig) *Tunnel {
	orch := NewOrchestrator(config)
	return &Tunnel{
		scheduler:    NewPathScheduler("lowest-rtt"),
		orchestrator: orch,
		fec:          orch.fec,
		sendBuffer:   make(chan []byte, 256),
		recvBuffer:   make(map[uint64][]*Shard),
	}
}

// GetOrchestrator 获取 Orchestrator 引用
func (t *Tunnel) GetOrchestrator() *Orchestrator {
	return t.orchestrator
}

// AddPath 添加传输路径
func (t *Tunnel) AddPath(cellID, iface string, remoteAddr, localAddr *net.UDPAddr) error {
	path := &Path{
		ID:         cellID + "-" + iface,
		CellID:     cellID,
		Interface:  iface,
		RemoteAddr: remoteAddr,
		LocalAddr:  localAddr,
	}

	return t.scheduler.AddPath(path)
}

// Send 发送数据
func (t *Tunnel) Send(data []byte) error {
	// 如果有 Orchestrator，通过 Orchestrator 发送
	if t.orchestrator != nil {
		return t.orchestrator.Send(data)
	}

	t.mu.Lock()
	packetID := t.packetID
	t.packetID++
	t.mu.Unlock()

	// FEC 编码
	shards, err := t.fec.EncodePacket(data)
	if err != nil {
		return err
	}

	// 多路径发送
	for _, shard := range shards {
		serialized := SerializeShard(shard, packetID)
		if err := t.scheduler.SendShard(serialized); err != nil {
			log.Printf("⚠️  [G-Tunnel] 发送分片失败: %v", err)
		}
	}

	return nil
}

// Start 启动隧道
func (t *Tunnel) Start() {
	if t.orchestrator != nil {
		if err := t.orchestrator.Start(context.Background()); err != nil {
			log.Printf("⚠️  [G-Tunnel] Orchestrator 启动失败: %v，回退到 PathScheduler", err)
		} else {
			log.Println("🚀 [G-Tunnel] 隧道已启动（Orchestrator 模式）")
			return
		}
	}

	go t.scheduler.MonitorPaths()
	log.Println("🚀 [G-Tunnel] 隧道已启动（PathScheduler 模式）")
}
