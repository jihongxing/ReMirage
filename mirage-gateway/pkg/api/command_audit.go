package api

import (
	"fmt"
	"log"
	"sync"
	"time"
)

// CommandAuditEntry 命令审计日志条目
type CommandAuditEntry struct {
	Timestamp   time.Time `json:"ts"`
	CommandType string    `json:"cmd"`
	SourceAddr  string    `json:"src"`
	Params      string    `json:"params"`
	Success     bool      `json:"ok"`
	Message     string    `json:"msg"`
}

// CommandAuditor 命令审计日志记录器（环形缓冲）
type CommandAuditor struct {
	mu         sync.Mutex
	entries    []CommandAuditEntry
	maxEntries int
	pos        int
	full       bool
}

// NewCommandAuditor 创建审计日志记录器
func NewCommandAuditor() *CommandAuditor {
	return &CommandAuditor{
		entries:    make([]CommandAuditEntry, 5000),
		maxEntries: 5000,
	}
}

// Log 记录审计日志（仅记录摘要，不记录原始参数全文）
func (a *CommandAuditor) Log(commandType, sourceAddr, params string, success bool, message string) {
	// 摘要化：只保留 type 和 level，不记录原始参数
	summary := fmt.Sprintf("type=%s", commandType)
	entry := CommandAuditEntry{
		Timestamp:   time.Now(),
		CommandType: commandType,
		SourceAddr:  sourceAddr,
		Params:      summary,
		Success:     success,
		Message:     message,
	}

	a.mu.Lock()
	a.entries[a.pos] = entry
	a.pos++
	if a.pos >= a.maxEntries {
		a.pos = 0
		a.full = true
	}
	a.mu.Unlock()

	log.Printf("[Audit] cmd=%s src=%s ok=%v msg=%s", commandType, sourceAddr, success, message)
}

// Recent 获取最近 n 条审计日志
func (a *CommandAuditor) Recent(n int) []CommandAuditEntry {
	a.mu.Lock()
	defer a.mu.Unlock()

	total := a.pos
	if a.full {
		total = a.maxEntries
	}
	if n > total {
		n = total
	}
	if n == 0 {
		return nil
	}

	result := make([]CommandAuditEntry, n)
	for i := 0; i < n; i++ {
		idx := (a.pos - n + i + a.maxEntries) % a.maxEntries
		result[i] = a.entries[idx]
	}
	return result
}
