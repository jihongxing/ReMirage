package events

import (
	"sync"
	"time"
)

// DeduplicationStore 幂等事件去重集合
type DeduplicationStore interface {
	// Contains 检查 event_id 是否已处理
	Contains(eventID string) bool
	// Add 添加已处理的 event_id
	Add(eventID string)
	// Cleanup 清理超过 1 小时的记录
	Cleanup()
}

// dedupImpl 基于 sync.Map 的 DeduplicationStore 实现
type dedupImpl struct {
	store sync.Map // key: string (event_id), value: time.Time (processed_at)
}

// NewDeduplicationStore 创建 DeduplicationStore 实例
func NewDeduplicationStore() DeduplicationStore {
	return &dedupImpl{}
}

func (d *dedupImpl) Contains(eventID string) bool {
	_, ok := d.store.Load(eventID)
	return ok
}

func (d *dedupImpl) Add(eventID string) {
	d.store.Store(eventID, time.Now())
}

func (d *dedupImpl) Cleanup() {
	cutoff := time.Now().Add(-1 * time.Hour)
	d.store.Range(func(key, value any) bool {
		if t, ok := value.(time.Time); ok && t.Before(cutoff) {
			d.store.Delete(key)
		}
		return true
	})
}
