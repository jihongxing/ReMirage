// Package api - 用户隔离配额桶
package api

import (
	"sync"
	"sync/atomic"
	"time"

	pb "mirage-proto/gen"
)

// GlobalBucketKey 向后兼容：user_id 为空时使用的全局桶 key
const GlobalBucketKey = "__global__"

// UserQuota 单用户配额桶
type UserQuota struct {
	UserID         string
	RemainingBytes uint64 // atomic
	TotalBytes     uint64
	Exhausted      uint32 // atomic: 0=正常, 1=耗尽
	UpdatedAt      time.Time
}

// QuotaBucketManager 按 user_id 隔离的配额桶管理器
type QuotaBucketManager struct {
	buckets     map[string]*UserQuota // user_id → quota
	mu          sync.RWMutex
	onExhausted func(userID string)
}

// NewQuotaBucketManager 创建配额桶管理器
func NewQuotaBucketManager() *QuotaBucketManager {
	return &QuotaBucketManager{
		buckets: make(map[string]*UserQuota),
	}
}

// SetOnExhausted 设置配额耗尽回调
func (m *QuotaBucketManager) SetOnExhausted(fn func(userID string)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onExhausted = fn
}

// UpdateQuota OS 下发配额更新
func (m *QuotaBucketManager) UpdateQuota(userID string, remainingBytes uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	bucket, ok := m.buckets[userID]
	if !ok {
		bucket = &UserQuota{UserID: userID, TotalBytes: remainingBytes}
		m.buckets[userID] = bucket
	}
	atomic.StoreUint64(&bucket.RemainingBytes, remainingBytes)
	atomic.StoreUint32(&bucket.Exhausted, 0) // 重置耗尽标记
	bucket.TotalBytes = remainingBytes
	bucket.UpdatedAt = time.Now()
}

// Consume 消费配额（CAS 原子操作），返回是否允许
func (m *QuotaBucketManager) Consume(userID string, bytes uint64) bool {
	m.mu.RLock()
	bucket, ok := m.buckets[userID]
	m.mu.RUnlock()
	if !ok {
		return false // 未知用户拒绝
	}
	if atomic.LoadUint32(&bucket.Exhausted) == 1 {
		return false
	}
	for {
		remaining := atomic.LoadUint64(&bucket.RemainingBytes)
		if remaining < bytes {
			if atomic.CompareAndSwapUint32(&bucket.Exhausted, 0, 1) {
				m.mu.RLock()
				cb := m.onExhausted
				m.mu.RUnlock()
				if cb != nil {
					go cb(userID)
				}
			}
			return false
		}
		if atomic.CompareAndSwapUint64(&bucket.RemainingBytes, remaining, remaining-bytes) {
			if remaining == bytes { // 恰好耗尽：余额从 remaining 减至 0
				if atomic.CompareAndSwapUint32(&bucket.Exhausted, 0, 1) {
					m.mu.RLock()
					cb := m.onExhausted
					m.mu.RUnlock()
					if cb != nil {
						go cb(userID)
					}
				}
			}
			return true
		}
	}
}

// GetSummaries 获取所有用户配额摘要（心跳上报用）
func (m *QuotaBucketManager) GetSummaries() []*pb.UserQuotaSummary {
	m.mu.RLock()
	defer m.mu.RUnlock()
	summaries := make([]*pb.UserQuotaSummary, 0, len(m.buckets))
	for _, b := range m.buckets {
		summaries = append(summaries, &pb.UserQuotaSummary{
			UserId:         b.UserID,
			RemainingBytes: atomic.LoadUint64(&b.RemainingBytes),
		})
	}
	return summaries
}

// GetRemaining 获取指定用户剩余配额
func (m *QuotaBucketManager) GetRemaining(userID string) (uint64, bool) {
	m.mu.RLock()
	bucket, ok := m.buckets[userID]
	m.mu.RUnlock()
	if !ok {
		return 0, false
	}
	return atomic.LoadUint64(&bucket.RemainingBytes), true
}

// IsExhausted 检查指定用户配额是否耗尽
func (m *QuotaBucketManager) IsExhausted(userID string) bool {
	m.mu.RLock()
	bucket, ok := m.buckets[userID]
	m.mu.RUnlock()
	if !ok {
		return false
	}
	return atomic.LoadUint32(&bucket.Exhausted) == 1
}
