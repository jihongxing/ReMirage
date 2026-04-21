package threat

import (
	"log"
	"sync"
	"time"
)

// IngressLog 入口处置日志条目
type IngressLog struct {
	Timestamp   time.Time     `json:"ts"`
	SourceIP    string        `json:"src"`
	Action      IngressAction `json:"action"`
	Reason      string        `json:"reason"`
	ThreatLevel int           `json:"level"`
}

// IngressLogger 入口处置日志记录器（环形缓冲）
type IngressLogger struct {
	mu         sync.Mutex
	entries    []IngressLog
	maxEntries int
	pos        int
	full       bool
}

// NewIngressLogger 创建入口日志记录器
func NewIngressLogger() *IngressLogger {
	return &IngressLogger{
		entries:    make([]IngressLog, 10000),
		maxEntries: 10000,
	}
}

// Log 记录入口处置日志
func (l *IngressLogger) Log(entry IngressLog) {
	l.mu.Lock()
	l.entries[l.pos] = entry
	l.pos++
	if l.pos >= l.maxEntries {
		l.pos = 0
		l.full = true
	}
	l.mu.Unlock()

	log.Printf("[Ingress] %s src=%s action=%s reason=%s level=%d",
		entry.Timestamp.Format(time.RFC3339), entry.SourceIP,
		entry.Action.String(), entry.Reason, entry.ThreatLevel)
}

// Recent 获取最近 n 条日志
func (l *IngressLogger) Recent(n int) []IngressLog {
	l.mu.Lock()
	defer l.mu.Unlock()

	total := l.pos
	if l.full {
		total = l.maxEntries
	}
	if n > total {
		n = total
	}
	if n == 0 {
		return nil
	}

	result := make([]IngressLog, n)
	for i := 0; i < n; i++ {
		idx := (l.pos - n + i + l.maxEntries) % l.maxEntries
		result[i] = l.entries[idx]
	}
	return result
}
