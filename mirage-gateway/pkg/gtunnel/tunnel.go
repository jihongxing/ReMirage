// Package gtunnel - G-Tunnel 主控制器
package gtunnel

import (
	"log"
	"net"
	"sync"
)

// Tunnel G-Tunnel 隧道
type Tunnel struct {
	scheduler    *PathScheduler
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
	go t.scheduler.MonitorPaths()
	log.Println("🚀 [G-Tunnel] 隧道已启动")
}
