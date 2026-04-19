// 配额管理器 - 内存态原子操作
package billing

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

type QuotaManager struct {
	mu     sync.RWMutex
	quotas map[string]*Quota
	
	// XMR -> Quota 转换率 (piconero per byte)
	xmrToQuotaRate uint64
	
	onExhausted func(uid string)
	onLow       func(uid string, remaining uint64)
	
	// Redis 双重对账
	redisClient   RedisClient
	snapshotCh    chan struct{}
	lastSnapshot  time.Time
	snapshotMu    sync.Mutex
}

// RedisClient Redis 客户端接口
type RedisClient interface {
	HSet(key string, field string, value interface{}) error
	HGet(key string, field string) (string, error)
	HGetAll(key string) (map[string]string, error)
}

type Quota struct {
	UID            string    `json:"uid"`
	TotalBytes     uint64    `json:"total_bytes"`
	UsedBytes      uint64    `json:"used_bytes"`
	RemainingBytes uint64    `json:"remaining_bytes"`
	ExpiresAt      time.Time `json:"expires_at"`
	PlanType       string    `json:"plan_type"` // starter, pro, unlimited
	Exhausted      uint32    // atomic
}

func NewQuotaManager(xmrToQuotaRate uint64) *QuotaManager {
	return &QuotaManager{
		quotas:         make(map[string]*Quota),
		xmrToQuotaRate: xmrToQuotaRate,
		snapshotCh:     make(chan struct{}),
	}
}

// SetRedisClient 设置 Redis 客户端用于双重对账
func (m *QuotaManager) SetRedisClient(client RedisClient) {
	m.redisClient = client
}

// StartSnapshotLoop 启动快照同步循环（每 60 秒）
func (m *QuotaManager) StartSnapshotLoop() {
	go func() {
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		
		for {
			select {
			case <-m.snapshotCh:
				return
			case <-ticker.C:
				m.syncToRedis()
			}
		}
	}()
}

// StopSnapshotLoop 停止快照同步
func (m *QuotaManager) StopSnapshotLoop() {
	close(m.snapshotCh)
}

// syncToRedis 同步增量快照到 Redis
func (m *QuotaManager) syncToRedis() {
	if m.redisClient == nil {
		return
	}
	
	m.snapshotMu.Lock()
	defer m.snapshotMu.Unlock()
	
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	for uid, quota := range m.quotas {
		// 序列化配额状态
		data := fmt.Sprintf("%d:%d:%d:%d:%s",
			atomic.LoadUint64(&quota.TotalBytes),
			atomic.LoadUint64(&quota.UsedBytes),
			atomic.LoadUint64(&quota.RemainingBytes),
			atomic.LoadUint32(&quota.Exhausted),
			quota.ExpiresAt.Format(time.RFC3339),
		)
		m.redisClient.HSet("mirage:quotas", uid, data)
	}
	
	m.lastSnapshot = time.Now()
}

// RecoverFromRedis 从 Redis 恢复配额状态（崩溃恢复）
func (m *QuotaManager) RecoverFromRedis() error {
	if m.redisClient == nil {
		return nil
	}
	
	data, err := m.redisClient.HGetAll("mirage:quotas")
	if err != nil {
		return err
	}
	
	m.mu.Lock()
	defer m.mu.Unlock()
	
	for uid, value := range data {
		var total, used, remaining uint64
		var exhausted uint32
		var expiresStr string
		
		_, err := fmt.Sscanf(value, "%d:%d:%d:%d:%s", &total, &used, &remaining, &exhausted, &expiresStr)
		if err != nil {
			continue
		}
		
		expiresAt, _ := time.Parse(time.RFC3339, expiresStr)
		
		m.quotas[uid] = &Quota{
			UID:            uid,
			TotalBytes:     total,
			UsedBytes:      used,
			RemainingBytes: remaining,
			ExpiresAt:      expiresAt,
			Exhausted:      exhausted,
		}
	}
	
	return nil
}

