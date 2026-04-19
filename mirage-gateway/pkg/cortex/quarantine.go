// Package cortex 隔离区管理
// 实现指纹灰度测试，防止误伤
package cortex

import (
	"math/rand"
	"sync"
	"time"
)

// QuarantineManager 隔离区管理器
type QuarantineManager struct {
	mu sync.RWMutex

	// 隔离中的指纹
	quarantined map[string]*QuarantineEntry

	// 配置
	config QuarantineConfig

	// 回调
	onPromote  func(hash string) // 晋升到黑名单
	onRelease  func(hash string) // 释放（误判）
	onJitter   func(hash string, delay time.Duration)
}

// QuarantineEntry 隔离条目
type QuarantineEntry struct {
	Hash           string
	EnteredAt      time.Time
	ExpiresAt      time.Time
	ObservationLog []ObservationEvent
	SuspicionScore int
	JitterApplied  int
	
	// 行为特征
	ScanPatternDetected    bool
	HighFrequencyRequests  bool
	SensitivePathAccess    bool
	AnomalousUserAgent     bool
}

// ObservationEvent 观察事件
type ObservationEvent struct {
	Timestamp   time.Time
	EventType   string
	Description string
	Score       int
}

// QuarantineConfig 隔离配置
type QuarantineConfig struct {
	ObservationPeriod   time.Duration // 观察期
	MinJitter           time.Duration // 最小抖动
	MaxJitter           time.Duration // 最大抖动
	PromotionThreshold  int           // 晋升阈值
	ReleaseThreshold    int           // 释放阈值（低于此分数释放）
	MaxQuarantineSize   int           // 最大隔离数量
}

// DefaultQuarantineConfig 默认配置
func DefaultQuarantineConfig() QuarantineConfig {
	return QuarantineConfig{
		ObservationPeriod:  30 * time.Minute,
		MinJitter:          100 * time.Millisecond,
		MaxJitter:          500 * time.Millisecond,
		PromotionThreshold: 50,
		ReleaseThreshold:   10,
		MaxQuarantineSize:  5000,
	}
}

// NewQuarantineManager 创建隔离区管理器
func NewQuarantineManager(config QuarantineConfig) *QuarantineManager {
	qm := &QuarantineManager{
		quarantined: make(map[string]*QuarantineEntry),
		config:      config,
	}
	go qm.runExpirationLoop()
	return qm
}

// Quarantine 将指纹放入隔离区
func (qm *QuarantineManager) Quarantine(hash string) *QuarantineEntry {
	qm.mu.Lock()
	defer qm.mu.Unlock()

	if entry, exists := qm.quarantined[hash]; exists {
		return entry
	}

	// 检查容量
	if len(qm.quarantined) >= qm.config.MaxQuarantineSize {
		qm.evictOldest()
	}

	entry := &QuarantineEntry{
		Hash:           hash,
		EnteredAt:      time.Now(),
		ExpiresAt:      time.Now().Add(qm.config.ObservationPeriod),
		ObservationLog: make([]ObservationEvent, 0),
	}
	qm.quarantined[hash] = entry
	return entry
}

// IsQuarantined 检查是否在隔离区
func (qm *QuarantineManager) IsQuarantined(hash string) bool {
	qm.mu.RLock()
	defer qm.mu.RUnlock()
	_, exists := qm.quarantined[hash]
	return exists
}

// GetJitterDelay 获取应注入的抖动延迟
func (qm *QuarantineManager) GetJitterDelay(hash string) time.Duration {
	qm.mu.Lock()
	defer qm.mu.Unlock()

	entry, exists := qm.quarantined[hash]
	if !exists {
		return 0
	}

	// 计算随机抖动
	jitterRange := qm.config.MaxJitter - qm.config.MinJitter
	delay := qm.config.MinJitter + time.Duration(rand.Int63n(int64(jitterRange)))

	entry.JitterApplied++

	if qm.onJitter != nil {
		go qm.onJitter(hash, delay)
	}

	return delay
}

