// Package mcc - 法定人数同步加固
// 解决分布式环境下的"脏状态"和"失步"问题
package mcc

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

// SyncPhase 同步阶段
type SyncPhase int

const (
	PhaseIdle       SyncPhase = 0
	PhasePreCommit  SyncPhase = 1
	PhaseCommit     SyncPhase = 2
	PhaseRollback   SyncPhase = 3
)

// NodeSyncState 节点同步状态
type NodeSyncState struct {
	NodeID        string    `json:"node_id"`
	Region        string    `json:"region"`
	CurrentDomain string    `json:"current_domain"`
	LastSyncTime  time.Time `json:"last_sync_time"`
	SyncOffset    int64     `json:"sync_offset_ms"` // 与主控的时间偏移
	IsReady       bool      `json:"is_ready"`       // PreCommit 就绪
	IsCommitted   bool      `json:"is_committed"`   // Commit 完成
	FailCount     int       `json:"fail_count"`
	NeedResync    bool      `json:"need_resync"`
}

// QuorumProposal 法定人数提案
type QuorumProposal struct {
	ID            string            `json:"id"`
	Type          string            `json:"type"` // gswitch, bdna_reset
	Payload       interface{}       `json:"payload"`
	EffectiveTime time.Time         `json:"effective_time"` // 延迟生效时间
	Phase         SyncPhase         `json:"phase"`
	Votes         map[string]bool   `json:"votes"`
	CreatedAt     time.Time         `json:"created_at"`
	CommittedAt   *time.Time        `json:"committed_at"`
}

// QuorumConfig 法定人数配置
type QuorumConfig struct {
	MinQuorum         int           // 最小法定人数 (N/2 + 1)
	PreCommitTimeout  time.Duration // PreCommit 超时
	CommitTimeout     time.Duration // Commit 超时
	ResyncInterval    time.Duration // 重同步间隔
	LatencyCompensation time.Duration // 延迟补偿
}

