// 实时烧录引擎 - eBPF 流量计费与熔断
package ebpf

import (
	"context"
	"encoding/binary"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cilium/ebpf"
)

type BurnEngine struct {
	mu              sync.RWMutex
	trafficStatsMap *ebpf.Map
	quotaMap        *ebpf.Map
	whitelistMap    *ebpf.Map
	
	activeQuotas    map[string]*UserQuota // UID -> Quota
	pricePerByte    map[string]uint64     // protocol -> price (piconero/byte)
	
	onQuotaExhausted func(uid string)
	onQuotaLow       func(uid string, remaining uint64)
	
	stopCh chan struct{}
}

type UserQuota struct {
	UID            string
	TotalBytes     uint64
	UsedBytes      uint64
	RemainingBytes uint64
	BurnRate       uint64 // bytes/sec (滑动窗口)
	LastUpdate     time.Time
	Exhausted      uint32 // atomic flag
}

type TrafficStats struct {
	UID        [12]byte
	PacketsTx  uint64
	PacketsRx  uint64
	BytesTx    uint64
	BytesRx    uint64
	LastUpdate uint64
}

func NewBurnEngine(trafficMap, quotaMap, whitelistMap *ebpf.Map) *BurnEngine {
	return &BurnEngine{
		trafficStatsMap: trafficMap,
		quotaMap:        quotaMap,
		whitelistMap:    whitelistMap,
		activeQuotas:    make(map[string]*UserQuota),
		pricePerByte: map[string]uint64{
			"h3":       2,  // H3 流量单价高
			"standard": 1,
		},
		stopCh: make(chan struct{}),
	}
}

// Start 启动烧录引擎
func (e *BurnEngine) Start(ctx context.Context) {
	go e.burnLoop(ctx)
}

// burnLoop 每秒拉取 eBPF 计数器并扣费
func (e *BurnEngine) burnLoop(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-e.stopCh:
			return
		case <-ticker.C:
			e.processBurn()
		}
	}
}

// processBurn 处理一轮扣费
func (e *BurnEngine) processBurn() {
	e.mu.Lock()
	defer e.mu.Unlock()
	
	now := time.Now()
	
	for uid, quota := range e.activeQuotas {
		if atomic.LoadUint32(&quota.Exhausted) == 1 {
			continue
		}
		
		// 从 eBPF Map 读取流量统计
		stats, err := e.readTrafficStats(uid)
		if err != nil {
			continue
		}
		
		// 计算增量
		totalBytes := stats.BytesTx + stats.BytesRx
		if totalBytes <= quota.UsedBytes {
			continue
		}
		
		deltaBytes := totalBytes - quota.UsedBytes
		
		// 计算费用（按协议权重）
		cost := deltaBytes * e.pricePerByte["standard"]
		
		// 扣费
		if quota.RemainingBytes <= cost {
			// 配额耗尽
			quota.RemainingBytes = 0
			quota.UsedBytes = totalBytes
			atomic.StoreUint32(&quota.Exhausted, 1)
			
			// 从 eBPF 白名单移除
			e.revokeAccess(uid)
			
			if e.onQuotaExhausted != nil {
				go e.onQuotaExhausted(uid)
			}
		} else {
			quota.RemainingBytes -= cost
			quota.UsedBytes = totalBytes
			
			// 计算烧录速率（滑动窗口）
			elapsed := now.Sub(quota.LastUpdate).Seconds()
			if elapsed > 0 {
				quota.BurnRate = uint64(float64(deltaBytes) / elapsed)
			}
			
			// 低配额预警 (< 10%)
			if quota.RemainingBytes < quota.TotalBytes/10 && e.onQuotaLow != nil {
				go e.onQuotaLow(uid, quota.RemainingBytes)
			}
		}
		
		quota.LastUpdate = now
	}
}

// readTrafficStats 从 eBPF Map 读取流量统计
func (e *BurnEngine) readTrafficStats(uid string) (*TrafficStats, error) {
	if e.trafficStatsMap == nil {
		return &TrafficStats{}, nil
	}
	
	var key [12]byte
	copy(key[:], uid)
	
	var stats TrafficStats
	if err := e.trafficStatsMap.Lookup(key, &stats); err != nil {
		return nil, err
	}
	
	return &stats, nil
}

// GrantAccess 授权用户访问
func (e *BurnEngine) GrantAccess(uid string, quotaBytes uint64) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	
	quota := &UserQuota{
		UID:            uid,
		TotalBytes:     quotaBytes,
		UsedBytes:      0,
		RemainingBytes: quotaBytes,
		BurnRate:       0,
		LastUpdate:     time.Now(),
		Exhausted:      0,
	}
	
	e.activeQuotas[uid] = quota
	
	// 写入 eBPF 白名单
	return e.updateWhitelist(uid, 1)
}

// RevokeAccess 撤销用户访问
func (e *BurnEngine) RevokeAccess(uid string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.revokeAccess(uid)
}

func (e *BurnEngine) revokeAccess(uid string) error {
	delete(e.activeQuotas, uid)
	return e.updateWhitelist(uid, 0)
}

// updateWhitelist 更新 eBPF 白名单
func (e *BurnEngine) updateWhitelist(uid string, allowed uint32) error {
	if e.whitelistMap == nil {
		return nil
	}
	
	var key [12]byte
	copy(key[:], uid)
	
	value := make([]byte, 4)
	binary.LittleEndian.PutUint32(value, allowed)
	
	return e.whitelistMap.Put(key, value)
}

// AddQuota 追加配额
func (e *BurnEngine) AddQuota(uid string, additionalBytes uint64) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	
	quota, exists := e.activeQuotas[uid]
	if !exists {
		return e.GrantAccess(uid, additionalBytes)
	}
	
	quota.TotalBytes += additionalBytes
	quota.RemainingBytes += additionalBytes
	
	// 如果之前耗尽，重新激活
	if atomic.LoadUint32(&quota.Exhausted) == 1 {
		atomic.StoreUint32(&quota.Exhausted, 0)
		return e.updateWhitelist(uid, 1)
	}
	
	return nil
}

// GetQuotaStatus 获取配额状态
func (e *BurnEngine) GetQuotaStatus(uid string) *UserQuota {
	e.mu.RLock()
	defer e.mu.RUnlock()
	
	if quota, exists := e.activeQuotas[uid]; exists {
		// 返回副本
		return &UserQuota{
			UID:            quota.UID,
			TotalBytes:     quota.TotalBytes,
			UsedBytes:      quota.UsedBytes,
			RemainingBytes: quota.RemainingBytes,
			BurnRate:       quota.BurnRate,
			LastUpdate:     quota.LastUpdate,
			Exhausted:      atomic.LoadUint32(&quota.Exhausted),
		}
	}
	return nil
}

// SetOnQuotaExhausted 设置配额耗尽回调
func (e *BurnEngine) SetOnQuotaExhausted(fn func(uid string)) {
	e.onQuotaExhausted = fn
}

// SetOnQuotaLow 设置低配额预警回调
func (e *BurnEngine) SetOnQuotaLow(fn func(uid string, remaining uint64)) {
	e.onQuotaLow = fn
}

// Stop 停止引擎
func (e *BurnEngine) Stop() {
	close(e.stopCh)
}
