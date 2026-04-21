// Package api - 按用户流量计数器
package api

import (
	"encoding/binary"
	"os"
	"sync"
	"sync/atomic"
)

// TrafficStats 单用户流量统计
type TrafficStats struct {
	UserID        string
	SessionID     string
	BusinessBytes uint64
	DefenseBytes  uint64
}

// UserTrafficCounter 按 user_id 维度的流量计数器
type UserTrafficCounter struct {
	counters map[string]*trafficEntry // user_id → entry
	mu       sync.Mutex
	seqNum   uint64 // atomic: 全局单调递增序列号
}

// trafficEntry 内部流量条目（原子累加）
type trafficEntry struct {
	userID        string
	sessionID     string
	businessBytes uint64 // atomic
	defenseBytes  uint64 // atomic
}

// NewUserTrafficCounter 创建流量计数器
func NewUserTrafficCounter() *UserTrafficCounter {
	return &UserTrafficCounter{
		counters: make(map[string]*trafficEntry),
	}
}

// Add 累加用户流量
func (tc *UserTrafficCounter) Add(userID, sessionID string, bizBytes, defBytes uint64) {
	tc.mu.Lock()
	e, ok := tc.counters[userID]
	if !ok {
		e = &trafficEntry{userID: userID, sessionID: sessionID}
		tc.counters[userID] = e
	}
	tc.mu.Unlock()
	atomic.AddUint64(&e.businessBytes, bizBytes)
	atomic.AddUint64(&e.defenseBytes, defBytes)
}

// Flush 返回所有用户的流量快照并重置计数器
func (tc *UserTrafficCounter) Flush() []*TrafficStats {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	result := make([]*TrafficStats, 0, len(tc.counters))
	for _, e := range tc.counters {
		biz := atomic.SwapUint64(&e.businessBytes, 0)
		def := atomic.SwapUint64(&e.defenseBytes, 0)
		if biz > 0 || def > 0 {
			result = append(result, &TrafficStats{
				UserID:        e.userID,
				SessionID:     e.sessionID,
				BusinessBytes: biz,
				DefenseBytes:  def,
			})
		}
	}
	return result
}

// NextSeqNum 获取下一个序列号（原子递增）
func (tc *UserTrafficCounter) NextSeqNum() uint64 {
	return atomic.AddUint64(&tc.seqNum, 1)
}

// CurrentSeqNum 获取当前序列号（不递增）
func (tc *UserTrafficCounter) CurrentSeqNum() uint64 {
	return atomic.LoadUint64(&tc.seqNum)
}

// SaveSeqNum 持久化序列号到文件
func (tc *UserTrafficCounter) SaveSeqNum(path string) error {
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, atomic.LoadUint64(&tc.seqNum))
	return os.WriteFile(path, buf, 0600)
}

// LoadSeqNum 从文件恢复序列号
func (tc *UserTrafficCounter) LoadSeqNum(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // 文件不存在，从 0 开始
		}
		return err
	}
	if len(data) < 8 {
		return nil // 文件损坏，从 0 开始
	}
	seq := binary.LittleEndian.Uint64(data)
	atomic.StoreUint64(&tc.seqNum, seq)
	return nil
}