// QuorumSync 法定人数同步器
type QuorumSync struct {
	mu sync.RWMutex

	// 节点状态
	nodeID     string
	nodeStates map[string]*NodeSyncState

	// 当前提案
	activeProposal *QuorumProposal

	// 活跃域名集
	activeDomainSet []string

	// 配置
	config QuorumConfig

	// 回调
	onPreCommit  func(proposal *QuorumProposal) bool
	onCommit     func(proposal *QuorumProposal) error
	onResync     func(nodeID string, domains []string) error

	// 统计
	stats struct {
		ProposalsCreated   uint64
		ProposalsCommitted uint64
		ProposalsFailed    uint64
		ResyncsTriggered   uint64
		QuorumReached      uint64
	}

	// 控制
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewQuorumSync 创建法定人数同步器
func NewQuorumSync(nodeID string, totalNodes int) *QuorumSync {
	ctx, cancel := context.WithCancel(context.Background())

	return &QuorumSync{
		nodeID:          nodeID,
		nodeStates:      make(map[string]*NodeSyncState),
		activeDomainSet: make([]string, 0),
		config: QuorumConfig{
			MinQuorum:           totalNodes/2 + 1,
			PreCommitTimeout:    500 * time.Millisecond,
			CommitTimeout:       1 * time.Second,
			ResyncInterval:      30 * time.Second,
			LatencyCompensation: 50 * time.Millisecond,
		},
		ctx:    ctx,
		cancel: cancel,
	}
}

// SetCallbacks 设置回调
func (qs *QuorumSync) SetCallbacks(
	onPreCommit func(*QuorumProposal) bool,
	onCommit func(*QuorumProposal) error,
	onResync func(string, []string) error,
) {
	qs.mu.Lock()
	defer qs.mu.Unlock()
	qs.onPreCommit = onPreCommit
	qs.onCommit = onCommit
	qs.onResync = onResync
}

// Start 启动同步器
func (qs *QuorumSync) Start() {
	qs.wg.Add(2)
	go qs.resyncLoop()
	go qs.healthCheckLoop()

	log.Printf("🔒 法定人数同步器已启动 (quorum=%d)", qs.config.MinQuorum)
}

// Stop 停止同步器
func (qs *QuorumSync) Stop() {
	qs.cancel()
	qs.wg.Wait()
	log.Println("🛑 法定人数同步器已停止")
}

// RegisterNode 注册节点
func (qs *QuorumSync) RegisterNode(nodeID, region string) {
	qs.mu.Lock()
	defer qs.mu.Unlock()

	qs.nodeStates[nodeID] = &NodeSyncState{
		NodeID:       nodeID,
		Region:       region,
		LastSyncTime: time.Now(),
		IsReady:      true,
	}
}

// ProposeGSwitch 提议 G-Switch 切换
func (qs *QuorumSync) ProposeGSwitch(oldDomain, newDomain string) (*QuorumProposal, error) {
	qs.mu.Lock()

	if qs.activeProposal != nil && qs.activeProposal.Phase != PhaseIdle {
		qs.mu.Unlock()
		return nil, fmt.Errorf("已有活跃提案")
	}

	// 计算延迟生效时间
	effectiveTime := time.Now().Add(qs.config.LatencyCompensation)

	proposal := &QuorumProposal{
		ID:   fmt.Sprintf("GS-%d", time.Now().UnixNano()),
		Type: "gswitch",
		Payload: map[string]string{
			"old_domain": oldDomain,
			"new_domain": newDomain,
		},
		EffectiveTime: effectiveTime,
		Phase:         PhasePreCommit,
		Votes:         make(map[string]bool),
		CreatedAt:     time.Now(),
	}

	qs.activeProposal = proposal
	qs.stats.ProposalsCreated++
	qs.mu.Unlock()

	log.Printf("📋 创建提案: %s (effective=%v)", proposal.ID, effectiveTime)

	// 执行两阶段提交
	if err := qs.executePreCommit(proposal); err != nil {
		return proposal, err
	}

	if err := qs.executeCommit(proposal); err != nil {
		return proposal, err
	}

	return proposal, nil
}

// executePreCommit 执行 PreCommit 阶段
func (qs *QuorumSync) executePreCommit(proposal *QuorumProposal) error {
	qs.mu.Lock()
	nodes := make([]*NodeSyncState, 0, len(qs.nodeStates))
	for _, state := range qs.nodeStates {
		nodes = append(nodes, state)
	}
	callback := qs.onPreCommit
	qs.mu.Unlock()

	log.Printf("🔄 Phase 1: PreCommit (nodes=%d)", len(nodes))

	// 并行发送 PreCommit
	var wg sync.WaitGroup
	var mu sync.Mutex
	readyCount := 0

	for _, node := range nodes {
		wg.Add(1)
		go func(n *NodeSyncState) {
			defer wg.Done()

			// 模拟 PreCommit 请求
			ready := true
			if callback != nil {
				ready = callback(proposal)
			}

			mu.Lock()
			if ready {
				proposal.Votes[n.NodeID] = true
				n.IsReady = true
				readyCount++
			} else {
				proposal.Votes[n.NodeID] = false
				n.IsReady = false
			}
			mu.Unlock()
		}(node)
	}

	// 等待超时或全部响应
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(qs.config.PreCommitTimeout):
		log.Printf("⚠️  PreCommit 超时")
	}

	// 检查法定人数
	qs.mu.Lock()
	if readyCount < qs.config.MinQuorum {
		proposal.Phase = PhaseRollback
		qs.stats.ProposalsFailed++
		qs.mu.Unlock()
		return fmt.Errorf("未达到法定人数: %d/%d", readyCount, qs.config.MinQuorum)
	}
	qs.stats.QuorumReached++
	qs.mu.Unlock()

	log.Printf("✅ PreCommit 完成: %d/%d 就绪", readyCount, len(nodes))
	return nil
}

// executeCommit 执行 Commit 阶段
func (qs *QuorumSync) executeCommit(proposal *QuorumProposal) error {
	qs.mu.Lock()
	proposal.Phase = PhaseCommit
	nodes := make([]*NodeSyncState, 0)
	for _, state := range qs.nodeStates {
		if state.IsReady {
			nodes = append(nodes, state)
		}
	}
	callback := qs.onCommit
	effectiveTime := proposal.EffectiveTime
	qs.mu.Unlock()

	log.Printf("🔄 Phase 2: Commit (nodes=%d, effective=%v)", len(nodes), effectiveTime)

	// 等待到生效时间
	waitDuration := time.Until(effectiveTime)
	if waitDuration > 0 {
		time.Sleep(waitDuration)
	}

	// 并行发送 Commit
	var wg sync.WaitGroup
	var mu sync.Mutex
	committedCount := 0
	failedNodes := make([]string, 0)

	for _, node := range nodes {
		wg.Add(1)
		go func(n *NodeSyncState) {
			defer wg.Done()

			var err error
			if callback != nil {
				err = callback(proposal)
			}

			mu.Lock()
			if err == nil {
				n.IsCommitted = true
				n.LastSyncTime = time.Now()
				n.SyncOffset = time.Since(effectiveTime).Milliseconds()
				committedCount++
			} else {
				n.IsCommitted = false
				n.NeedResync = true
				n.FailCount++
				failedNodes = append(failedNodes, n.NodeID)
			}
			mu.Unlock()
		}(node)
	}

	// 等待超时或全部响应
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(qs.config.CommitTimeout):
		log.Printf("⚠️  Commit 超时")
	}

	qs.mu.Lock()
	now := time.Now()
	proposal.CommittedAt = &now
	proposal.Phase = PhaseIdle
	qs.stats.ProposalsCommitted++

	// 更新活跃域名集
	if payload, ok := proposal.Payload.(map[string]string); ok {
		if newDomain := payload["new_domain"]; newDomain != "" {
			qs.activeDomainSet = append(qs.activeDomainSet, newDomain)
		}
	}
	qs.mu.Unlock()

	log.Printf("✅ Commit 完成: %d 成功, %d 失败", committedCount, len(failedNodes))

	// 触发失败节点重同步
	if len(failedNodes) > 0 {
		go qs.triggerResync(failedNodes)
	}

	return nil
}

