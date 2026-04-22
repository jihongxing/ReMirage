package budget

import (
	"time"

	"mirage-gateway/pkg/orchestrator/commit"
)

// LedgerEntry 账本条目
type LedgerEntry struct {
	SessionID    string        `json:"session_id"`
	TxType       commit.TxType `json:"tx_type"`
	CostEstimate *CostEstimate `json:"cost_estimate"`
	Timestamp    time.Time     `json:"timestamp"`
}

// BudgetLedger 预算账本接口
type BudgetLedger interface {
	// Record 记录一次预算消耗
	Record(entry *LedgerEntry)
	// SwitchCountInLastHour 统计过去 1 小时内指定 session 的切换次数
	// 统计 LinkMigration + GatewayReassignment
	SwitchCountInLastHour(sessionID string) int
	// EntryBurnCountInLastDay 统计过去 24 小时内指定 session 的入口消耗次数
	// 统计 GatewayReassignment
	EntryBurnCountInLastDay(sessionID string) int
	// Cleanup 清理超过 24 小时的历史记录
	Cleanup()
}
