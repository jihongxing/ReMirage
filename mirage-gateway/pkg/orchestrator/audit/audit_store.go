package audit

import (
	"context"
	"time"

	"mirage-gateway/pkg/orchestrator/commit"
)

// AuditFilter 审计记录查询过滤条件
type AuditFilter struct {
	TxType            *commit.TxType
	RollbackTriggered *bool
	StartTime         *time.Time
	EndTime           *time.Time
}

// AuditStore 审计存储接口
type AuditStore interface {
	// Save 保存审计记录
	Save(ctx context.Context, record *AuditRecord) error
	// GetByTxID 按 tx_id 查询审计记录
	GetByTxID(ctx context.Context, txID string) (*AuditRecord, error)
	// List 按过滤条件查询审计记录列表
	List(ctx context.Context, filter *AuditFilter) ([]*AuditRecord, error)
	// Cleanup 清理超过保留天数的记录，默认 90 天
	Cleanup(ctx context.Context, retentionDays int) (int64, error)
}