// triggerResync 触发重同步
func (qs *QuorumSync) triggerResync(nodeIDs []string) {
	qs.mu.Lock()
	domains := make([]string, len(qs.activeDomainSet))
	copy(domains, qs.activeDomainSet)
	callback := qs.onResync
	qs.mu.Unlock()

	for _, nodeID := range nodeIDs {
		log.Printf("🔄 强制重同步: %s", nodeID)
		qs.stats.ResyncsTriggered++

		if callback != nil {
			if err := callback(nodeID, domains); err != nil {
				log.Printf("⚠️  重同步失败: %s - %v", nodeID, err)
				continue
			}
		}

		// 更新节点状态
		qs.mu.Lock()
		if state, ok := qs.nodeStates[nodeID]; ok {
			state.NeedResync = false
			state.IsCommitted = true
			state.LastSyncTime = time.Now()
		}
		qs.mu.Unlock()

		log.Printf("✅ 重同步完成: %s", nodeID)
	}
}

// resyncLoop 重同步循环
func (qs *QuorumSync) resyncLoop() {
	defer qs.wg.Done()

	ticker := time.NewTicker(qs.config.ResyncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-qs.ctx.Done():
			return
		case <-ticker.C:
			qs.checkAndResync()
		}
	}
}

// checkAndResync 检查并重同步
func (qs *QuorumSync) checkAndResync() {
	qs.mu.RLock()
	needResync := make([]string, 0)
	for nodeID, state := range qs.nodeStates {
		if state.NeedResync || time.Since(state.LastSyncTime) > 5*time.Minute {
			needResync = append(needResync, nodeID)
		}
	}
	qs.mu.RUnlock()

	if len(needResync) > 0 {
		qs.triggerResync(needResync)
	}
}

// healthCheckLoop 健康检查循环
func (qs *QuorumSync) healthCheckLoop() {
	defer qs.wg.Done()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-qs.ctx.Done():
			return
		case <-ticker.C:
			qs.updateHealthStatus()
		}
	}
}

// updateHealthStatus 更新健康状态
func (qs *QuorumSync) updateHealthStatus() {
	qs.mu.Lock()
	defer qs.mu.Unlock()

	staleThreshold := 2 * time.Minute
	now := time.Now()

	for _, state := range qs.nodeStates {
		if now.Sub(state.LastSyncTime) > staleThreshold {
			state.NeedResync = true
		}
	}
}

// GetNodeStates 获取节点状态
func (qs *QuorumSync) GetNodeStates() map[string]*NodeSyncState {
	qs.mu.RLock()
	defer qs.mu.RUnlock()

	result := make(map[string]*NodeSyncState)
	for k, v := range qs.nodeStates {
		stateCopy := *v
		result[k] = &stateCopy
	}
	return result
}

// GetSyncStats 获取同步统计
func (qs *QuorumSync) GetSyncStats() map[string]uint64 {
	qs.mu.RLock()
	defer qs.mu.RUnlock()

	return map[string]uint64{
		"proposals_created":   qs.stats.ProposalsCreated,
		"proposals_committed": qs.stats.ProposalsCommitted,
		"proposals_failed":    qs.stats.ProposalsFailed,
		"resyncs_triggered":   qs.stats.ResyncsTriggered,
		"quorum_reached":      qs.stats.QuorumReached,
	}
}

// GetActiveDomainSet 获取活跃域名集
func (qs *QuorumSync) GetActiveDomainSet() []string {
	qs.mu.RLock()
	defer qs.mu.RUnlock()

	result := make([]string, len(qs.activeDomainSet))
	copy(result, qs.activeDomainSet)
	return result
}

// ForceResyncNode 强制重同步节点
func (qs *QuorumSync) ForceResyncNode(nodeID string) {
	qs.triggerResync([]string{nodeID})
}
