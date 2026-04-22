package budget

import (
	"sync"
	"time"

	"mirage-gateway/pkg/orchestrator/commit"
)

// InMemoryLedger 内存账本实现
type InMemoryLedger struct {
	mu      sync.Mutex
	entries []*LedgerEntry
}

// NewInMemoryLedger 创建内存账本
func NewInMemoryLedger() *InMemoryLedger {
	return &InMemoryLedger{}
}

// Record 记录一次预算消耗
func (l *InMemoryLedger) Record(entry *LedgerEntry) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = append(l.entries, entry)
}

// SwitchCountInLastHour 统计过去 1 小时内指定 session 的 LinkMigration + GatewayReassignment 数量
func (l *InMemoryLedger) SwitchCountInLastHour(sessionID string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	cutoff := time.Now().Add(-1 * time.Hour)
	count := 0
	for _, e := range l.entries {
		if e.SessionID == sessionID && !e.Timestamp.Before(cutoff) &&
			(e.TxType == commit.TxTypeLinkMigration || e.TxType == commit.TxTypeGatewayReassignment) {
			count++
		}
	}
	return count
}

// EntryBurnCountInLastDay 统计过去 24 小时内指定 session 的 GatewayReassignment 数量
func (l *InMemoryLedger) EntryBurnCountInLastDay(sessionID string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	cutoff := time.Now().Add(-24 * time.Hour)
	count := 0
	for _, e := range l.entries {
		if e.SessionID == sessionID && !e.Timestamp.Before(cutoff) &&
			e.TxType == commit.TxTypeGatewayReassignment {
			count++
		}
	}
	return count
}

// Cleanup 清理超过 24 小时的历史记录
func (l *InMemoryLedger) Cleanup() {
	l.mu.Lock()
	defer l.mu.Unlock()
	cutoff := time.Now().Add(-24 * time.Hour)
	kept := make([]*LedgerEntry, 0, len(l.entries))
	for _, e := range l.entries {
		if !e.Timestamp.Before(cutoff) {
			kept = append(kept, e)
		}
	}
	l.entries = kept
}
