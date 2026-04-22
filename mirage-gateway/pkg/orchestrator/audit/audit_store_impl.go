package audit

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// gormAuditStore 基于 GORM 的 AuditStore 实现
type gormAuditStore struct {
	db *gorm.DB
}

// NewGormAuditStore 创建基于 GORM 的 AuditStore
func NewGormAuditStore(db *gorm.DB) AuditStore {
	return &gormAuditStore{db: db}
}

// Save 保存审计记录
func (s *gormAuditStore) Save(ctx context.Context, record *AuditRecord) error {
	return s.db.WithContext(ctx).Create(record).Error
}

// GetByTxID 按 tx_id 查询审计记录
func (s *gormAuditStore) GetByTxID(ctx context.Context, txID string) (*AuditRecord, error) {
	var record AuditRecord
	err := s.db.WithContext(ctx).Where("tx_id = ?", txID).First(&record).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, &ErrAuditRecordNotFound{TxID: txID}
		}
		return nil, err
	}
	return &record, nil
}

// List 按过滤条件查询审计记录列表
func (s *gormAuditStore) List(ctx context.Context, filter *AuditFilter) ([]*AuditRecord, error) {
	query := s.db.WithContext(ctx)

	if filter != nil {
		if filter.TxType != nil {
			query = query.Where("tx_type = ?", *filter.TxType)
		}
		if filter.RollbackTriggered != nil {
			query = query.Where("rollback_triggered = ?", *filter.RollbackTriggered)
		}
		if filter.StartTime != nil {
			query = query.Where("initiated_at >= ?", *filter.StartTime)
		}
		if filter.EndTime != nil {
			query = query.Where("initiated_at <= ?", *filter.EndTime)
		}
	}

	var records []*AuditRecord
	if err := query.Find(&records).Error; err != nil {
		return nil, err
	}
	return records, nil
}

// Cleanup 清理超过保留天数的记录
func (s *gormAuditStore) Cleanup(ctx context.Context, retentionDays int) (int64, error) {
	if retentionDays <= 0 {
		return 0, fmt.Errorf("retentionDays must be > 0, got %d", retentionDays)
	}

	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	result := s.db.WithContext(ctx).Where("created_at < ?", cutoff).Delete(&AuditRecord{})
	if result.Error != nil {
		return 0, result.Error
	}
	return result.RowsAffected, nil
}