// RecordObservation 记录观察事件
func (qm *QuarantineManager) RecordObservation(hash string, eventType string, description string, score int) {
	qm.mu.Lock()
	defer qm.mu.Unlock()

	entry, exists := qm.quarantined[hash]
	if !exists {
		return
	}

	event := ObservationEvent{
		Timestamp:   time.Now(),
		EventType:   eventType,
		Description: description,
		Score:       score,
	}
	entry.ObservationLog = append(entry.ObservationLog, event)
	entry.SuspicionScore += score

	// 更新行为特征
	switch eventType {
	case "scan_pattern":
		entry.ScanPatternDetected = true
	case "high_frequency":
		entry.HighFrequencyRequests = true
	case "sensitive_path":
		entry.SensitivePathAccess = true
	case "anomalous_ua":
		entry.AnomalousUserAgent = true
	}

	// 检查是否达到晋升阈值
	if entry.SuspicionScore >= qm.config.PromotionThreshold {
		qm.promoteToBlacklist(hash)
	}
}

// promoteToBlacklist 晋升到黑名单
func (qm *QuarantineManager) promoteToBlacklist(hash string) {
	delete(qm.quarantined, hash)
	if qm.onPromote != nil {
		go qm.onPromote(hash)
	}
}

// release 释放指纹
func (qm *QuarantineManager) release(hash string) {
	delete(qm.quarantined, hash)
	if qm.onRelease != nil {
		go qm.onRelease(hash)
	}
}

// runExpirationLoop 运行过期检查循环
func (qm *QuarantineManager) runExpirationLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		qm.checkExpirations()
	}
}

// checkExpirations 检查过期条目
func (qm *QuarantineManager) checkExpirations() {
	qm.mu.Lock()
	defer qm.mu.Unlock()

	now := time.Now()
	for hash, entry := range qm.quarantined {
		if now.After(entry.ExpiresAt) {
			// 观察期结束，根据分数决定
			if entry.SuspicionScore >= qm.config.PromotionThreshold {
				qm.promoteToBlacklist(hash)
			} else if entry.SuspicionScore <= qm.config.ReleaseThreshold {
				qm.release(hash)
			} else {
				// 延长观察期
				entry.ExpiresAt = now.Add(qm.config.ObservationPeriod / 2)
			}
		}
	}
}

// evictOldest 驱逐最旧条目
func (qm *QuarantineManager) evictOldest() {
	var oldest *QuarantineEntry
	var oldestHash string

	for hash, entry := range qm.quarantined {
		if oldest == nil || entry.EnteredAt.Before(oldest.EnteredAt) {
			oldest = entry
			oldestHash = hash
		}
	}

	if oldestHash != "" {
		delete(qm.quarantined, oldestHash)
	}
}

// GetEntry 获取隔离条目
func (qm *QuarantineManager) GetEntry(hash string) *QuarantineEntry {
	qm.mu.RLock()
	defer qm.mu.RUnlock()
	return qm.quarantined[hash]
}

// GetAllEntries 获取所有隔离条目
func (qm *QuarantineManager) GetAllEntries() []*QuarantineEntry {
	qm.mu.RLock()
	defer qm.mu.RUnlock()

	entries := make([]*QuarantineEntry, 0, len(qm.quarantined))
	for _, entry := range qm.quarantined {
		entries = append(entries, entry)
	}
	return entries
}

// GetStats 获取统计
func (qm *QuarantineManager) GetStats() QuarantineStats {
	qm.mu.RLock()
	defer qm.mu.RUnlock()

	stats := QuarantineStats{
		TotalQuarantined: len(qm.quarantined),
	}

	for _, entry := range qm.quarantined {
		stats.TotalJitterApplied += entry.JitterApplied
		if entry.ScanPatternDetected {
			stats.ScanPatternsDetected++
		}
	}

	return stats
}

// QuarantineStats 统计信息
type QuarantineStats struct {
	TotalQuarantined     int
	TotalJitterApplied   int
	ScanPatternsDetected int
}

// OnPromote 设置晋升回调
func (qm *QuarantineManager) OnPromote(fn func(hash string)) {
	qm.mu.Lock()
	defer qm.mu.Unlock()
	qm.onPromote = fn
}

// OnRelease 设置释放回调
func (qm *QuarantineManager) OnRelease(fn func(hash string)) {
	qm.mu.Lock()
	defer qm.mu.Unlock()
	qm.onRelease = fn
}

// OnJitter 设置抖动回调
func (qm *QuarantineManager) OnJitter(fn func(hash string, delay time.Duration)) {
	qm.mu.Lock()
	defer qm.mu.Unlock()
	qm.onJitter = fn
}
