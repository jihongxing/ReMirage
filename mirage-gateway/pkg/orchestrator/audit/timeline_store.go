package audit

import (
	"context"
	"time"
)

// TimeRange 时间范围过滤
type TimeRange struct {
	Start *time.Time
	End   *time.Time
}

// TimelineStore 时间线存储接口
type TimelineStore interface {
	// Session 时间线
	SaveSessionEntry(ctx context.Context, entry *SessionTimelineEntry) error
	ListSessionEntries(ctx context.Context, sessionID string, tr *TimeRange) ([]*SessionTimelineEntry, error)

	// Link 健康时间线
	SaveLinkHealthEntry(ctx context.Context, entry *LinkHealthTimelineEntry) error
	ListLinkHealthEntries(ctx context.Context, linkID string, tr *TimeRange) ([]*LinkHealthTimelineEntry, error)

	// Persona 版本时间线
	SavePersonaVersionEntry(ctx context.Context, entry *PersonaVersionTimelineEntry) error
	ListPersonaVersionEntries(ctx context.Context, sessionID string, tr *TimeRange) ([]*PersonaVersionTimelineEntry, error)
	ListPersonaVersionEntriesByPersona(ctx context.Context, personaID string, tr *TimeRange) ([]*PersonaVersionTimelineEntry, error)

	// Survival Mode 时间线
	SaveSurvivalModeEntry(ctx context.Context, entry *SurvivalModeTimelineEntry) error
	ListSurvivalModeEntries(ctx context.Context, tr *TimeRange) ([]*SurvivalModeTimelineEntry, error)

	// Transaction 时间线
	SaveTransactionEntry(ctx context.Context, entry *TransactionTimelineEntry) error
	ListTransactionEntries(ctx context.Context, txID string) ([]*TransactionTimelineEntry, error)

	// Cleanup 清理超过保留天数的记录
	Cleanup(ctx context.Context, retentionDays int) (int64, error)
}
