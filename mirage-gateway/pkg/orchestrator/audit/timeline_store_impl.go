package audit

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// gormTimelineStore 基于 GORM 的 TimelineStore 实现
type gormTimelineStore struct {
	db *gorm.DB
}

// NewGormTimelineStore 创建基于 GORM 的 TimelineStore
func NewGormTimelineStore(db *gorm.DB) TimelineStore {
	return &gormTimelineStore{db: db}
}

// applyTimeRange 应用时间范围过滤
func (s *gormTimelineStore) applyTimeRange(query *gorm.DB, tr *TimeRange) *gorm.DB {
	if tr != nil {
		if tr.Start != nil {
			query = query.Where("timestamp >= ?", *tr.Start)
		}
		if tr.End != nil {
			query = query.Where("timestamp <= ?", *tr.End)
		}
	}
	return query
}

// SaveSessionEntry 保存 Session 时间线条目
func (s *gormTimelineStore) SaveSessionEntry(ctx context.Context, entry *SessionTimelineEntry) error {
	return s.db.WithContext(ctx).Create(entry).Error
}

// ListSessionEntries 查询 Session 时间线条目
func (s *gormTimelineStore) ListSessionEntries(ctx context.Context, sessionID string, tr *TimeRange) ([]*SessionTimelineEntry, error) {
	query := s.db.WithContext(ctx).Where("session_id = ?", sessionID)
	query = s.applyTimeRange(query, tr)

	var entries []*SessionTimelineEntry
	if err := query.Order("timestamp ASC").Find(&entries).Error; err != nil {
		return nil, err
	}
	return entries, nil
}

// SaveLinkHealthEntry 保存 Link 健康时间线条目
func (s *gormTimelineStore) SaveLinkHealthEntry(ctx context.Context, entry *LinkHealthTimelineEntry) error {
	return s.db.WithContext(ctx).Create(entry).Error
}

// ListLinkHealthEntries 查询 Link 健康时间线条目
func (s *gormTimelineStore) ListLinkHealthEntries(ctx context.Context, linkID string, tr *TimeRange) ([]*LinkHealthTimelineEntry, error) {
	query := s.db.WithContext(ctx).Where("link_id = ?", linkID)
	query = s.applyTimeRange(query, tr)

	var entries []*LinkHealthTimelineEntry
	if err := query.Order("timestamp ASC").Find(&entries).Error; err != nil {
		return nil, err
	}
	return entries, nil
}

// SavePersonaVersionEntry 保存 Persona 版本时间线条目
func (s *gormTimelineStore) SavePersonaVersionEntry(ctx context.Context, entry *PersonaVersionTimelineEntry) error {
	return s.db.WithContext(ctx).Create(entry).Error
}

// ListPersonaVersionEntries 按 session_id 查询 Persona 版本时间线条目
func (s *gormTimelineStore) ListPersonaVersionEntries(ctx context.Context, sessionID string, tr *TimeRange) ([]*PersonaVersionTimelineEntry, error) {
	query := s.db.WithContext(ctx).Where("session_id = ?", sessionID)
	query = s.applyTimeRange(query, tr)

	var entries []*PersonaVersionTimelineEntry
	if err := query.Order("timestamp ASC").Find(&entries).Error; err != nil {
		return nil, err
	}
	return entries, nil
}

// ListPersonaVersionEntriesByPersona 按 persona_id 查询 Persona 版本时间线条目
func (s *gormTimelineStore) ListPersonaVersionEntriesByPersona(ctx context.Context, personaID string, tr *TimeRange) ([]*PersonaVersionTimelineEntry, error) {
	query := s.db.WithContext(ctx).Where("persona_id = ?", personaID)
	query = s.applyTimeRange(query, tr)

	var entries []*PersonaVersionTimelineEntry
	if err := query.Order("timestamp ASC").Find(&entries).Error; err != nil {
		return nil, err
	}
	return entries, nil
}

// SaveSurvivalModeEntry 保存 Survival Mode 时间线条目
func (s *gormTimelineStore) SaveSurvivalModeEntry(ctx context.Context, entry *SurvivalModeTimelineEntry) error {
	return s.db.WithContext(ctx).Create(entry).Error
}

// ListSurvivalModeEntries 查询 Survival Mode 时间线条目
func (s *gormTimelineStore) ListSurvivalModeEntries(ctx context.Context, tr *TimeRange) ([]*SurvivalModeTimelineEntry, error) {
	query := s.db.WithContext(ctx).Model(&SurvivalModeTimelineEntry{})
	query = s.applyTimeRange(query, tr)

	var entries []*SurvivalModeTimelineEntry
	if err := query.Order("timestamp ASC").Find(&entries).Error; err != nil {
		return nil, err
	}
	return entries, nil
}

// SaveTransactionEntry 保存 Transaction 时间线条目
func (s *gormTimelineStore) SaveTransactionEntry(ctx context.Context, entry *TransactionTimelineEntry) error {
	return s.db.WithContext(ctx).Create(entry).Error
}

// ListTransactionEntries 按 tx_id 查询 Transaction 时间线条目
func (s *gormTimelineStore) ListTransactionEntries(ctx context.Context, txID string) ([]*TransactionTimelineEntry, error) {
	var entries []*TransactionTimelineEntry
	if err := s.db.WithContext(ctx).Where("tx_id = ?", txID).Order("timestamp ASC").Find(&entries).Error; err != nil {
		return nil, err
	}
	return entries, nil
}

// Cleanup 清理超过保留天数的所有时间线记录
func (s *gormTimelineStore) Cleanup(ctx context.Context, retentionDays int) (int64, error) {
	if retentionDays <= 0 {
		return 0, fmt.Errorf("retentionDays must be > 0, got %d", retentionDays)
	}

	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	var totalDeleted int64

	tables := []interface{}{
		&SessionTimelineEntry{},
		&LinkHealthTimelineEntry{},
		&PersonaVersionTimelineEntry{},
		&SurvivalModeTimelineEntry{},
		&TransactionTimelineEntry{},
	}

	for _, model := range tables {
		result := s.db.WithContext(ctx).Where("timestamp < ?", cutoff).Delete(model)
		if result.Error != nil {
			return totalDeleted, result.Error
		}
		totalDeleted += result.RowsAffected
	}

	return totalDeleted, nil
}
