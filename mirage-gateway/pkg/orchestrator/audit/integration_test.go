package audit

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"mirage-gateway/pkg/orchestrator"
	"mirage-gateway/pkg/orchestrator/budget"
	"mirage-gateway/pkg/orchestrator/commit"
	"mirage-gateway/pkg/orchestrator/events"
	"mirage-gateway/pkg/orchestrator/survival"
)

// setupTestDB creates an in-memory SQLite DB with all audit models migrated.
func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	err = db.AutoMigrate(
		&AuditRecord{},
		&SessionTimelineEntry{},
		&LinkHealthTimelineEntry{},
		&PersonaVersionTimelineEntry{},
		&SurvivalModeTimelineEntry{},
		&TransactionTimelineEntry{},
	)
	if err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	return db
}

// =============================================================================
// 10.1 GORM AutoMigrate creates 6 tables + basic CRUD verification
// =============================================================================

func TestIntegration_AutoMigrate_Creates6Tables(t *testing.T) {
	db := setupTestDB(t)

	// Verify each table exists by performing a simple query
	tables := []struct {
		name  string
		model interface{}
	}{
		{"audit_records", &AuditRecord{}},
		{"session_timeline", &SessionTimelineEntry{}},
		{"link_health_timeline", &LinkHealthTimelineEntry{}},
		{"persona_version_timeline", &PersonaVersionTimelineEntry{}},
		{"survival_mode_timeline", &SurvivalModeTimelineEntry{}},
		{"transaction_timeline", &TransactionTimelineEntry{}},
	}

	for _, tt := range tables {
		var count int64
		err := db.Model(tt.model).Count(&count).Error
		if err != nil {
			t.Errorf("table %s: query failed: %v", tt.name, err)
		}
		if count != 0 {
			t.Errorf("table %s: expected 0 rows, got %d", tt.name, count)
		}
	}

	// Verify basic CRUD works on each table
	now := time.Now().UTC().Truncate(time.Second)

	rec := &AuditRecord{AuditID: uuid.New().String(), TxID: "tx-1", TxType: commit.TxTypePersonaSwitch, InitiatedAt: now, FinishedAt: now, TargetState: json.RawMessage(`{}`)}
	if err := db.Create(rec).Error; err != nil {
		t.Fatalf("audit_records insert failed: %v", err)
	}
	var fetched AuditRecord
	if err := db.First(&fetched, "audit_id = ?", rec.AuditID).Error; err != nil {
		t.Fatalf("audit_records read failed: %v", err)
	}

	se := &SessionTimelineEntry{EntryID: uuid.New().String(), SessionID: "s1", FromState: orchestrator.SessionPhaseActive, ToState: orchestrator.SessionPhaseDegraded, Timestamp: now}
	if err := db.Create(se).Error; err != nil {
		t.Fatalf("session_timeline insert failed: %v", err)
	}

	lhe := &LinkHealthTimelineEntry{EntryID: uuid.New().String(), LinkID: "l1", Phase: orchestrator.LinkPhaseActive, EventType: "health_update", Timestamp: now}
	if err := db.Create(lhe).Error; err != nil {
		t.Fatalf("link_health_timeline insert failed: %v", err)
	}

	pve := &PersonaVersionTimelineEntry{EntryID: uuid.New().String(), SessionID: "s1", PersonaID: "p1", EventType: "switch", Timestamp: now}
	if err := db.Create(pve).Error; err != nil {
		t.Fatalf("persona_version_timeline insert failed: %v", err)
	}

	sme := &SurvivalModeTimelineEntry{EntryID: uuid.New().String(), FromMode: orchestrator.SurvivalModeNormal, ToMode: orchestrator.SurvivalModeHardened, Timestamp: now}
	if err := db.Create(sme).Error; err != nil {
		t.Fatalf("survival_mode_timeline insert failed: %v", err)
	}

	tte := &TransactionTimelineEntry{EntryID: uuid.New().String(), TxID: "tx-1", FromPhase: commit.TxPhasePreparing, ToPhase: commit.TxPhaseValidating, PhaseData: json.RawMessage(`{}`), Timestamp: now}
	if err := db.Create(tte).Error; err != nil {
		t.Fatalf("transaction_timeline insert failed: %v", err)
	}
}

// =============================================================================
// 10.2 AuditStore CRUD
// =============================================================================

