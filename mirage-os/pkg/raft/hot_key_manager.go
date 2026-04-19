// Package raft - 热密钥管理器
package raft

import (
	"context"
	"log"
	"sync"
	"time"

	"mirage-os/pkg/crypto"
)

// HotKeyManager 热密钥管理器
type HotKeyManager struct {
	mu            sync.RWMutex
	backupMgr     *crypto.BackupManager
	cluster       *Cluster
	refreshTicker *time.Ticker
	ctx           context.Context
	cancel        context.CancelFunc
}

// NewHotKeyManager 创建热密钥管理器
func NewHotKeyManager(backupMgr *crypto.BackupManager, cluster *Cluster) *HotKeyManager {
	ctx, cancel := context.WithCancel(context.Background())
	return &HotKeyManager{
		backupMgr: backupMgr,
		cluster:   cluster,
		ctx:       ctx,
		cancel:    cancel,
	}
}

// Start 启动热密钥管理器
func (hkm *HotKeyManager) Start() error {
	log.Println("[HotKey] 启动热密钥管理器")
	
	// 1. 检查热密钥状态
	if !hkm.backupMgr.IsHotKeyActive() {
		log.Println("[HotKey] ⚠️ 热密钥未激活，需要冷启动")
		return hkm.ColdStart()
	}
	
	log.Println("[HotKey] ✅ 热密钥已激活，零延迟模式")
	
	// 2. 启动定期刷新（可选）
	// hkm.startRefresh()
	
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
	
	// 1. 从 Raft 集群收集份额
	shares, err := hkm.collectShares()
	if err != nil {
		return err
	}
	
	// 2. 激活热密钥
	if err := hkm.backupMgr.ActivateHotKey(shares); err != nil {
		return err
	}
	
	elapsed := time.Since(startTime)
	log.Printf("[HotKey] ✅ 冷启动完成，耗时: %v", elapsed)
	
	if elapsed > 5*time.Second {
		log.Printf("[HotKey] ⚠️ 冷启动耗时超过 5 秒，可能影响 G-Switch 性能")
	}
	
	return nil
}

// collectShares 从 Raft 集群收集份额
func (hkm *HotKeyManager) collectShares() ([]crypto.Share, error) {
	log.Println("[HotKey] 从 Raft 集群收集 Shamir 份额")
	
	// TODO: 实现从 Raft 集群收集份额的逻辑
	// 1. 查询所有在线节点
	// 2. 请求份额（需要至少 3 个）
	// 3. 验证份额有效性
	
	// 模拟收集
	shares := make([]crypto.Share, 3)
	for i := 0; i < 3; i++ {
		shares[i] = crypto.Share{
			Index: i + 1,
			Value: make([]byte, 32),
		}
	}
	
	log.Printf("[HotKey] ✅ 已收集 %d 个份额", len(shares))
	
	return shares, nil
}

// GetMasterKey 获取主密钥（热密钥优先）
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

// RefreshHotKey 刷新热密钥（定期轮换）
func (hkm *HotKeyManager) RefreshHotKey() error {
	hkm.mu.Lock()
	defer hkm.mu.Unlock()
	
	log.Println("[HotKey] 🔄 刷新热密钥")
	
	// 1. 停用旧密钥
	hkm.backupMgr.DeactivateHotKey()
	
	// 2. 生成新密钥
	// TODO: 实现密钥轮换逻辑
	
	// 3. 激活新密钥
	shares, err := hkm.collectShares()
	if err != nil {
		return err
	}
	
	if err := hkm.backupMgr.ActivateHotKey(shares); err != nil {
		return err
	}
	
	log.Println("[HotKey] ✅ 热密钥已刷新")
	
	return nil
}

// startRefresh 启动定期刷新（可选）
func (hkm *HotKeyManager) startRefresh() {
	// 每 24 小时刷新一次热密钥
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
	
	log.Println("[HotKey] 启动定期刷新（24 小时）")
}

// GetStats 获取统计信息
func (hkm *HotKeyManager) GetStats() map[string]interface{} {
	hkm.mu.RLock()
	defer hkm.mu.RUnlock()
	
	return map[string]interface{}{
		"hot_key_active": hkm.backupMgr.IsHotKeyActive(),
		"mode":           hkm.getMode(),
	}
}

// getMode 获取运行模式
func (hkm *HotKeyManager) getMode() string {
	if hkm.backupMgr.IsHotKeyActive() {
		return "hot_key_mode"
	}
	return "cold_start_mode"
}