// AllocateQuota 分配配额
func (m *QuotaManager) AllocateQuota(uid string, bytes uint64, planType string, duration time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if existing, exists := m.quotas[uid]; exists {
		// 追加配额
		atomic.AddUint64(&existing.TotalBytes, bytes)
		atomic.AddUint64(&existing.RemainingBytes, bytes)
		if time.Now().Add(duration).After(existing.ExpiresAt) {
			existing.ExpiresAt = time.Now().Add(duration)
		}
		atomic.StoreUint32(&existing.Exhausted, 0)
		return nil
	}
	
	m.quotas[uid] = &Quota{
		UID:            uid,
		TotalBytes:     bytes,
		UsedBytes:      0,
		RemainingBytes: bytes,
		ExpiresAt:      time.Now().Add(duration),
		PlanType:       planType,
		Exhausted:      0,
	}
	
	return nil
}

// ConvertXMRToQuota 将 XMR 转换为配额
func (m *QuotaManager) ConvertXMRToQuota(piconero uint64) uint64 {
	if m.xmrToQuotaRate == 0 {
		return 0
	}
	return piconero / m.xmrToQuotaRate
}

// Consume 消费配额（原子操作）
func (m *QuotaManager) Consume(uid string, bytes uint64) error {
	m.mu.RLock()
	quota, exists := m.quotas[uid]
	m.mu.RUnlock()
	
	if !exists {
		return errors.New("quota not found")
	}
	
	if atomic.LoadUint32(&quota.Exhausted) == 1 {
		return errors.New("quota exhausted")
	}
	
	// 检查过期
	if time.Now().After(quota.ExpiresAt) {
		atomic.StoreUint32(&quota.Exhausted, 1)
		return errors.New("quota expired")
	}
	
	// 原子扣减
	for {
		remaining := atomic.LoadUint64(&quota.RemainingBytes)
		if remaining < bytes {
			atomic.StoreUint32(&quota.Exhausted, 1)
			if m.onExhausted != nil {
				go m.onExhausted(uid)
			}
			return errors.New("insufficient quota")
		}
		
		if atomic.CompareAndSwapUint64(&quota.RemainingBytes, remaining, remaining-bytes) {
			atomic.AddUint64(&quota.UsedBytes, bytes)
			
			// 低配额预警
			newRemaining := remaining - bytes
			if newRemaining < quota.TotalBytes/10 && m.onLow != nil {
				go m.onLow(uid, newRemaining)
			}
			
			return nil
		}
	}
}

// GetQuota 获取配额状态
func (m *QuotaManager) GetQuota(uid string) *Quota {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	if q, exists := m.quotas[uid]; exists {
		return &Quota{
			UID:            q.UID,
			TotalBytes:     atomic.LoadUint64(&q.TotalBytes),
			UsedBytes:      atomic.LoadUint64(&q.UsedBytes),
			RemainingBytes: atomic.LoadUint64(&q.RemainingBytes),
			ExpiresAt:      q.ExpiresAt,
			PlanType:       q.PlanType,
			Exhausted:      atomic.LoadUint32(&q.Exhausted),
		}
	}
	return nil
}

// SetOnExhausted 设置耗尽回调
func (m *QuotaManager) SetOnExhausted(fn func(uid string)) {
	m.onExhausted = fn
}

// SetOnLow 设置低配额回调
func (m *QuotaManager) SetOnLow(fn func(uid string, remaining uint64)) {
	m.onLow = fn
}

// IsActive 检查配额是否有效
func (m *QuotaManager) IsActive(uid string) bool {
	m.mu.RLock()
	quota, exists := m.quotas[uid]
	m.mu.RUnlock()
	
	if !exists {
		return false
	}
	
	return atomic.LoadUint32(&quota.Exhausted) == 0 && time.Now().Before(quota.ExpiresAt)
}
