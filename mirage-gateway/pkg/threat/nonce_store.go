package threat

import (
	"context"
	"encoding/hex"
	"sort"
	"sync"
	"time"
)

// nonceEntry 存储单个 nonce 的元数据
type nonceEntry struct {
	SourceIP  string
	Timestamp time.Time
	CreatedAt time.Time
}

// NonceStore 抗重放 nonce 存储，用于检测重复 nonce 并归因源 IP
type NonceStore struct {
	mu      sync.RWMutex
	entries map[string]*nonceEntry // key = nonce hex encoding
	maxSize int
	ttl     time.Duration
}

// NewNonceStore 创建 NonceStore 实例
// maxSize: 最大条目数（默认 100000）
// ttl: 条目过期时间（默认 5min）
func NewNonceStore(maxSize int, ttl time.Duration) *NonceStore {
	return &NonceStore{
		entries: make(map[string]*nonceEntry),
		maxSize: maxSize,
		ttl:     ttl,
	}
}

// CheckAndStore 检查 nonce 是否重复，不重复则存储
// 返回 (isDuplicate, originalIP)
func (ns *NonceStore) CheckAndStore(nonce []byte, sourceIP string, timestamp time.Time) (bool, string) {
	key := hex.EncodeToString(nonce)

	ns.mu.Lock()
	defer ns.mu.Unlock()

	if entry, ok := ns.entries[key]; ok {
		return true, entry.SourceIP
	}

	// 容量超限时执行 LRU 淘汰（删除最早 10%）
	if len(ns.entries) >= ns.maxSize {
		ns.evictOldest()
	}

	ns.entries[key] = &nonceEntry{
		SourceIP:  sourceIP,
		Timestamp: timestamp,
		CreatedAt: time.Now(),
	}
	return false, ""
}

// evictOldest 删除最早的 10% 条目（按 CreatedAt 排序）
// 调用方必须持有写锁
func (ns *NonceStore) evictOldest() {
	type kv struct {
		key       string
		createdAt time.Time
	}

	items := make([]kv, 0, len(ns.entries))
	for k, v := range ns.entries {
		items = append(items, kv{key: k, createdAt: v.CreatedAt})
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].createdAt.Before(items[j].createdAt)
	})

	evictCount := len(items) / 10
	if evictCount == 0 {
		evictCount = 1
	}
	for i := 0; i < evictCount; i++ {
		delete(ns.entries, items[i].key)
	}
}

// StartCleanup 启动过期清理循环，每 30 秒清理 CreatedAt 超过 ttl 的条目
func (ns *NonceStore) StartCleanup(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				ns.cleanup()
			}
		}
	}()
}

// cleanup 删除过期条目
func (ns *NonceStore) cleanup() {
	ns.mu.Lock()
	defer ns.mu.Unlock()

	now := time.Now()
	for k, v := range ns.entries {
		if now.Sub(v.CreatedAt) > ns.ttl {
			delete(ns.entries, k)
		}
	}
}

// CheckTimestamp 检查时间戳偏差是否在允许范围内
// 返回 true 表示时间戳有效，false 表示偏差超过 maxSkew
func (ns *NonceStore) CheckTimestamp(timestamp time.Time, maxSkew time.Duration) bool {
	diff := time.Since(timestamp)
	if diff < 0 {
		diff = -diff
	}
	return diff <= maxSkew
}
