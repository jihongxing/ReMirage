package dispatch

import (
	"database/sql"
	"log"
	"sync"
	"time"
)

// PushLogEntry 下推状态记录条目
type PushLogEntry struct {
	GatewayID   string    `json:"gateway_id"`
	CommandType string    `json:"command_type"`
	Result      string    `json:"result"`
	Timestamp   time.Time `json:"timestamp"`
}

// PushLog 下推状态记录器：内存环形缓冲 + 异步 DB 持久化
type PushLog struct {
	entries []PushLogEntry
	mu      sync.Mutex
	db      *sql.DB
	maxSize int
}

// NewPushLog 创建下推状态记录器
func NewPushLog(db *sql.DB, maxSize int) *PushLog {
	if maxSize <= 0 {
		maxSize = 10000
	}
	return &PushLog{
		entries: make([]PushLogEntry, 0, maxSize),
		db:      db,
		maxSize: maxSize,
	}
}

// Record 记录一次下推结果（内存环形缓冲 + 异步写 DB）
func (pl *PushLog) Record(gatewayID, cmdType, result string) {
	entry := PushLogEntry{
		GatewayID:   gatewayID,
		CommandType: cmdType,
		Result:      result,
		Timestamp:   time.Now(),
	}

	pl.mu.Lock()
	pl.entries = append(pl.entries, entry)
	if len(pl.entries) > pl.maxSize {
		pl.entries = pl.entries[len(pl.entries)-pl.maxSize:]
	}
	pl.mu.Unlock()

	go pl.persistToDB(entry)
}

// persistToDB 异步写入 DB
func (pl *PushLog) persistToDB(entry PushLogEntry) {
	if pl.db == nil {
		return
	}
	_, err := pl.db.Exec(
		`INSERT INTO push_logs (id, gateway_id, command_type, result, created_at) VALUES (gen_random_uuid(), $1, $2, $3, $4)`,
		entry.GatewayID, entry.CommandType, entry.Result, entry.Timestamp,
	)
	if err != nil {
		log.Printf("[PushLog] DB persist error: %v", err)
	}
}

// GetRecent 获取最近 N 条下推记录
func (pl *PushLog) GetRecent(limit int) []PushLogEntry {
	pl.mu.Lock()
	defer pl.mu.Unlock()

	start := len(pl.entries) - limit
	if start < 0 {
		start = 0
	}
	result := make([]PushLogEntry, len(pl.entries)-start)
	copy(result, pl.entries[start:])
	return result
}
