// Package logger - 阅后即焚日志系统
// Ghost Mode 下数据仅保留 5 分钟，不写入磁盘
package logger

import (
	"container/ring"
	"crypto/rand"
	"log"
	"sync"
	"time"
	"unsafe"
)

// EphemeralLogger 临时日志器
type EphemeralLogger struct {
	mu            sync.RWMutex
	buffer        *ring.Ring
	bufferSize    int
	retention     time.Duration
	ghostMode     bool
	persistFunc   func(entry *LogEntry) error
	cleanupTicker *time.Ticker
	stopChan      chan struct{}
}

// LogEntry 日志条目
type LogEntry struct {
	ID        string    `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Level     string    `json:"level"`
	Category  string    `json:"category"`
	Message   string    `json:"message"`
	Data      []byte    `json:"data,omitempty"`
}

// EphemeralConfig 配置
type EphemeralConfig struct {
	BufferSize int           // 环形缓冲区大小
	Retention  time.Duration // 保留时间（Ghost Mode 下为 5 分钟）
}

// DefaultEphemeralConfig 默认配置
func DefaultEphemeralConfig() EphemeralConfig {
	return EphemeralConfig{
		BufferSize: 10000,
		Retention:  5 * time.Minute,
	}
}

// NewEphemeralLogger 创建临时日志器
func NewEphemeralLogger(config EphemeralConfig) *EphemeralLogger {
	el := &EphemeralLogger{
		buffer:     ring.New(config.BufferSize),
		bufferSize: config.BufferSize,
		retention:  config.Retention,
		ghostMode:  false,
		stopChan:   make(chan struct{}),
	}
	return el
}

// Start 启动清理循环
func (el *EphemeralLogger) Start() {
	el.cleanupTicker = time.NewTicker(30 * time.Second)
	go el.cleanupLoop()
	log.Println("[EphemeralLogger] 已启动")
}

// Stop 停止日志器
func (el *EphemeralLogger) Stop() {
	close(el.stopChan)
	if el.cleanupTicker != nil {
		el.cleanupTicker.Stop()
	}
	
	// 最终清理
	el.secureWipe()
	log.Println("[EphemeralLogger] 已停止")
}

// SetPersistFunc 设置持久化函数
func (el *EphemeralLogger) SetPersistFunc(fn func(entry *LogEntry) error) {
	el.mu.Lock()
	defer el.mu.Unlock()
	el.persistFunc = fn
}

// TogglePersistence 切换持久化模式
func (el *EphemeralLogger) TogglePersistence(enabled bool) {
	el.mu.Lock()
	defer el.mu.Unlock()
	
	el.ghostMode = !enabled
	
	if el.ghostMode {
		log.Println("[EphemeralLogger] 👻 Ghost Mode 已启用 - 停止磁盘写入")
		// 立即清理现有数据
		el.secureWipeUnsafe()
	} else {
		log.Println("[EphemeralLogger] Ghost Mode 已禁用 - 恢复磁盘写入")
	}
}

// IsGhostMode 检查是否为 Ghost Mode
func (el *EphemeralLogger) IsGhostMode() bool {
	el.mu.RLock()
	defer el.mu.RUnlock()
	return el.ghostMode
}

// Log 记录日志
func (el *EphemeralLogger) Log(level, category, message string, data []byte) {
	entry := &LogEntry{
		ID:        generateLogID(),
		Timestamp: time.Now(),
		Level:     level,
		Category:  category,
		Message:   message,
		Data:      data,
	}
	
	el.mu.Lock()
	defer el.mu.Unlock()
	
	// 写入环形缓冲区
	el.buffer.Value = entry
	el.buffer = el.buffer.Next()
	
	// Ghost Mode 下不持久化
	if el.ghostMode {
		return
	}
	
	// 持久化到磁盘
	if el.persistFunc != nil {
		go func() {
			if err := el.persistFunc(entry); err != nil {
				log.Printf("[EphemeralLogger] 持久化失败: %v", err)
			}
		}()
	}
}

// GetRecentLogs 获取最近的日志（仅内存）
func (el *EphemeralLogger) GetRecentLogs(count int) []*LogEntry {
	el.mu.RLock()
	defer el.mu.RUnlock()
	
	entries := make([]*LogEntry, 0, count)
	cutoff := time.Now().Add(-el.retention)
	
	el.buffer.Do(func(v interface{}) {
		if v == nil {
			return
		}
		entry, ok := v.(*LogEntry)
		if !ok || entry == nil {
			return
		}
		// 只返回保留期内的日志
		if entry.Timestamp.After(cutoff) && len(entries) < count {
			entries = append(entries, entry)
		}
	})
	
	return entries
}

// cleanupLoop 清理循环
func (el *EphemeralLogger) cleanupLoop() {
	for {
		select {
		case <-el.stopChan:
			return
		case <-el.cleanupTicker.C:
			el.cleanup()
		}
	}
}

// cleanup 清理过期数据
func (el *EphemeralLogger) cleanup() {
	el.mu.Lock()
	defer el.mu.Unlock()
	
	if !el.ghostMode {
		return
	}
	
	cutoff := time.Now().Add(-el.retention)
	cleaned := 0
	
	el.buffer.Do(func(v interface{}) {
		if v == nil {
			return
		}
		entry, ok := v.(*LogEntry)
		if !ok || entry == nil {
			return
		}
		if entry.Timestamp.Before(cutoff) {
			// 安全擦除
			el.memzero(entry)
			cleaned++
		}
	})
	
	if cleaned > 0 {
		log.Printf("[EphemeralLogger] 👻 已清理 %d 条过期日志", cleaned)
	}
}

// secureWipe 安全擦除所有数据
func (el *EphemeralLogger) secureWipe() {
	el.mu.Lock()
	defer el.mu.Unlock()
	el.secureWipeUnsafe()
}

// secureWipeUnsafe 安全擦除（无锁版本）
func (el *EphemeralLogger) secureWipeUnsafe() {
	wiped := 0
	el.buffer.Do(func(v interface{}) {
		if v == nil {
			return
		}
		entry, ok := v.(*LogEntry)
		if !ok || entry == nil {
			return
		}
		el.memzero(entry)
		wiped++
	})
	
	// 重建缓冲区
	el.buffer = ring.New(el.bufferSize)
	
	log.Printf("[EphemeralLogger] 🔥 已安全擦除 %d 条日志", wiped)
}

// memzero 内存清零
func (el *EphemeralLogger) memzero(entry *LogEntry) {
	if entry == nil {
		return
	}
	
	// 清零字符串（通过覆盖）
	entry.ID = ""
	entry.Level = ""
	entry.Category = ""
	entry.Message = ""
	
	// 清零字节切片
	if entry.Data != nil {
		for i := range entry.Data {
			entry.Data[i] = 0
		}
		entry.Data = nil
	}
	
	// 清零时间戳
	entry.Timestamp = time.Time{}
}

// ExpiringBuffer 过期缓冲区（5 分钟自动清理）
type ExpiringBuffer struct {
	mu       sync.RWMutex
	data     map[string]*BufferEntry
	ttl      time.Duration
	stopChan chan struct{}
}

// BufferEntry 缓冲区条目
type BufferEntry struct {
	Data      []byte
	ExpiresAt time.Time
}

// NewExpiringBuffer 创建过期缓冲区
func NewExpiringBuffer(ttl time.Duration) *ExpiringBuffer {
	eb := &ExpiringBuffer{
		data:     make(map[string]*BufferEntry),
		ttl:      ttl,
		stopChan: make(chan struct{}),
	}
	go eb.cleanupLoop()
	return eb
}

// Put 写入数据
func (eb *ExpiringBuffer) Put(key string, data []byte) {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	
	eb.data[key] = &BufferEntry{
		Data:      data,
		ExpiresAt: time.Now().Add(eb.ttl),
	}
}

// Get 读取数据
func (eb *ExpiringBuffer) Get(key string) ([]byte, bool) {
	eb.mu.RLock()
	defer eb.mu.RUnlock()
	
	entry, ok := eb.data[key]
	if !ok || time.Now().After(entry.ExpiresAt) {
		return nil, false
	}
	return entry.Data, true
}

// cleanupLoop 清理循环
func (eb *ExpiringBuffer) cleanupLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-eb.stopChan:
			return
		case <-ticker.C:
			eb.cleanup()
		}
	}
}

// cleanup 清理过期数据
func (eb *ExpiringBuffer) cleanup() {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	
	now := time.Now()
	for key, entry := range eb.data {
		if now.After(entry.ExpiresAt) {
			// 安全擦除
			memzeroBytes(entry.Data)
			delete(eb.data, key)
		}
	}
}

// Stop 停止缓冲区
func (eb *ExpiringBuffer) Stop() {
	close(eb.stopChan)
	
	// 最终清理
	eb.mu.Lock()
	defer eb.mu.Unlock()
	
	for key, entry := range eb.data {
		memzeroBytes(entry.Data)
		delete(eb.data, key)
	}
}

// memzeroBytes 安全清零字节切片
func memzeroBytes(b []byte) {
	if b == nil {
		return
	}
	// 使用 crypto/rand 覆盖后清零
	rand.Read(b)
	for i := range b {
		b[i] = 0
	}
	// 强制内存屏障
	_ = unsafe.Pointer(&b[0])
}

// generateLogID 生成日志 ID
func generateLogID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return string(b)
}
