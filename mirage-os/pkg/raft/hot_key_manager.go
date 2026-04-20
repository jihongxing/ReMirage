// Package raft - 热密钥管理器
package raft

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"mirage-os/pkg/crypto"
)

// ShareProvider 份额提供者接口
type ShareProvider interface {
	RequestShare(ctx context.Context, nodeAddr string) (*crypto.Share, error)
	GetOnlineNodes() ([]string, error)
}

// RaftShareProvider 基于 Raft 集群的份额提供者
type RaftShareProvider struct {
	cluster *Cluster
}

// NewRaftShareProvider 创建 Raft 份额提供者
func NewRaftShareProvider(cluster *Cluster) *RaftShareProvider {
	return &RaftShareProvider{cluster: cluster}
}

// RequestShare 从节点请求份额
func (rsp *RaftShareProvider) RequestShare(ctx context.Context, nodeAddr string) (*crypto.Share, error) {
	// 通过 Raft Transport 请求份额
	// 实际实现会通过 gRPC 或 Raft 自定义 RPC
	return nil, fmt.Errorf("节点 %s 份额请求未实现", nodeAddr)
}

// GetOnlineNodes 获取在线节点列表
func (rsp *RaftShareProvider) GetOnlineNodes() ([]string, error) {
	if rsp.cluster == nil || rsp.cluster.raft == nil {
		return nil, fmt.Errorf("Raft 集群未初始化")
	}
	future := rsp.cluster.raft.GetConfiguration()
	if err := future.Error(); err != nil {
		return nil, fmt.Errorf("获取集群配置失败: %w", err)
	}
	var nodes []string
	for _, server := range future.Configuration().Servers {
		nodes = append(nodes, string(server.Address))
	}
	return nodes, nil
}

// ValidateShare 验证份额有效性（导出函数，用于属性测试）
func ValidateShare(share *crypto.Share) bool {
	if share == nil {
		return false
	}
	if share.Index < 1 || share.Index > 5 {
		return false
	}
	if len(share.Value) != 32 {
		return false
	}
	return true
}

// KeyRotationCommand Raft 日志中的密钥轮换命令
type KeyRotationCommand struct {
	Type            string            `json:"type"`             // "key_rotation"
	EncryptedShares map[string][]byte `json:"encrypted_shares"` // nodeID -> 加密份额
	Epoch           uint64            `json:"epoch"`            // 轮换纪元号
}

// HotKeyManager 热密钥管理器
type HotKeyManager struct {
	mu            sync.RWMutex
	backupMgr     *crypto.BackupManager
	cluster       *Cluster
	shareProvider ShareProvider
	refreshTicker *time.Ticker
	ctx           context.Context
	cancel        context.CancelFunc
	epoch         uint64
}

// NewHotKeyManager 创建热密钥管理器
func NewHotKeyManager(backupMgr *crypto.BackupManager, cluster *Cluster) *HotKeyManager {
	ctx, cancel := context.WithCancel(context.Background())
	hkm := &HotKeyManager{
		backupMgr: backupMgr,
		cluster:   cluster,
		ctx:       ctx,
		cancel:    cancel,
	}
	hkm.shareProvider = NewRaftShareProvider(cluster)
	return hkm
}

// SetShareProvider 注入 ShareProvider（用于测试）
func (hkm *HotKeyManager) SetShareProvider(sp ShareProvider) {
	hkm.shareProvider = sp
}

// Start 启动热密钥管理器
func (hkm *HotKeyManager) Start() error {
	log.Println("[HotKey] 启动热密钥管理器")

	if !hkm.backupMgr.IsHotKeyActive() {
		log.Println("[HotKey] ⚠️ 热密钥未激活，需要冷启动")
		return hkm.ColdStart()
	}

	log.Println("[HotKey] ✅ 热密钥已激活，零延迟模式")
	return nil
}

// Stop 停止热密钥管理器
func (hkm *HotKeyManager) Stop() {
	log.Println("[HotKey] 停止热密钥管理器")
	if hkm.refreshTicker != nil {
		hkm.refreshTicker.Stop()
	}
	hkm.cancel()
}

// ColdStart 冷启动（从 Shamir 份额恢复）
func (hkm *HotKeyManager) ColdStart() error {
	log.Println("[HotKey] 🥶 执行冷启动（Shamir 恢复）")
	startTime := time.Now()

	shares, err := hkm.collectShares()
	if err != nil {
		return err
	}

	if err := hkm.backupMgr.ActivateHotKey(shares); err != nil {
		return err
	}

	elapsed := time.Since(startTime)
	log.Printf("[HotKey] ✅ 冷启动完成，耗时: %v", elapsed)
	if elapsed > 5*time.Second {
		log.Printf("[HotKey] ⚠️ 冷启动耗时超过 5 秒")
	}
	return nil
}

// collectShares 从 Raft 集群收集份额
func (hkm *HotKeyManager) collectShares() ([]crypto.Share, error) {
	log.Println("[HotKey] 从 Raft 集群收集 Shamir 份额")

	nodes, err := hkm.shareProvider.GetOnlineNodes()
	if err != nil {
		return nil, fmt.Errorf("获取在线节点失败: %w", err)
	}

	type shareResult struct {
		share *crypto.Share
		err   error
	}

	results := make(chan shareResult, len(nodes))
	var wg sync.WaitGroup

	for _, node := range nodes {
		wg.Add(1)
		go func(addr string) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(hkm.ctx, 5*time.Second)
			defer cancel()

			share, err := hkm.shareProvider.RequestShare(ctx, addr)
			results <- shareResult{share: share, err: err}
		}(node)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	var validShares []crypto.Share
	for r := range results {
		if r.err != nil {
			log.Printf("[HotKey] 节点份额请求失败: %v", r.err)
			continue
		}
		if r.share != nil && ValidateShare(r.share) {
			validShares = append(validShares, *r.share)
		}
	}

	if len(validShares) < 3 {
		return nil, fmt.Errorf("有效份额不足: 需要 3 个，仅收集到 %d 个", len(validShares))
	}

	log.Printf("[HotKey] ✅ 已收集 %d 个有效份额", len(validShares))
	return validShares, nil
}

