package audit

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

// AuditEntry 审计日志条目
type AuditEntry struct {
	Timestamp   time.Time `json:"ts"`
	CommandType string    `json:"cmd"`
	SourceAddr  string    `json:"src"`
	TargetGW    string    `json:"target"`
	Params      string    `json:"params"`
	Result      string    `json:"result"`
}

// AuditLogger 文件审计日志写入器
type AuditLogger struct {
	mu   sync.Mutex
	file *os.File
}

// NewAuditLogger 创建审计日志写入器，写入指定文件路径
func NewAuditLogger(path string) (*AuditLogger, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return nil, fmt.Errorf("open audit log file: %w", err)
	}
	return &AuditLogger{file: f}, nil
}

// Log 写入一条审计日志（JSON Lines 格式）
func (al *AuditLogger) Log(entry AuditEntry) {
	entry.Timestamp = time.Now()
	al.mu.Lock()
	defer al.mu.Unlock()
	data, err := json.Marshal(entry)
	if err != nil {
		return
	}
	al.file.Write(append(data, '\n'))
}

// Close 关闭审计日志文件
func (al *AuditLogger) Close() error {
	al.mu.Lock()
	defer al.mu.Unlock()
	if al.file != nil {
		return al.file.Close()
	}
	return nil
}
