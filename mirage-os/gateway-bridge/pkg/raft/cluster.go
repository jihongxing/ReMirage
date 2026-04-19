package raft

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/hashicorp/raft"
	raftboltdb "github.com/hashicorp/raft-boltdb/v2"

	"mirage-os/gateway-bridge/pkg/config"
)

// Cluster Raft 集群管理器
type Cluster struct {
	raft      *raft.Raft
	fsm       *FSM
	config    config.RaftConfig
	transport raft.Transport
	logStore  raft.LogStore
	stable    raft.StableStore
	snapStore raft.SnapshotStore
}

// NewCluster 创建 Raft 集群
func NewCluster(cfg config.RaftConfig) (*Cluster, error) {
	if err := os.MkdirAll(cfg.DataDir, 0o750); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	// BoltDB LogStore + StableStore
	boltPath := filepath.Join(cfg.DataDir, "raft.db")
	boltStore, err := raftboltdb.NewBoltStore(boltPath)
	if err != nil {
		return nil, fmt.Errorf("new bolt store: %w", err)
	}

	// FileSnapshotStore（保留 3 个快照）
	snapStore, err := raft.NewFileSnapshotStore(cfg.DataDir, 3, os.Stderr)
	if err != nil {
		return nil, fmt.Errorf("new snapshot store: %w", err)
	}

	// TCP Transport
	addr, err := net.ResolveTCPAddr("tcp", cfg.BindAddr)
	if err != nil {
		return nil, fmt.Errorf("resolve bind addr: %w", err)
	}
	transport, err := raft.NewTCPTransport(cfg.BindAddr, addr, 3, 10*time.Second, os.Stderr)
	if err != nil {
		return nil, fmt.Errorf("new tcp transport: %w", err)
	}

	// FSM
	fsm := NewFSM()

	// Raft 配置
	raftCfg := raft.DefaultConfig()
	raftCfg.LocalID = raft.ServerID(cfg.NodeID)
	raftCfg.SnapshotThreshold = 1024
	raftCfg.SnapshotInterval = 30 * time.Second

	r, err := raft.NewRaft(raftCfg, fsm, boltStore, boltStore, snapStore, transport)
	if err != nil {
		return nil, fmt.Errorf("new raft: %w", err)
	}

	return &Cluster{
		raft:      r,
		fsm:       fsm,
		config:    cfg,
		transport: transport,
		logStore:  boltStore,
		stable:    boltStore,
		snapStore: snapStore,
	}, nil
}

// Start 启动集群
func (c *Cluster) Start() error {
	if !c.config.Bootstrap {
		log.Println("[INFO] raft: non-bootstrap node, waiting for leader")
		return nil
	}

	// 检查是否已有日志（已初始化过）
	hasState, err := raft.HasExistingState(c.logStore, c.stable, c.snapStore)
	if err != nil {
		return fmt.Errorf("check existing state: %w", err)
	}
	if hasState {
		log.Println("[INFO] raft: existing state found, skipping bootstrap")
		return nil
	}

	// 构建集群配置
	var servers []raft.Server
	for _, p := range c.config.Peers {
		suffrage := raft.Voter
		if !p.Voter {
			suffrage = raft.Nonvoter
		}
		servers = append(servers, raft.Server{
			ID:       raft.ServerID(p.ID),
			Address:  raft.ServerAddress(p.Address),
			Suffrage: suffrage,
		})
	}

	future := c.raft.BootstrapCluster(raft.Configuration{Servers: servers})
	if err := future.Error(); err != nil {
		return fmt.Errorf("bootstrap cluster: %w", err)
	}
	log.Println("[INFO] raft: cluster bootstrapped")
	return nil
}

// IsLeader 返回当前节点是否为 Leader
func (c *Cluster) IsLeader() bool {
	return c.raft.State() == raft.Leader
}

// Apply 提交状态变更到 Raft 日志
func (c *Cluster) Apply(cmd FSMCommand) error {
	data, err := json.Marshal(cmd)
	if err != nil {
		return fmt.Errorf("marshal command: %w", err)
	}
	future := c.raft.Apply(data, 10*time.Second)
	if err := future.Error(); err != nil {
		return fmt.Errorf("raft apply: %w", err)
	}
	if resp := future.Response(); resp != nil {
		if e, ok := resp.(error); ok {
			return e
		}
	}
	return nil
}

// GetLeaderAddr 返回当前 Leader 地址
func (c *Cluster) GetLeaderAddr() string {
	addr, _ := c.raft.LeaderWithID()
	return string(addr)
}

// GetFSM 返回 FSM 实例
func (c *Cluster) GetFSM() *FSM {
	return c.fsm
}

// Shutdown 优雅关闭
func (c *Cluster) Shutdown() error {
	future := c.raft.Shutdown()
	return future.Error()
}