// RefreshHotKey 刷新热密钥（Raft Log 强一致性分发）
func (hkm *HotKeyManager) RefreshHotKey() error {
	hkm.mu.Lock()
	defer hkm.mu.Unlock()

	log.Println("[HotKey] 🔄 刷新热密钥")

	// 1. 备份旧份额引用（用于失败恢复）
	oldShares, _ := hkm.collectSharesForBackup()

	// 2. 生成新 32 字节 AES-256 主密钥
	newKey := make([]byte, 32)
	if _, err := rand.Read(newKey); err != nil {
		return fmt.Errorf("生成新密钥失败: %w", err)
	}

	// 3. SplitSecret 3-of-5
	shares, err := crypto.SplitSecret(newKey, crypto.ShamirConfig{Threshold: 3, Shares: 5})
	if err != nil {
		return fmt.Errorf("分割密钥失败: %w", err)
	}

	// 4. 构建 KeyRotationCommand（实际应用中会用节点公钥加密）
	hkm.epoch++
	encryptedShares := make(map[string][]byte)
	nodes, err := hkm.shareProvider.GetOnlineNodes()
	if err != nil {
		log.Printf("[HotKey] 获取节点列表失败，使用默认分配: %v", err)
		for i, s := range shares {
			encryptedShares[fmt.Sprintf("node-%d", i)] = s.Value
		}
	} else {
		for i, s := range shares {
			if i < len(nodes) {
				encryptedShares[nodes[i]] = s.Value
			}
		}
	}

	cmd := KeyRotationCommand{
		Type:            "key_rotation",
		EncryptedShares: encryptedShares,
		Epoch:           hkm.epoch,
	}

	cmdBytes, err := json.Marshal(Command{
		Type:    CommandType("key_rotation"),
		Payload: mustMarshal(cmd),
	})
	if err != nil {
		return fmt.Errorf("序列化命令失败: %w", err)
	}

	// 5. 通过 raft.Apply 提交到 Raft 日志
	if err := hkm.cluster.Apply(cmdBytes, 10*time.Second); err != nil {
		// 失败：保持旧密钥不变
		log.Printf("[HotKey] ❌ Raft Apply 失败，保持旧密钥: %v", err)
		if len(oldShares) >= 3 {
			if recoverErr := hkm.backupMgr.ActivateHotKey(oldShares); recoverErr != nil {
				log.Printf("[HotKey] ❌ CRITICAL: 恢复旧密钥也失败: %v", recoverErr)
			}
		}
		return fmt.Errorf("密钥轮换失败: %w", err)
	}

	// 6. 成功：激活新密钥
	hkm.backupMgr.DeactivateHotKey()
	if err := hkm.backupMgr.ActivateHotKey(shares); err != nil {
		log.Printf("[HotKey] ❌ 激活新密钥失败: %v", err)
		return err
	}

	log.Println("[HotKey] ✅ 热密钥已刷新")
	return nil
}

// collectSharesForBackup 收集当前份额用于备份
func (hkm *HotKeyManager) collectSharesForBackup() ([]crypto.Share, error) {
	// 尝试从当前状态获取份额，失败则返回空
	nodes, err := hkm.shareProvider.GetOnlineNodes()
	if err != nil {
		return nil, err
	}

	var shares []crypto.Share
	for _, node := range nodes {
		ctx, cancel := context.WithTimeout(hkm.ctx, 2*time.Second)
		share, err := hkm.shareProvider.RequestShare(ctx, node)
		cancel()
		if err == nil && share != nil && ValidateShare(share) {
			shares = append(shares, *share)
		}
	}
	return shares, nil
}

// GetMasterKey 获取主密钥
func (hkm *HotKeyManager) GetMasterKey() ([]byte, error) {
	hkm.mu.RLock()
	defer hkm.mu.RUnlock()
	return hkm.backupMgr.GetMasterKey()
}

// IsHotKeyActive 检查热密钥是否激活
func (hkm *HotKeyManager) IsHotKeyActive() bool {
	hkm.mu.RLock()
	defer hkm.mu.RUnlock()
	return hkm.backupMgr.IsHotKeyActive()
}

// startRefresh 启动定期刷新
func (hkm *HotKeyManager) startRefresh() {
	hkm.refreshTicker = time.NewTicker(24 * time.Hour)
	go func() {
		for {
			select {
			case <-hkm.ctx.Done():
				return
			case <-hkm.refreshTicker.C:
				if err := hkm.RefreshHotKey(); err != nil {
					log.Printf("[HotKey] ❌ 刷新热密钥失败: %v", err)
				}
			}
		}
	}()
}

// GetStats 获取统计信息
func (hkm *HotKeyManager) GetStats() map[string]any {
	hkm.mu.RLock()
	defer hkm.mu.RUnlock()
	mode := "cold_start_mode"
	if hkm.backupMgr.IsHotKeyActive() {
		mode = "hot_key_mode"
	}
	return map[string]any{
		"hot_key_active": hkm.backupMgr.IsHotKeyActive(),
		"mode":           mode,
		"epoch":          hkm.epoch,
	}
}

func mustMarshal(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}
