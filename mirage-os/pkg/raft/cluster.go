// Package raft - Raft 集群管理
package raft

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/hashicorp/raft"
	raftboltdb "github.com/hashicorp/raft-boltdb"

	"mirage-os/pkg/crypto"
)

// Jurisdiction 司法管辖区
type Jurisdiction string

const (
	JurisdictionIceland     Jurisdiction = "IS" // 冰岛
	JurisdictionSwitzerland Jurisdiction = "CH" // 瑞士
	JurisdictionSingapore   Jurisdiction = "SG" // 新加坡
	JurisdictionPanama      Jurisdiction = "PA" // 巴拿马
	JurisdictionSeychelles  Jurisdiction = "SC" // 塞舌尔
)

// ClusterConfig Raft 集群配置
type ClusterConfig struct {
	NodeID       string
	BindAddr     string
	DataDir      string
	Jurisdiction Jurisdiction
	Peers        []string
}

// Cluster Raft 集群
type Cluster struct {
	config      *ClusterConfig
	raft        *raft.Raft
	fsm         *FSM
	transport   *raft.NetworkTransport
	threatLevel int
	ctx         context.Context
	cancel      context.CancelFunc
	hotKeyMgr   *HotKeyManager        // 热密钥管理器
	backupMgr   *crypto.BackupManager // 备份管理器
}

// NewCluster 创建 Raft 集群
func NewCluster(config *ClusterConfig) (*Cluster, error) {
	ctx, cancel := context.WithCancel(context.Background())

	// 创建备份管理器（3-of-5 Shamir）
	backupMgr, err := crypto.NewBackupManager(3, 5)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("创建备份管理器失败: %w", err)
	}

	cluster := &Cluster{
		config:      config,
		threatLevel: 0,
		ctx:         ctx,
		cancel:      cancel,
		backupMgr:   backupMgr,
	}

	// 创建 FSM
	cluster.fsm = NewFSM()

	// 创建热密钥管理器
	cluster.hotKeyMgr = NewHotKeyManager(backupMgr, cluster)

	return cluster, nil
}

// Start 启动 Raft 集群
func (c *Cluster) Start() error {
	log.Printf("[Raft] 启动节点: %s (司法管辖区: %s)", c.config.NodeID, c.config.Jurisdiction)

	// 1. 创建 Raft 配置
	raftConfig := raft.DefaultConfig()
	raftConfig.LocalID = raft.ServerID(c.config.NodeID)
	raftConfig.HeartbeatTimeout = 1000 * time.Millisecond
	raftConfig.ElectionTimeout = 1000 * time.Millisecond
	raftConfig.CommitTimeout = 500 * time.Millisecond
	raftConfig.LeaderLeaseTimeout = 500 * time.Millisecond

	// 2. 创建传输层
	addr, err := net.ResolveTCPAddr("tcp", c.config.BindAddr)
	if err != nil {
		return fmt.Errorf("解析地址失败: %w", err)
	}

	transport, err := raft.NewTCPTransport(c.config.BindAddr, addr, 3, 10*time.Second, os.Stderr)
	if err != nil {
		return fmt.Errorf("创建传输层失败: %w", err)
	}
	c.transport = transport

	// 3. 创建快照存储
	snapshotStore, err := raft.NewFileSnapshotStore(c.config.DataDir, 2, os.Stderr)
	if err != nil {
		return fmt.Errorf("创建快照存储失败: %w", err)
	}

	// 4. 创建日志存储
	logStore, err := raftboltdb.NewBoltStore(filepath.Join(c.config.DataDir, "raft-log.db"))
	if err != nil {
		return fmt.Errorf("创建日志存储失败: %w", err)
	}

	// 5. 创建稳定存储
	stableStore, err := raftboltdb.NewBoltStore(filepath.Join(c.config.DataDir, "raft-stable.db"))
	if err != nil {
		return fmt.Errorf("创建稳定存储失败: %w", err)
	}

	// 6. 创建 Raft 实例
	r, err := raft.NewRaft(raftConfig, c.fsm, logStore, stableStore, snapshotStore, transport)
	if err != nil {
		return fmt.Errorf("创建 Raft 实例失败: %w", err)
	}
	c.raft = r

	// 7. 引导集群（仅首次启动）
	if len(c.config.Peers) > 0 {
		configuration := raft.Configuration{
			Servers: []raft.Server{
				{
					ID:      raft.ServerID(c.config.NodeID),
					Address: raft.ServerAddress(c.config.BindAddr),
				},
			},
		}

		// 添加 Peer 节点
		for _, peer := range c.config.Peers {
			configuration.Servers = append(configuration.Servers, raft.Server{
				ID:      raft.ServerID(peer),
				Address: raft.ServerAddress(peer),
			})
		}

		f := r.BootstrapCluster(configuration)
		if err := f.Error(); err != nil {
			log.Printf("[Raft] ⚠️ 引导集群失败（可能已引导）: %v", err)
		}
	}

	// 8. 启动热密钥管理器
	if err := c.hotKeyMgr.Start(); err != nil {
		return fmt.Errorf("启动热密钥管理器失败: %w", err)
	}

	// 9. 启动威胁监控
	go c.monitorThreat()

	log.Printf("[Raft] ✅ 节点已启动")

	return nil
}

// Stop 停止 Raft 集群
func (c *Cluster) Stop() error {
	log.Println("[Raft] 停止节点")

	c.cancel()

	// 停止热密钥管理器
	if c.hotKeyMgr != nil {
		c.hotKeyMgr.Stop()
	}

	if c.raft != nil {
		if err := c.raft.Shutdown().Error(); err != nil {
			return fmt.Errorf("关闭 Raft 失败: %w", err)
		}
	}

	if c.transport != nil {
		if err := c.transport.Close(); err != nil {
			return fmt.Errorf("关闭传输层失败: %w", err)
		}
	}

	return nil
}