func TestIntegration_AuditStore_CRUD(t *testing.T) {
	db := setupTestDB(t)
	store := NewGormAuditStore(db)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	past := now.Add(-48 * time.Hour)

	// 1. Save 3 AuditRecords with different tx_types and rollback_triggered values
	records := []*AuditRecord{
		{
			AuditID: uuid.New().String(), TxID: "tx-a", TxType: commit.TxTypePersonaSwitch,
			InitiatedAt: now, FinishedAt: now, FlipSuccess: true, RollbackTriggered: false,
			TargetState: json.RawMessage(`{}`),
		},
		{
			AuditID: uuid.New().String(), TxID: "tx-b", TxType: commit.TxTypeLinkMigration,
			InitiatedAt: past, FinishedAt: past, FlipSuccess: false, RollbackTriggered: true,
			TargetState: json.RawMessage(`{}`),
		},
		{
			AuditID: uuid.New().String(), TxID: "tx-c", TxType: commit.TxTypePersonaSwitch,
			InitiatedAt: now, FinishedAt: now, FlipSuccess: false, RollbackTriggered: false,
			TargetState: json.RawMessage(`{}`),
		},
	}

	for _, r := range records {
		if err := store.Save(ctx, r); err != nil {
			t.Fatalf("Save failed: %v", err)
		}
	}

	// 2. GetByTxID for each, verify fields
	for _, r := range records {
		got, err := store.GetByTxID(ctx, r.TxID)
		if err != nil {
			t.Fatalf("GetByTxID(%s) failed: %v", r.TxID, err)
		}
		if got.AuditID != r.AuditID {
			t.Errorf("AuditID mismatch for %s", r.TxID)
		}
		if got.TxType != r.TxType {
			t.Errorf("TxType mismatch for %s: got %s, want %s", r.TxID, got.TxType, r.TxType)
		}
		if got.FlipSuccess != r.FlipSuccess {
			t.Errorf("FlipSuccess mismatch for %s", r.TxID)
		}
		if got.RollbackTriggered != r.RollbackTriggered {
			t.Errorf("RollbackTriggered mismatch for %s", r.TxID)
		}
	}

	// 3. List with tx_type filter
	txType := commit.TxTypePersonaSwitch
	filtered, err := store.List(ctx, &AuditFilter{TxType: &txType})
	if err != nil {
		t.Fatalf("List by tx_type failed: %v", err)
	}
	if len(filtered) != 2 {
		t.Fatalf("expected 2 PersonaSwitch records, got %d", len(filtered))
	}

	// 4. List with rollback_triggered filter
	rbTrue := true
	filtered, err = store.List(ctx, &AuditFilter{RollbackTriggered: &rbTrue})
	if err != nil {
		t.Fatalf("List by rollback_triggered failed: %v", err)
	}
	if len(filtered) != 1 {
		t.Fatalf("expected 1 rollback record, got %d", len(filtered))
	}
	if filtered[0].TxID != "tx-b" {
		t.Errorf("expected tx-b, got %s", filtered[0].TxID)
	}

	// 5. List with time range filter
	rangeStart := now.Add(-1 * time.Hour)
	rangeEnd := now.Add(1 * time.Hour)
	filtered, err = store.List(ctx, &AuditFilter{StartTime: &rangeStart, EndTime: &rangeEnd})
	if err != nil {
		t.Fatalf("List by time range failed: %v", err)
	}
	if len(filtered) != 2 {
		t.Fatalf("expected 2 records in time range, got %d", len(filtered))
	}

	// 6. Cleanup with retention days — set records' created_at to old time, then cleanup
	// Update created_at of tx-b to 100 days ago
	db.Model(&AuditRecord{}).Where("tx_id = ?", "tx-b").Update("created_at", now.AddDate(0, 0, -100))
	deleted, err := store.Cleanup(ctx, 90)
	if err != nil {
		t.Fatalf("Cleanup failed: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("expected 1 deleted, got %d", deleted)
	}

	// Verify tx-b is gone
	_, err = store.GetByTxID(ctx, "tx-b")
	if err == nil {
		t.Fatal("expected tx-b to be deleted after cleanup")
	}
	if _, ok := err.(*ErrAuditRecordNotFound); !ok {
		t.Fatalf("expected ErrAuditRecordNotFound, got %T: %v", err, err)
	}

	// Verify tx-a and tx-c still exist
	for _, txID := range []string{"tx-a", "tx-c"} {
		if _, err := store.GetByTxID(ctx, txID); err != nil {
			t.Errorf("expected %s to still exist, got error: %v", txID, err)
		}
	}
}

// =============================================================================
// 10.3 TimelineStore 5 types CRUD
// =============================================================================

func TestIntegration_TimelineStore_5Types_CRUD(t *testing.T) {
	db := setupTestDB(t)
	store := NewGormTimelineStore(db)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	t1 := now.Add(-3 * time.Hour)
	t2 := now.Add(-2 * time.Hour)
	t3 := now.Add(-1 * time.Hour)

	// --- Session Timeline ---
	t.Run("SessionTimeline", func(t *testing.T) {
		entries := []*SessionTimelineEntry{
			{EntryID: uuid.New().String(), SessionID: "s1", FromState: orchestrator.SessionPhaseBootstrapping, ToState: orchestrator.SessionPhaseActive, Reason: "init", Timestamp: t2},
			{EntryID: uuid.New().String(), SessionID: "s1", FromState: orchestrator.SessionPhaseActive, ToState: orchestrator.SessionPhaseDegraded, Reason: "link_down", Timestamp: t3},
			{EntryID: uuid.New().String(), SessionID: "s1", FromState: orchestrator.SessionPhaseActive, ToState: orchestrator.SessionPhaseProtected, Reason: "threat", Timestamp: t1},
		}
		for _, e := range entries {
			if err := store.SaveSessionEntry(ctx, e); err != nil {
				t.Fatalf("SaveSessionEntry failed: %v", err)
			}
		}

		// List all — verify timestamp ASC
		list, err := store.ListSessionEntries(ctx, "s1", nil)
		if err != nil {
			t.Fatalf("ListSessionEntries failed: %v", err)
		}
		if len(list) != 3 {
			t.Fatalf("expected 3 entries, got %d", len(list))
		}
		if !list[0].Timestamp.Before(list[1].Timestamp) || !list[1].Timestamp.Before(list[2].Timestamp) {
			t.Fatal("entries not in timestamp ASC order")
		}

		// List with time range filter
		rangeStart := t2.Add(-30 * time.Minute)
		rangeEnd := t2.Add(30 * time.Minute)
		filtered, err := store.ListSessionEntries(ctx, "s1", &TimeRange{Start: &rangeStart, End: &rangeEnd})
		if err != nil {
			t.Fatalf("ListSessionEntries with range failed: %v", err)
		}
		if len(filtered) != 1 {
			t.Fatalf("expected 1 filtered entry, got %d", len(filtered))
		}
	})

	// --- Link Health Timeline ---
	t.Run("LinkHealthTimeline", func(t *testing.T) {
		entries := []*LinkHealthTimelineEntry{
			{EntryID: uuid.New().String(), LinkID: "l1", HealthScore: 95.0, RTTMs: 10, LossRate: 0.01, JitterMs: 2, Phase: orchestrator.LinkPhaseActive, EventType: "health_update", Timestamp: t1},
			{EntryID: uuid.New().String(), LinkID: "l1", HealthScore: 70.0, RTTMs: 50, LossRate: 0.05, JitterMs: 10, Phase: orchestrator.LinkPhaseDegrading, EventType: "phase_transition", Timestamp: t2},
			{EntryID: uuid.New().String(), LinkID: "l1", HealthScore: 30.0, RTTMs: 200, LossRate: 0.20, JitterMs: 50, Phase: orchestrator.LinkPhaseUnavailable, EventType: "phase_transition", Timestamp: t3},
		}
		for _, e := range entries {
			if err := store.SaveLinkHealthEntry(ctx, e); err != nil {
				t.Fatalf("SaveLinkHealthEntry failed: %v", err)
			}
		}

		list, err := store.ListLinkHealthEntries(ctx, "l1", nil)
		if err != nil {
			t.Fatalf("ListLinkHealthEntries failed: %v", err)
		}
		if len(list) != 3 {
			t.Fatalf("expected 3 entries, got %d", len(list))
		}
		if !list[0].Timestamp.Before(list[1].Timestamp) || !list[1].Timestamp.Before(list[2].Timestamp) {
			t.Fatal("entries not in timestamp ASC order")
		}
	})

	// --- Persona Version Timeline ---
	t.Run("PersonaVersionTimeline", func(t *testing.T) {
		entries := []*PersonaVersionTimelineEntry{
			{EntryID: uuid.New().String(), SessionID: "s1", PersonaID: "p1", FromVersion: 1, ToVersion: 2, EventType: "switch", Timestamp: t1},
			{EntryID: uuid.New().String(), SessionID: "s1", PersonaID: "p1", FromVersion: 2, ToVersion: 3, EventType: "switch", Timestamp: t2},
			{EntryID: uuid.New().String(), SessionID: "s1", PersonaID: "p1", FromVersion: 3, ToVersion: 2, EventType: "rollback", Timestamp: t3},
		}
		for _, e := range entries {
			if err := store.SavePersonaVersionEntry(ctx, e); err != nil {
				t.Fatalf("SavePersonaVersionEntry failed: %v", err)
			}
		}

		list, err := store.ListPersonaVersionEntries(ctx, "s1", nil)
		if err != nil {
			t.Fatalf("ListPersonaVersionEntries failed: %v", err)
		}
		if len(list) != 3 {
			t.Fatalf("expected 3 entries, got %d", len(list))
		}
		if !list[0].Timestamp.Before(list[1].Timestamp) || !list[1].Timestamp.Before(list[2].Timestamp) {
			t.Fatal("entries not in timestamp ASC order")
		}

		// Also test ListPersonaVersionEntriesByPersona
		byPersona, err := store.ListPersonaVersionEntriesByPersona(ctx, "p1", nil)
		if err != nil {
			t.Fatalf("ListPersonaVersionEntriesByPersona failed: %v", err)
		}
		if len(byPersona) != 3 {
			t.Fatalf("expected 3 entries by persona, got %d", len(byPersona))
		}
	})

	// --- Survival Mode Timeline ---
	t.Run("SurvivalModeTimeline", func(t *testing.T) {
		entries := []*SurvivalModeTimelineEntry{
			{EntryID: uuid.New().String(), FromMode: orchestrator.SurvivalModeNormal, ToMode: orchestrator.SurvivalModeLowNoise, Triggers: json.RawMessage(`["threat_detected"]`), TxID: "tx-1", Timestamp: t1},
			{EntryID: uuid.New().String(), FromMode: orchestrator.SurvivalModeLowNoise, ToMode: orchestrator.SurvivalModeHardened, Triggers: json.RawMessage(`["escalation"]`), TxID: "tx-2", Timestamp: t2},
			{EntryID: uuid.New().String(), FromMode: orchestrator.SurvivalModeHardened, ToMode: orchestrator.SurvivalModeNormal, Triggers: json.RawMessage(`["all_clear"]`), TxID: "tx-3", Timestamp: t3},
		}
		for _, e := range entries {
			if err := store.SaveSurvivalModeEntry(ctx, e); err != nil {
				t.Fatalf("SaveSurvivalModeEntry failed: %v", err)
			}
		}

		list, err := store.ListSurvivalModeEntries(ctx, nil)
		if err != nil {
			t.Fatalf("ListSurvivalModeEntries failed: %v", err)
		}
		if len(list) != 3 {
			t.Fatalf("expected 3 entries, got %d", len(list))
		}
		if !list[0].Timestamp.Before(list[1].Timestamp) || !list[1].Timestamp.Before(list[2].Timestamp) {
			t.Fatal("entries not in timestamp ASC order")
		}

		// Time range filter
		rangeStart := t2.Add(-30 * time.Minute)
		rangeEnd := t2.Add(30 * time.Minute)
		filtered, err := store.ListSurvivalModeEntries(ctx, &TimeRange{Start: &rangeStart, End: &rangeEnd})
		if err != nil {
			t.Fatalf("ListSurvivalModeEntries with range failed: %v", err)
		}
		if len(filtered) != 1 {
			t.Fatalf("expected 1 filtered entry, got %d", len(filtered))
		}
	})

	// --- Transaction Timeline ---
	t.Run("TransactionTimeline", func(t *testing.T) {
		entries := []*TransactionTimelineEntry{
			{EntryID: uuid.New().String(), TxID: "tx-x", FromPhase: commit.TxPhasePreparing, ToPhase: commit.TxPhaseValidating, PhaseData: json.RawMessage(`{"step":"prepare"}`), Timestamp: t1},
			{EntryID: uuid.New().String(), TxID: "tx-x", FromPhase: commit.TxPhaseValidating, ToPhase: commit.TxPhaseShadowWriting, PhaseData: json.RawMessage(`{"step":"validate"}`), Timestamp: t2},
			{EntryID: uuid.New().String(), TxID: "tx-x", FromPhase: commit.TxPhaseShadowWriting, ToPhase: commit.TxPhaseCommitted, PhaseData: json.RawMessage(`{"step":"shadow"}`), Timestamp: t3},
		}
		for _, e := range entries {
			if err := store.SaveTransactionEntry(ctx, e); err != nil {
				t.Fatalf("SaveTransactionEntry failed: %v", err)
			}
		}

		list, err := store.ListTransactionEntries(ctx, "tx-x")
		if err != nil {
			t.Fatalf("ListTransactionEntries failed: %v", err)
		}
		if len(list) != 3 {
			t.Fatalf("expected 3 entries, got %d", len(list))
		}
		if !list[0].Timestamp.Before(list[1].Timestamp) || !list[1].Timestamp.Before(list[2].Timestamp) {
			t.Fatal("entries not in timestamp ASC order")
		}
	})

	// --- Cleanup ---
	t.Run("Cleanup", func(t *testing.T) {
		// Set all timestamps to 60 days ago
		oldTime := now.AddDate(0, 0, -60)
		db.Model(&SessionTimelineEntry{}).Where("1=1").Update("timestamp", oldTime)
		db.Model(&LinkHealthTimelineEntry{}).Where("1=1").Update("timestamp", oldTime)
		db.Model(&PersonaVersionTimelineEntry{}).Where("1=1").Update("timestamp", oldTime)
		db.Model(&SurvivalModeTimelineEntry{}).Where("1=1").Update("timestamp", oldTime)
		db.Model(&TransactionTimelineEntry{}).Where("1=1").Update("timestamp", oldTime)

		deleted, err := store.Cleanup(ctx, 30)
		if err != nil {
			t.Fatalf("Cleanup failed: %v", err)
		}
		// 3 session + 3 link + 3 persona + 3 survival + 3 tx = 15
		if deleted != 15 {
			t.Fatalf("expected 15 deleted, got %d", deleted)
		}

		// Verify all tables are empty
		list1, _ := store.ListSessionEntries(ctx, "s1", nil)
		list2, _ := store.ListLinkHealthEntries(ctx, "l1", nil)
		list3, _ := store.ListPersonaVersionEntries(ctx, "s1", nil)
		list4, _ := store.ListSurvivalModeEntries(ctx, nil)
		list5, _ := store.ListTransactionEntries(ctx, "tx-x")
		total := len(list1) + len(list2) + len(list3) + len(list4) + len(list5)
		if total != 0 {
			t.Fatalf("expected 0 remaining entries, got %d", total)
		}
	})
}

// =============================================================================
// 10.4 AuditCollector registers as EventHandler
// =============================================================================

func TestIntegration_AuditCollector_EventHandlerRegistration(t *testing.T) {
	db := setupTestDB(t)
	auditStore := NewGormAuditStore(db)
	txProvider := &mockTransactionProvider{tx: &commit.CommitTransaction{}}
	budgetProvider := &mockBudgetDecisionProvider{}

	// Create two AuditCollector instances for different event types
	rollbackCollector := NewAuditCollector(auditStore, txProvider, budgetProvider, events.EventRollbackDone)
	budgetCollector := NewAuditCollector(auditStore, txProvider, budgetProvider, events.EventBudgetReject)

	// Verify EventType() returns correct type
	if rollbackCollector.EventType() != events.EventRollbackDone {
		t.Fatalf("rollbackCollector EventType: got %s, want %s", rollbackCollector.EventType(), events.EventRollbackDone)
	}
	if budgetCollector.EventType() != events.EventBudgetReject {
		t.Fatalf("budgetCollector EventType: got %s, want %s", budgetCollector.EventType(), events.EventBudgetReject)
	}

	// Register both with EventRegistry
	registry := events.NewEventRegistry()

	if err := registry.Register(rollbackCollector); err != nil {
		t.Fatalf("Register rollbackCollector failed: %v", err)
	}
	if err := registry.Register(budgetCollector); err != nil {
		t.Fatalf("Register budgetCollector failed: %v", err)
	}

	// Verify both are registered
	if !registry.IsRegistered(events.EventRollbackDone) {
		t.Fatal("EventRollbackDone not registered")
	}
	if !registry.IsRegistered(events.EventBudgetReject) {
		t.Fatal("EventBudgetReject not registered")
	}

	// Verify GetHandler returns the correct handlers
	h1, err := registry.GetHandler(events.EventRollbackDone)
	if err != nil {
		t.Fatalf("GetHandler(EventRollbackDone) failed: %v", err)
	}
	if h1.EventType() != events.EventRollbackDone {
		t.Fatalf("handler EventType mismatch: got %s", h1.EventType())
	}

	h2, err := registry.GetHandler(events.EventBudgetReject)
	if err != nil {
		t.Fatalf("GetHandler(EventBudgetReject) failed: %v", err)
	}
	if h2.EventType() != events.EventBudgetReject {
		t.Fatalf("handler EventType mismatch: got %s", h2.EventType())
	}
}

// =============================================================================
// 10.5 Full audit flow end-to-end
// =============================================================================

func TestIntegration_FullAuditFlow_EndToEnd(t *testing.T) {
	db := setupTestDB(t)
	auditStore := NewGormAuditStore(db)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	finishedAt := now.Add(5 * time.Second)

	// Create a CommitTransaction in Committed state
	tx := &commit.CommitTransaction{
		TxID:            uuid.New().String(),
		TxType:          commit.TxTypePersonaSwitch,
		TxPhase:         commit.TxPhaseCommitted,
		TxScope:         commit.TxScopeSession,
		TargetSessionID: "session-123",
		TargetPersonaID: "persona-456",
		CreatedAt:       now,
		FinishedAt:      &finishedAt,
		PrepareState:    json.RawMessage(`{}`),
		ValidateState:   json.RawMessage(`{}`),
		ShadowState:     json.RawMessage(`{}`),
		FlipState:       json.RawMessage(`{}`),
		AckState:        json.RawMessage(`{}`),
		CommitState:     json.RawMessage(`{}`),
	}

	txProvider := &mockTransactionProvider{tx: tx}
	budgetProvider := &mockBudgetDecisionProvider{
		decision: &budget.BudgetDecision{
			Verdict: budget.VerdictAllow,
		},
	}

	collector := NewAuditCollector(auditStore, txProvider, budgetProvider, events.EventRollbackDone)

	// Call OnTransactionFinished
	if err := collector.OnTransactionFinished(ctx, tx); err != nil {
		t.Fatalf("OnTransactionFinished failed: %v", err)
	}

	// Query AuditStore.GetByTxID
	record, err := auditStore.GetByTxID(ctx, tx.TxID)
	if err != nil {
		t.Fatalf("GetByTxID failed: %v", err)
	}

	// Verify all fields match
	if record.TxID != tx.TxID {
		t.Errorf("TxID mismatch: got %s, want %s", record.TxID, tx.TxID)
	}
	if record.TxType != tx.TxType {
		t.Errorf("TxType mismatch: got %s, want %s", record.TxType, tx.TxType)
	}
	if !record.InitiatedAt.Equal(tx.CreatedAt) {
		t.Errorf("InitiatedAt mismatch: got %v, want %v", record.InitiatedAt, tx.CreatedAt)
	}
	if !record.FinishedAt.Equal(*tx.FinishedAt) {
		t.Errorf("FinishedAt mismatch: got %v, want %v", record.FinishedAt, *tx.FinishedAt)
	}
	if !record.FlipSuccess {
		t.Error("FlipSuccess should be true for Committed")
	}
	if record.RollbackTriggered {
		t.Error("RollbackTriggered should be false for Committed")
	}
	if record.BudgetVerdict != string(budget.VerdictAllow) {
		t.Errorf("BudgetVerdict mismatch: got %s, want %s", record.BudgetVerdict, budget.VerdictAllow)
	}
	if record.DenyReason != "" {
		t.Errorf("DenyReason should be empty for allow verdict, got %s", record.DenyReason)
	}

	// Also test RolledBack flow
	tx2 := &commit.CommitTransaction{
		TxID:          uuid.New().String(),
		TxType:        commit.TxTypeLinkMigration,
		TxPhase:       commit.TxPhaseRolledBack,
		TxScope:       commit.TxScopeLink,
		CreatedAt:     now,
		FinishedAt:    &finishedAt,
		PrepareState:  json.RawMessage(`{}`),
		ValidateState: json.RawMessage(`{}`),
		ShadowState:   json.RawMessage(`{}`),
		FlipState:     json.RawMessage(`{}`),
		AckState:      json.RawMessage(`{}`),
		CommitState:   json.RawMessage(`{}`),
	}

	budgetProvider2 := &mockBudgetDecisionProvider{
		decision: &budget.BudgetDecision{
			Verdict:    budget.VerdictDenyAndHold,
			DenyReason: "over_budget",
		},
	}
	collector2 := NewAuditCollector(auditStore, &mockTransactionProvider{tx: tx2}, budgetProvider2, events.EventRollbackDone)

	if err := collector2.OnTransactionFinished(ctx, tx2); err != nil {
		t.Fatalf("OnTransactionFinished (RolledBack) failed: %v", err)
	}

	record2, err := auditStore.GetByTxID(ctx, tx2.TxID)
	if err != nil {
		t.Fatalf("GetByTxID (RolledBack) failed: %v", err)
	}
	if record2.FlipSuccess {
		t.Error("FlipSuccess should be false for RolledBack")
	}
	if !record2.RollbackTriggered {
		t.Error("RollbackTriggered should be true for RolledBack")
	}
	if record2.BudgetVerdict != string(budget.VerdictDenyAndHold) {
		t.Errorf("BudgetVerdict mismatch: got %s", record2.BudgetVerdict)
	}
	if record2.DenyReason != "over_budget" {
		t.Errorf("DenyReason mismatch: got %s, want over_budget", record2.DenyReason)
	}
}

// =============================================================================
// 10.6 Full timeline flow end-to-end
// =============================================================================

func TestIntegration_FullTimelineFlow_EndToEnd(t *testing.T) {
	db := setupTestDB(t)
	timelineStore := NewGormTimelineStore(db)
	collector := NewTimelineCollector(timelineStore)
	ctx := context.Background()

	// 1. Session transition flow
	t.Run("SessionTransition", func(t *testing.T) {
		err := collector.OnSessionTransition(ctx, "session-abc",
			orchestrator.SessionPhaseBootstrapping, orchestrator.SessionPhaseActive,
			"initialization_complete", "link-1", "persona-1", orchestrator.SurvivalModeNormal)
		if err != nil {
			t.Fatalf("OnSessionTransition failed: %v", err)
		}

		entries, err := timelineStore.ListSessionEntries(ctx, "session-abc", nil)
		if err != nil {
			t.Fatalf("ListSessionEntries failed: %v", err)
		}
		if len(entries) != 1 {
			t.Fatalf("expected 1 entry, got %d", len(entries))
		}

		e := entries[0]
		if e.SessionID != "session-abc" {
			t.Errorf("SessionID mismatch: got %s", e.SessionID)
		}
		if e.FromState != orchestrator.SessionPhaseBootstrapping {
			t.Errorf("FromState mismatch: got %s", e.FromState)
		}
		if e.ToState != orchestrator.SessionPhaseActive {
			t.Errorf("ToState mismatch: got %s", e.ToState)
		}
		if e.Reason != "initialization_complete" {
			t.Errorf("Reason mismatch: got %s", e.Reason)
		}
		if e.LinkID != "link-1" {
			t.Errorf("LinkID mismatch: got %s", e.LinkID)
		}
		if e.PersonaID != "persona-1" {
			t.Errorf("PersonaID mismatch: got %s", e.PersonaID)
		}
		if e.SurvivalMode != orchestrator.SurvivalModeNormal {
			t.Errorf("SurvivalMode mismatch: got %s", e.SurvivalMode)
		}
		if e.Timestamp.IsZero() {
			t.Error("Timestamp is zero")
		}
		if e.EntryID == "" {
			t.Error("EntryID is empty")
		}
	})

	// 2. Link health update flow
	t.Run("LinkHealthUpdate", func(t *testing.T) {
		err := collector.OnLinkHealthUpdate(ctx, "link-xyz",
			85.5, 25, 0.02, 5, orchestrator.LinkPhaseActive)
		if err != nil {
			t.Fatalf("OnLinkHealthUpdate failed: %v", err)
		}

		entries, err := timelineStore.ListLinkHealthEntries(ctx, "link-xyz", nil)
		if err != nil {
			t.Fatalf("ListLinkHealthEntries failed: %v", err)
		}
		if len(entries) != 1 {
			t.Fatalf("expected 1 entry, got %d", len(entries))
		}

		e := entries[0]
		if e.LinkID != "link-xyz" {
			t.Errorf("LinkID mismatch: got %s", e.LinkID)
		}
		if e.HealthScore != 85.5 {
			t.Errorf("HealthScore mismatch: got %f", e.HealthScore)
		}
		if e.RTTMs != 25 {
			t.Errorf("RTTMs mismatch: got %d", e.RTTMs)
		}
		if e.LossRate != 0.02 {
			t.Errorf("LossRate mismatch: got %f", e.LossRate)
		}
		if e.JitterMs != 5 {
			t.Errorf("JitterMs mismatch: got %d", e.JitterMs)
		}
		if e.Phase != orchestrator.LinkPhaseActive {
			t.Errorf("Phase mismatch: got %s", e.Phase)
		}
		if e.EventType != "health_update" {
			t.Errorf("EventType mismatch: got %s", e.EventType)
		}
	})

	// 3. Survival mode transition flow
	t.Run("SurvivalModeTransition", func(t *testing.T) {
		triggers := json.RawMessage(`["high_threat_level"]`)
		err := collector.OnModeTransition(ctx,
			orchestrator.SurvivalModeNormal, orchestrator.SurvivalModeHardened,
			triggers, "tx-mode-1")
		if err != nil {
			t.Fatalf("OnModeTransition failed: %v", err)
		}

		entries, err := timelineStore.ListSurvivalModeEntries(ctx, nil)
		if err != nil {
			t.Fatalf("ListSurvivalModeEntries failed: %v", err)
		}
		if len(entries) != 1 {
			t.Fatalf("expected 1 entry, got %d", len(entries))
		}

		e := entries[0]
		if e.FromMode != orchestrator.SurvivalModeNormal {
			t.Errorf("FromMode mismatch: got %s", e.FromMode)
		}
		if e.ToMode != orchestrator.SurvivalModeHardened {
			t.Errorf("ToMode mismatch: got %s", e.ToMode)
		}
		if e.TxID != "tx-mode-1" {
			t.Errorf("TxID mismatch: got %s", e.TxID)
		}
	})
}

// =============================================================================
// 10.7 Diagnostic view API endpoint request/response verification
// =============================================================================

func TestIntegration_DiagnosticAggregator_Views(t *testing.T) {
	ctx := context.Background()

	// Set up mock providers with known data
	session := &orchestrator.SessionState{
		SessionID:           "sess-diag-1",
		UserID:              "user-1",
		ClientID:            "client-1",
		GatewayID:           "gw-1",
		CurrentPersonaID:    "persona-diag-1",
		CurrentLinkID:       "link-diag-1",
		CurrentSurvivalMode: orchestrator.SurvivalModeHardened,
		State:               orchestrator.SessionPhaseActive,
	}

	link := &orchestrator.LinkState{
		LinkID:           "link-diag-1",
		Phase:            orchestrator.LinkPhaseActive,
		LastSwitchReason: "probing_complete",
	}

	control := &orchestrator.ControlState{
		GatewayID:        "gw-1",
		PersonaVersion:   42,
		LastSwitchReason: "rollback_reason_xyz",
	}

	activeTx := &commit.CommitTransaction{
		TxID:          "tx-diag-1",
		TxType:        commit.TxTypeSurvivalModeSwitch,
		TxPhase:       commit.TxPhaseFlipping,
		TxScope:       commit.TxScopeGlobal,
		CreatedAt:     time.Now().UTC().Add(-10 * time.Second),
		PrepareState:  json.RawMessage(`{}`),
		ValidateState: json.RawMessage(`{}`),
		ShadowState:   json.RawMessage(`{}`),
		FlipState:     json.RawMessage(`{}`),
		AckState:      json.RawMessage(`{}`),
		CommitState:   json.RawMessage(`{}`),
	}

	transitionTime := time.Now().UTC().Add(-5 * time.Minute)

	sessions := &mockSessionProvider{sessions: map[string]*orchestrator.SessionState{
		session.SessionID: session,
	}}
	links := &mockLinkProvider{links: map[string]*orchestrator.LinkState{
		link.LinkID: link,
	}}
	controls := &mockControlProvider{controls: map[string]*orchestrator.ControlState{
		control.GatewayID: control,
	}}
	txs := &mockTxProvider{
		txs:       map[string]*commit.CommitTransaction{activeTx.TxID: activeTx},
		activeTxs: []*commit.CommitTransaction{activeTx},
	}
	survProv := &mockSurvivalProvider{
		mode: orchestrator.SurvivalModeHardened,
		history: []*survival.TransitionRecord{
			{
				FromMode:  orchestrator.SurvivalModeNormal,
				ToMode:    orchestrator.SurvivalModeHardened,
				TxID:      "tx-mode-switch",
				Timestamp: transitionTime,
			},
		},
	}
	timeline := &mockTimelineStore{}

	agg := NewDiagnosticAggregator(sessions, links, controls, txs, survProv, timeline)

	// 1. GetSessionDiagnostic
	t.Run("GetSessionDiagnostic", func(t *testing.T) {
		diag, err := agg.GetSessionDiagnostic(ctx, "sess-diag-1")
		if err != nil {
			t.Fatalf("GetSessionDiagnostic failed: %v", err)
		}
		if diag.SessionID != "sess-diag-1" {
			t.Errorf("SessionID mismatch: got %s", diag.SessionID)
		}
		if diag.CurrentLinkID != "link-diag-1" {
			t.Errorf("CurrentLinkID mismatch: got %s", diag.CurrentLinkID)
		}
		if diag.CurrentLinkPhase != orchestrator.LinkPhaseActive {
			t.Errorf("CurrentLinkPhase mismatch: got %s", diag.CurrentLinkPhase)
		}
		if diag.CurrentPersonaID != "persona-diag-1" {
			t.Errorf("CurrentPersonaID mismatch: got %s", diag.CurrentPersonaID)
		}
		if diag.CurrentPersonaVersion != 42 {
			t.Errorf("CurrentPersonaVersion mismatch: got %d", diag.CurrentPersonaVersion)
		}
		if diag.CurrentSurvivalMode != orchestrator.SurvivalModeHardened {
			t.Errorf("CurrentSurvivalMode mismatch: got %s", diag.CurrentSurvivalMode)
		}
		if diag.SessionState != orchestrator.SessionPhaseActive {
			t.Errorf("SessionState mismatch: got %s", diag.SessionState)
		}
		if diag.LastSwitchReason != "probing_complete" {
			t.Errorf("LastSwitchReason mismatch: got %s", diag.LastSwitchReason)
		}
		if diag.LastRollbackReason != "rollback_reason_xyz" {
			t.Errorf("LastRollbackReason mismatch: got %s", diag.LastRollbackReason)
		}
	})

	// 2. GetSystemDiagnostic
	t.Run("GetSystemDiagnostic", func(t *testing.T) {
		diag, err := agg.GetSystemDiagnostic(ctx)
		if err != nil {
			t.Fatalf("GetSystemDiagnostic failed: %v", err)
		}
		if diag.CurrentSurvivalMode != orchestrator.SurvivalModeHardened {
			t.Errorf("CurrentSurvivalMode mismatch: got %s", diag.CurrentSurvivalMode)
		}
		if diag.LastModeSwitchReason != "tx-mode-switch" {
			t.Errorf("LastModeSwitchReason mismatch: got %s", diag.LastModeSwitchReason)
		}
		if diag.LastModeSwitchTime == nil {
			t.Fatal("LastModeSwitchTime is nil")
		}
		if diag.ActiveSessionCount != 1 {
			t.Errorf("ActiveSessionCount mismatch: got %d, want 1", diag.ActiveSessionCount)
		}
		if diag.ActiveLinkCount != 1 {
			t.Errorf("ActiveLinkCount mismatch: got %d, want 1", diag.ActiveLinkCount)
		}
		if diag.ActiveTransaction == nil {
			t.Fatal("ActiveTransaction is nil")
		}
		if diag.ActiveTransaction.TxID != "tx-diag-1" {
			t.Errorf("ActiveTransaction.TxID mismatch: got %s", diag.ActiveTransaction.TxID)
		}
		if diag.ActiveTransaction.TxType != commit.TxTypeSurvivalModeSwitch {
			t.Errorf("ActiveTransaction.TxType mismatch: got %s", diag.ActiveTransaction.TxType)
		}
		if diag.ActiveTransaction.TxPhase != commit.TxPhaseFlipping {
			t.Errorf("ActiveTransaction.TxPhase mismatch: got %s", diag.ActiveTransaction.TxPhase)
		}
	})

	// 3. GetTransactionDiagnostic
	t.Run("GetTransactionDiagnostic", func(t *testing.T) {
		diag, err := agg.GetTransactionDiagnostic(ctx, "tx-diag-1")
		if err != nil {
			t.Fatalf("GetTransactionDiagnostic failed: %v", err)
		}
		if diag.TxID != "tx-diag-1" {
			t.Errorf("TxID mismatch: got %s", diag.TxID)
		}
		if diag.TxType != commit.TxTypeSurvivalModeSwitch {
			t.Errorf("TxType mismatch: got %s", diag.TxType)
		}
		if diag.CurrentPhase != commit.TxPhaseFlipping {
			t.Errorf("CurrentPhase mismatch: got %s", diag.CurrentPhase)
		}
		// Non-terminal phase → stuck_duration > 0
		if diag.StuckDuration <= 0 {
			t.Errorf("StuckDuration should be > 0 for non-terminal phase, got %v", diag.StuckDuration)
		}
	})

	// 4. Not-found cases
	t.Run("SessionNotFound", func(t *testing.T) {
		_, err := agg.GetSessionDiagnostic(ctx, "nonexistent-session")
		if err == nil {
			t.Fatal("expected error for nonexistent session")
		}
		if _, ok := err.(*ErrSessionNotFound); !ok {
			t.Fatalf("expected ErrSessionNotFound, got %T: %v", err, err)
		}
	})

	t.Run("TransactionNotFound", func(t *testing.T) {
		_, err := agg.GetTransactionDiagnostic(ctx, "nonexistent-tx")
		if err == nil {
			t.Fatal("expected error for nonexistent transaction")
		}
		if _, ok := err.(*ErrTransactionNotFound); !ok {
			t.Fatalf("expected ErrTransactionNotFound, got %T: %v", err, err)
		}
	})
}