// IsLeader 是否为 Leader
func (c *Cluster) IsLeader() bool {
	if c.raft == nil {
		return false
	}
	return c.raft.State() == raft.Leader
}

// GetLeader 获取 Leader 地址
func (c *Cluster) GetLeader() string {
	if c.raft == nil {
		return ""
	}
	addr, _ := c.raft.LeaderWithID()
	return string(addr)
}

// Apply 应用命令到 Raft 日志
func (c *Cluster) Apply(cmd []byte, timeout time.Duration) error {
	if c.raft == nil {
		return fmt.Errorf("Raft 未初始化")
	}

	if !c.IsLeader() {
		return fmt.Errorf("当前节点不是 Leader")
	}

	f := c.raft.Apply(cmd, timeout)
	if err := f.Error(); err != nil {
		return fmt.Errorf("应用命令失败: %w", err)
	}

	return nil
}

// StepDown 主动退位（威胁逃逸）
func (c *Cluster) StepDown() error {
	if c.raft == nil {
		return fmt.Errorf("Raft 未初始化")
	}

	if !c.IsLeader() {
		return fmt.Errorf("当前节点不是 Leader，无需退位")
	}

	log.Printf("[Raft] 🚨 检测到威胁，主动退位 (司法管辖区: %s)", c.config.Jurisdiction)

	if err := c.raft.LeadershipTransfer().Error(); err != nil {
		return fmt.Errorf("退位失败: %w", err)
	}

	log.Println("[Raft] ✅ 已退位，等待其他节点接管")

	return nil
}

// monitorThreat 监控威胁等级
func (c *Cluster) monitorThreat() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			c.checkThreatLevel()
		}
	}
}

// checkThreatLevel 检查威胁等级
// 仅 ControlPlane 级威胁（政府审计/物理入侵/路由异常）触发 Raft 退位
// Gateway 级威胁（DDoS/SYN Flood/异常流量）通过 CellScheduler 处理，不影响 Raft 集群
func (c *Cluster) checkThreatLevel() {
	if ShouldStepDown(c.threatLevel, c.IsLeader()) {
		log.Printf("[Raft] ⚠️ 控制面威胁等级过高 (%d)，触发退位", c.threatLevel)
		if err := c.StepDown(); err != nil {
			log.Printf("[Raft] ❌ 退位失败: %v", err)
		}
	}
}

// SetThreatLevel 设置威胁等级
func (c *Cluster) SetThreatLevel(level int) {
	c.threatLevel = level
	log.Printf("[Raft] 威胁等级更新: %d", level)
}

// GetStats 获取集群统计
func (c *Cluster) GetStats() map[string]interface{} {
	if c.raft == nil {
		return nil
	}

	stats := map[string]interface{}{
		"node_id":      c.config.NodeID,
		"jurisdiction": c.config.Jurisdiction,
		"state":        c.raft.State().String(),
		"leader":       c.GetLeader(),
		"is_leader":    c.IsLeader(),
		"threat_level": c.threatLevel,
	}

	// 添加热密钥统计
	if c.hotKeyMgr != nil {
		hotKeyStats := c.hotKeyMgr.GetStats()
		for k, v := range hotKeyStats {
			stats["hot_key_"+k] = v
		}
	}

	return stats
}

// AddPeer 添加 Peer 节点
func (c *Cluster) AddPeer(nodeID, addr string) error {
	if c.raft == nil {
		return fmt.Errorf("Raft 未初始化")
	}

	if !c.IsLeader() {
		return fmt.Errorf("只有 Leader 可以添加节点")
	}

	f := c.raft.AddVoter(raft.ServerID(nodeID), raft.ServerAddress(addr), 0, 0)
	if err := f.Error(); err != nil {
		return fmt.Errorf("添加节点失败: %w", err)
	}

	log.Printf("[Raft] ✅ 已添加节点: %s (%s)", nodeID, addr)

	return nil
}

// RemovePeer 移除 Peer 节点
func (c *Cluster) RemovePeer(nodeID string) error {
	if c.raft == nil {
		return fmt.Errorf("Raft 未初始化")
	}

	if !c.IsLeader() {
		return fmt.Errorf("只有 Leader 可以移除节点")
	}

	f := c.raft.RemoveServer(raft.ServerID(nodeID), 0, 0)
	if err := f.Error(); err != nil {
		return fmt.Errorf("移除节点失败: %w", err)
	}

	log.Printf("[Raft] ✅ 已移除节点: %s", nodeID)

	return nil
}

// GetMasterKey 获取主密钥（用于 G-Switch 加密）
func (c *Cluster) GetMasterKey() ([]byte, error) {
	if c.hotKeyMgr == nil {
		return nil, fmt.Errorf("热密钥管理器未初始化")
	}

	return c.hotKeyMgr.GetMasterKey()
}

// IsHotKeyActive 检查热密钥是否激活
func (c *Cluster) IsHotKeyActive() bool {
	if c.hotKeyMgr == nil {
		return false
	}

	return c.hotKeyMgr.IsHotKeyActive()
}

// GetBackupManager 获取备份管理器
func (c *Cluster) GetBackupManager() *crypto.BackupManager {
	return c.backupMgr
}

// GetHotKeyManager 获取热密钥管理器
func (c *Cluster) GetHotKeyManager() *HotKeyManager {
	return c.hotKeyMgr
}
