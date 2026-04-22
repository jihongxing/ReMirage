package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"pgregory.net/rapid"

	"mirage-gateway/pkg/orchestrator"
	"mirage-gateway/pkg/orchestrator/budget"
	"mirage-gateway/pkg/orchestrator/commit"
	"mirage-gateway/pkg/orchestrator/events"
	"mirage-gateway/pkg/orchestrator/survival"
)

// --- Mock implementations ---

// mockAuditStore captures the last saved record for assertion
type mockAuditStore struct {
	lastRecord *AuditRecord
}

func (m *mockAuditStore) Save(_ context.Context, record *AuditRecord) error {
	m.lastRecord = record
	return nil
}

func (m *mockAuditStore) GetByTxID(_ context.Context, _ string) (*AuditRecord, error) {
	return m.lastRecord, nil
}

func (m *mockAuditStore) List(_ context.Context, _ *AuditFilter) ([]*AuditRecord, error) {
	return nil, nil
}

func (m *mockAuditStore) Cleanup(_ context.Context, _ int) (int64, error) {
	return 0, nil
}

// mockTransactionProvider returns a pre-configured transaction
type mockTransactionProvider struct {
	tx *commit.CommitTransaction
}

func (m *mockTransactionProvider) GetTransaction(_ context.Context, _ string) (*commit.CommitTransaction, error) {
	return m.tx, nil
}

// mockBudgetDecisionProvider returns a pre-configured decision
type mockBudgetDecisionProvider struct {
	decision *budget.BudgetDecision
	err      error
}

func (m *mockBudgetDecisionProvider) GetLastDecision(_ context.Context, _ string) (*budget.BudgetDecision, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.decision, nil
}

// --- Rapid generators ---

var terminalPhases = []commit.TxPhase{
	commit.TxPhaseCommitted,
	commit.TxPhaseRolledBack,
	commit.TxPhaseFailed,
}

var allTxTypes = []commit.TxType{
	commit.TxTypePersonaSwitch,
	commit.TxTypeLinkMigration,
	commit.TxTypeGatewayReassignment,
	commit.TxTypeSurvivalModeSwitch,
}

var allVerdicts = []budget.BudgetVerdict{
	budget.VerdictAllow,
	budget.VerdictAllowDegraded,
	budget.VerdictAllowWithCharge,
	budget.VerdictDenyAndHold,
	budget.VerdictDenyAndSuspend,
}

func genTerminalTransaction(t *rapid.T) *commit.CommitTransaction {
	phase := rapid.SampledFrom(terminalPhases).Draw(t, "tx_phase")
	txType := rapid.SampledFrom(allTxTypes).Draw(t, "tx_type")
	createdAt := time.Now().UTC().Add(-time.Duration(rapid.IntRange(1, 3600).Draw(t, "age_seconds")) * time.Second)
	finishedAt := createdAt.Add(time.Duration(rapid.IntRange(1, 600).Draw(t, "duration_seconds")) * time.Second)

	tx := &commit.CommitTransaction{
		TxID:          uuid.New().String(),
		TxType:        txType,
		TxPhase:       phase,
		CreatedAt:     createdAt,
		FinishedAt:    &finishedAt,
		PrepareState:  json.RawMessage(`{}`),
		ValidateState: json.RawMessage(`{}`),
		ShadowState:   json.RawMessage(`{}`),
		FlipState:     json.RawMessage(`{}`),
		AckState:      json.RawMessage(`{}`),
		CommitState:   json.RawMessage(`{}`),
	}
	return tx
}

func genBudgetDecision(t *rapid.T) *budget.BudgetDecision {
	verdict := rapid.SampledFrom(allVerdicts).Draw(t, "verdict")
	decision := &budget.BudgetDecision{
		Verdict: verdict,
	}
	if verdict == budget.VerdictDenyAndHold || verdict == budget.VerdictDenyAndSuspend {
		decision.DenyReason = rapid.StringMatching(`[a-z_]{5,30}`).Draw(t, "deny_reason")
	}
	return decision
}

// --- Property test ---

// TestProperty1_AuditRecordFieldDerivation verifies AuditCollector generates correct AuditRecord fields.
//
// Feature: v2-observability, Property 1: AuditRecord field derivation correctness
// **Validates: Requirements 1.1, 1.2, 1.3, 1.4, 1.5, 1.6, 1.8**
func TestProperty1_AuditRecordFieldDerivation(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		tx := genTerminalTransaction(t)
		decision := genBudgetDecision(t)

		store := &mockAuditStore{}
		budgetProvider := &mockBudgetDecisionProvider{decision: decision}
		txProvider := &mockTransactionProvider{tx: tx}

		collector := NewAuditCollector(store, txProvider, budgetProvider, events.EventRollbackDone)

		err := collector.OnTransactionFinished(context.Background(), tx)
		if err != nil {
			t.Fatalf("OnTransactionFinished returned error: %v", err)
		}

		record := store.lastRecord
		if record == nil {
			t.Fatal("no AuditRecord was saved")
		}

		// (1) audit_id is valid UUID v4 and non-empty
		if record.AuditID == "" {
			t.Fatal("audit_id is empty")
		}
		parsed, err := uuid.Parse(record.AuditID)
		if err != nil {
			t.Fatalf("audit_id is not valid UUID: %v", err)
		}
		if parsed.Version() != 4 {
			t.Fatalf("audit_id is not UUID v4, got version %d", parsed.Version())
		}

		// (2) tx_id equals transaction's tx_id
		if record.TxID != tx.TxID {
			t.Fatalf("tx_id mismatch: got %s, want %s", record.TxID, tx.TxID)
		}

		// (3) tx_type equals transaction's tx_type
		if record.TxType != tx.TxType {
			t.Fatalf("tx_type mismatch: got %s, want %s", record.TxType, tx.TxType)
		}

		// (4) initiated_at equals transaction's created_at
		if !record.InitiatedAt.Equal(tx.CreatedAt) {
			t.Fatalf("initiated_at mismatch: got %v, want %v", record.InitiatedAt, tx.CreatedAt)
		}

		// (5) finished_at equals transaction's finished_at
		if tx.FinishedAt != nil && !record.FinishedAt.Equal(*tx.FinishedAt) {
			t.Fatalf("finished_at mismatch: got %v, want %v", record.FinishedAt, *tx.FinishedAt)
		}

		// (6) Committed → flip_success=true, rollback_triggered=false
		if tx.TxPhase == commit.TxPhaseCommitted {
			if !record.FlipSuccess {
				t.Fatal("Committed: flip_success should be true")
			}
			if record.RollbackTriggered {
				t.Fatal("Committed: rollback_triggered should be false")
			}
		}

		// (7) RolledBack → flip_success=false, rollback_triggered=true
		if tx.TxPhase == commit.TxPhaseRolledBack {
			if record.FlipSuccess {
				t.Fatal("RolledBack: flip_success should be false")
			}
			if !record.RollbackTriggered {
				t.Fatal("RolledBack: rollback_triggered should be true")
			}
		}

		// (8) Failed → flip_success=false, rollback_triggered=false
		if tx.TxPhase == commit.TxPhaseFailed {
			if record.FlipSuccess {
				t.Fatal("Failed: flip_success should be false")
			}
			if record.RollbackTriggered {
				t.Fatal("Failed: rollback_triggered should be false")
			}
		}

		// (9) deny verdicts → deny_reason non-empty
		if decision.Verdict == budget.VerdictDenyAndHold || decision.Verdict == budget.VerdictDenyAndSuspend {
			if strings.TrimSpace(record.DenyReason) == "" {
				t.Fatalf("deny verdict %s: deny_reason should be non-empty", decision.Verdict)
			}
		}

		// (10) allow verdicts → deny_reason empty
		if decision.Verdict == budget.VerdictAllow ||
			decision.Verdict == budget.VerdictAllowDegraded ||
			decision.Verdict == budget.VerdictAllowWithCharge {
			if record.DenyReason != "" {
				t.Fatalf("allow verdict %s: deny_reason should be empty, got %q", decision.Verdict, record.DenyReason)
			}
		}
	})
}

// --- Mock TimelineStore ---

type mockTimelineStore struct {
	lastSession  *SessionTimelineEntry
	lastLink     *LinkHealthTimelineEntry
	lastPersona  *PersonaVersionTimelineEntry
	lastSurvival *SurvivalModeTimelineEntry
	lastTx       *TransactionTimelineEntry
}

func (m *mockTimelineStore) SaveSessionEntry(_ context.Context, entry *SessionTimelineEntry) error {
	m.lastSession = entry
	return nil
}

func (m *mockTimelineStore) ListSessionEntries(_ context.Context, _ string, _ *TimeRange) ([]*SessionTimelineEntry, error) {
	return nil, nil
}

func (m *mockTimelineStore) SaveLinkHealthEntry(_ context.Context, entry *LinkHealthTimelineEntry) error {
	m.lastLink = entry
	return nil
}

func (m *mockTimelineStore) ListLinkHealthEntries(_ context.Context, _ string, _ *TimeRange) ([]*LinkHealthTimelineEntry, error) {
	return nil, nil
}

func (m *mockTimelineStore) SavePersonaVersionEntry(_ context.Context, entry *PersonaVersionTimelineEntry) error {
	m.lastPersona = entry
	return nil
}

func (m *mockTimelineStore) ListPersonaVersionEntries(_ context.Context, _ string, _ *TimeRange) ([]*PersonaVersionTimelineEntry, error) {
	return nil, nil
}

func (m *mockTimelineStore) ListPersonaVersionEntriesByPersona(_ context.Context, _ string, _ *TimeRange) ([]*PersonaVersionTimelineEntry, error) {
	return nil, nil
}

func (m *mockTimelineStore) SaveSurvivalModeEntry(_ context.Context, entry *SurvivalModeTimelineEntry) error {
	m.lastSurvival = entry
	return nil
}

func (m *mockTimelineStore) ListSurvivalModeEntries(_ context.Context, _ *TimeRange) ([]*SurvivalModeTimelineEntry, error) {
	return nil, nil
}

func (m *mockTimelineStore) SaveTransactionEntry(_ context.Context, entry *TransactionTimelineEntry) error {
	m.lastTx = entry
	return nil
}

func (m *mockTimelineStore) ListTransactionEntries(_ context.Context, _ string) ([]*TransactionTimelineEntry, error) {
	return nil, nil
}

func (m *mockTimelineStore) Cleanup(_ context.Context, _ int) (int64, error) {
	return 0, nil
}

// --- Rapid generators for TimelineCollector ---

var allSurvivalModes = []orchestrator.SurvivalMode{
	orchestrator.SurvivalModeNormal, orchestrator.SurvivalModeLowNoise,
	orchestrator.SurvivalModeHardened, orchestrator.SurvivalModeDegraded,
	orchestrator.SurvivalModeEscape, orchestrator.SurvivalModeLastResort,
}

// --- helper: validate UUID v4 ---

func assertValidUUIDv4(t *rapid.T, id string, label string) {
	if id == "" {
		t.Fatalf("%s is empty", label)
	}
	parsed, err := uuid.Parse(id)
	if err != nil {
		t.Fatalf("%s is not valid UUID: %v", label, err)
	}
	if parsed.Version() != 4 {
		t.Fatalf("%s is not UUID v4, got version %d", label, parsed.Version())
	}
}

// --- Property 2: Session timeline entry generation completeness ---

// TestProperty2_SessionTimelineEntryCompleteness verifies TimelineCollector generates correct SessionTimelineEntry fields.
//
// Feature: v2-observability, Property 2: Session timeline entry generation completeness
// **Validates: Requirements 2.1, 2.2**
func TestProperty2_SessionTimelineEntryCompleteness(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		store := &mockTimelineStore{}
		collector := NewTimelineCollector(store)

		sessionID := rapid.StringMatching(`[a-z0-9]{8,32}`).Draw(t, "session_id")
		from := rapid.SampledFrom(orchestrator.AllSessionPhases).Draw(t, "from_state")
		to := rapid.SampledFrom(orchestrator.AllSessionPhases).Draw(t, "to_state")
		reason := rapid.StringMatching(`[a-z_]{0,30}`).Draw(t, "reason")
		linkID := rapid.StringMatching(`[a-z0-9]{8,32}`).Draw(t, "link_id")
		personaID := rapid.StringMatching(`[a-z0-9]{8,32}`).Draw(t, "persona_id")
		mode := rapid.SampledFrom(allSurvivalModes).Draw(t, "survival_mode")

		err := collector.OnSessionTransition(context.Background(), sessionID, from, to, reason, linkID, personaID, mode)
		if err != nil {
			t.Fatalf("OnSessionTransition returned error: %v", err)
		}

		entry := store.lastSession
		if entry == nil {
			t.Fatal("no SessionTimelineEntry was saved")
		}

		assertValidUUIDv4(t, entry.EntryID, "entry_id")

		if entry.SessionID != sessionID {
			t.Fatalf("session_id mismatch: got %s, want %s", entry.SessionID, sessionID)
		}
		if entry.FromState != from {
			t.Fatalf("from_state mismatch: got %s, want %s", entry.FromState, from)
		}
		if entry.ToState != to {
			t.Fatalf("to_state mismatch: got %s, want %s", entry.ToState, to)
		}
		if entry.Reason != reason {
			t.Fatalf("reason mismatch: got %s, want %s", entry.Reason, reason)
		}
		if entry.LinkID != linkID {
			t.Fatalf("link_id mismatch: got %s, want %s", entry.LinkID, linkID)
		}
		if entry.PersonaID != personaID {
			t.Fatalf("persona_id mismatch: got %s, want %s", entry.PersonaID, personaID)
		}
		if entry.SurvivalMode != mode {
			t.Fatalf("survival_mode mismatch: got %s, want %s", entry.SurvivalMode, mode)
		}
		if entry.Timestamp.IsZero() {
			t.Fatal("timestamp is zero")
		}
	})
}

// --- Property 3: Link health timeline entry generation completeness ---

// TestProperty3_LinkHealthTimelineEntryCompleteness verifies TimelineCollector generates correct LinkHealthTimelineEntry fields.
//
// Feature: v2-observability, Property 3: Link health timeline entry generation completeness
// **Validates: Requirements 3.1, 3.2, 3.3**
func TestProperty3_LinkHealthTimelineEntryCompleteness(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		store := &mockTimelineStore{}
		collector := NewTimelineCollector(store)

		linkID := rapid.StringMatching(`[a-z0-9]{8,32}`).Draw(t, "link_id")
		score := rapid.Float64Range(0, 100).Draw(t, "health_score")
		rttMs := rapid.Int64Range(0, 10000).Draw(t, "rtt_ms")
		lossRate := rapid.Float64Range(0, 1).Draw(t, "loss_rate")
		jitterMs := rapid.Int64Range(0, 5000).Draw(t, "jitter_ms")
		phase := rapid.SampledFrom(orchestrator.AllLinkPhases).Draw(t, "phase")

		// Test OnLinkHealthUpdate → event_type = "health_update"
		err := collector.OnLinkHealthUpdate(context.Background(), linkID, score, rttMs, lossRate, jitterMs, phase)
		if err != nil {
			t.Fatalf("OnLinkHealthUpdate returned error: %v", err)
		}

		entry := store.lastLink
		if entry == nil {
			t.Fatal("no LinkHealthTimelineEntry was saved for health_update")
		}

		assertValidUUIDv4(t, entry.EntryID, "entry_id (health_update)")

		if entry.LinkID != linkID {
			t.Fatalf("link_id mismatch: got %s, want %s", entry.LinkID, linkID)
		}
		if entry.HealthScore != score {
			t.Fatalf("health_score mismatch: got %f, want %f", entry.HealthScore, score)
		}
		if entry.RTTMs != rttMs {
			t.Fatalf("rtt_ms mismatch: got %d, want %d", entry.RTTMs, rttMs)
		}
		if entry.LossRate != lossRate {
			t.Fatalf("loss_rate mismatch: got %f, want %f", entry.LossRate, lossRate)
		}
		if entry.JitterMs != jitterMs {
			t.Fatalf("jitter_ms mismatch: got %d, want %d", entry.JitterMs, jitterMs)
		}
		if entry.Phase != phase {
			t.Fatalf("phase mismatch: got %s, want %s", entry.Phase, phase)
		}
		if entry.EventType != "health_update" {
			t.Fatalf("event_type mismatch: got %s, want health_update", entry.EventType)
		}
		if entry.Timestamp.IsZero() {
			t.Fatal("timestamp is zero (health_update)")
		}

		// Test OnLinkPhaseTransition → event_type = "phase_transition"
		store.lastLink = nil
		err = collector.OnLinkPhaseTransition(context.Background(), linkID, score, rttMs, lossRate, jitterMs, phase)
		if err != nil {
			t.Fatalf("OnLinkPhaseTransition returned error: %v", err)
		}

		entry = store.lastLink
		if entry == nil {
			t.Fatal("no LinkHealthTimelineEntry was saved for phase_transition")
		}

		assertValidUUIDv4(t, entry.EntryID, "entry_id (phase_transition)")

		if entry.LinkID != linkID {
			t.Fatalf("link_id mismatch (phase_transition): got %s, want %s", entry.LinkID, linkID)
		}
		if entry.HealthScore != score {
			t.Fatalf("health_score mismatch (phase_transition): got %f, want %f", entry.HealthScore, score)
		}
		if entry.RTTMs != rttMs {
			t.Fatalf("rtt_ms mismatch (phase_transition): got %d, want %d", entry.RTTMs, rttMs)
		}
		if entry.LossRate != lossRate {
			t.Fatalf("loss_rate mismatch (phase_transition): got %f, want %f", entry.LossRate, lossRate)
		}
		if entry.JitterMs != jitterMs {
			t.Fatalf("jitter_ms mismatch (phase_transition): got %d, want %d", entry.JitterMs, jitterMs)
		}
		if entry.Phase != phase {
			t.Fatalf("phase mismatch (phase_transition): got %s, want %s", entry.Phase, phase)
		}
		if entry.EventType != "phase_transition" {
			t.Fatalf("event_type mismatch: got %s, want phase_transition", entry.EventType)
		}
		if entry.Timestamp.IsZero() {
			t.Fatal("timestamp is zero (phase_transition)")
		}
	})
}

// --- Property 4: Persona version timeline entry generation completeness ---

// TestProperty4_PersonaVersionTimelineEntryCompleteness verifies TimelineCollector generates correct PersonaVersionTimelineEntry fields.
//
// Feature: v2-observability, Property 4: Persona version timeline entry generation completeness
// **Validates: Requirements 4.1, 4.2, 4.3**
func TestProperty4_PersonaVersionTimelineEntryCompleteness(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		store := &mockTimelineStore{}
		collector := NewTimelineCollector(store)

		sessionID := rapid.StringMatching(`[a-z0-9]{8,32}`).Draw(t, "session_id")
		personaID := rapid.StringMatching(`[a-z0-9]{8,32}`).Draw(t, "persona_id")
		fromVersion := rapid.Uint64Range(0, 1000).Draw(t, "from_version")
		toVersion := rapid.Uint64Range(0, 1000).Draw(t, "to_version")

		// Test OnPersonaSwitch → event_type = "switch"
		err := collector.OnPersonaSwitch(context.Background(), sessionID, personaID, fromVersion, toVersion)
		if err != nil {
			t.Fatalf("OnPersonaSwitch returned error: %v", err)
		}

		entry := store.lastPersona
		if entry == nil {
			t.Fatal("no PersonaVersionTimelineEntry was saved for switch")
		}

		assertValidUUIDv4(t, entry.EntryID, "entry_id (switch)")

		if entry.SessionID != sessionID {
			t.Fatalf("session_id mismatch: got %s, want %s", entry.SessionID, sessionID)
		}
		if entry.PersonaID != personaID {
			t.Fatalf("persona_id mismatch: got %s, want %s", entry.PersonaID, personaID)
		}
		if entry.FromVersion != fromVersion {
			t.Fatalf("from_version mismatch: got %d, want %d", entry.FromVersion, fromVersion)
		}
		if entry.ToVersion != toVersion {
			t.Fatalf("to_version mismatch: got %d, want %d", entry.ToVersion, toVersion)
		}
		if entry.EventType != "switch" {
			t.Fatalf("event_type mismatch: got %s, want switch", entry.EventType)
		}
		if entry.Timestamp.IsZero() {
			t.Fatal("timestamp is zero (switch)")
		}

		// Test OnPersonaRollback → event_type = "rollback"
		store.lastPersona = nil
		err = collector.OnPersonaRollback(context.Background(), sessionID, personaID, fromVersion, toVersion)
		if err != nil {
			t.Fatalf("OnPersonaRollback returned error: %v", err)
		}

		entry = store.lastPersona
		if entry == nil {
			t.Fatal("no PersonaVersionTimelineEntry was saved for rollback")
		}

		assertValidUUIDv4(t, entry.EntryID, "entry_id (rollback)")

		if entry.SessionID != sessionID {
			t.Fatalf("session_id mismatch (rollback): got %s, want %s", entry.SessionID, sessionID)
		}
		if entry.PersonaID != personaID {
			t.Fatalf("persona_id mismatch (rollback): got %s, want %s", entry.PersonaID, personaID)
		}
		if entry.FromVersion != fromVersion {
			t.Fatalf("from_version mismatch (rollback): got %d, want %d", entry.FromVersion, fromVersion)
		}
		if entry.ToVersion != toVersion {
			t.Fatalf("to_version mismatch (rollback): got %d, want %d", entry.ToVersion, toVersion)
		}
		if entry.EventType != "rollback" {
			t.Fatalf("event_type mismatch: got %s, want rollback", entry.EventType)
		}
		if entry.Timestamp.IsZero() {
			t.Fatal("timestamp is zero (rollback)")
		}
	})
}

// --- Property 5: Survival Mode timeline entry generation completeness ---

// TestProperty5_SurvivalModeTimelineEntryCompleteness verifies TimelineCollector generates correct SurvivalModeTimelineEntry fields.
//
// Feature: v2-observability, Property 5: Survival Mode timeline entry generation completeness
// **Validates: Requirements 5.1, 5.2**
func TestProperty5_SurvivalModeTimelineEntryCompleteness(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		store := &mockTimelineStore{}
		collector := NewTimelineCollector(store)

		from := rapid.SampledFrom(allSurvivalModes).Draw(t, "from_mode")
		to := rapid.SampledFrom(allSurvivalModes).Draw(t, "to_mode")
		triggersStr := rapid.StringMatching(`\{"trigger":"[a-z_]{3,20}"\}`).Draw(t, "triggers")
		triggers := json.RawMessage(triggersStr)
		txID := rapid.StringMatching(`[a-z0-9]{8,32}`).Draw(t, "tx_id")

		err := collector.OnModeTransition(context.Background(), from, to, triggers, txID)
		if err != nil {
			t.Fatalf("OnModeTransition returned error: %v", err)
		}

		entry := store.lastSurvival
		if entry == nil {
			t.Fatal("no SurvivalModeTimelineEntry was saved")
		}

		assertValidUUIDv4(t, entry.EntryID, "entry_id")

		if entry.FromMode != from {
			t.Fatalf("from_mode mismatch: got %s, want %s", entry.FromMode, from)
		}
		if entry.ToMode != to {
			t.Fatalf("to_mode mismatch: got %s, want %s", entry.ToMode, to)
		}
		if string(entry.Triggers) != string(triggers) {
			t.Fatalf("triggers mismatch: got %s, want %s", string(entry.Triggers), string(triggers))
		}
		if entry.TxID != txID {
			t.Fatalf("tx_id mismatch: got %s, want %s", entry.TxID, txID)
		}
		if entry.Timestamp.IsZero() {
			t.Fatal("timestamp is zero")
		}
	})
}

// --- Property 6: Transaction timeline entry generation completeness ---

// TestProperty6_TransactionTimelineEntryCompleteness verifies TimelineCollector generates correct TransactionTimelineEntry fields.
//
// Feature: v2-observability, Property 6: Transaction timeline entry generation completeness
// **Validates: Requirements 6.1, 6.2**
func TestProperty6_TransactionTimelineEntryCompleteness(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		store := &mockTimelineStore{}
		collector := NewTimelineCollector(store)

		txID := rapid.StringMatching(`[a-z0-9]{8,32}`).Draw(t, "tx_id")
		from := rapid.SampledFrom(commit.AllTxPhases).Draw(t, "from_phase")
		to := rapid.SampledFrom(commit.AllTxPhases).Draw(t, "to_phase")
		phaseDataStr := rapid.StringMatching(`\{"state":"[a-z_]{3,20}"\}`).Draw(t, "phase_data")
		phaseData := json.RawMessage(phaseDataStr)

		err := collector.OnTxPhaseTransition(context.Background(), txID, from, to, phaseData)
		if err != nil {
			t.Fatalf("OnTxPhaseTransition returned error: %v", err)
		}

		entry := store.lastTx
		if entry == nil {
			t.Fatal("no TransactionTimelineEntry was saved")
		}

		assertValidUUIDv4(t, entry.EntryID, "entry_id")

		if entry.TxID != txID {
			t.Fatalf("tx_id mismatch: got %s, want %s", entry.TxID, txID)
		}
		if entry.FromPhase != from {
			t.Fatalf("from_phase mismatch: got %s, want %s", entry.FromPhase, from)
		}
		if entry.ToPhase != to {
			t.Fatalf("to_phase mismatch: got %s, want %s", entry.ToPhase, to)
		}
		if string(entry.PhaseData) != string(phaseData) {
			t.Fatalf("phase_data mismatch: got %s, want %s", string(entry.PhaseData), string(phaseData))
		}
		if entry.Timestamp.IsZero() {
			t.Fatal("timestamp is zero")
		}
	})
}

// --- Mock providers for DiagnosticAggregator ---

type mockSessionProvider struct {
	sessions map[string]*orchestrator.SessionState
}

func (m *mockSessionProvider) Get(_ context.Context, sessionID string) (*orchestrator.SessionState, error) {
	s, ok := m.sessions[sessionID]
	if !ok {
		return nil, &ErrSessionNotFound{SessionID: sessionID}
	}
	return s, nil
}

func (m *mockSessionProvider) ListByFilter(_ context.Context, _ orchestrator.SessionFilter) ([]*orchestrator.SessionState, error) {
	var result []*orchestrator.SessionState
	for _, s := range m.sessions {
		result = append(result, s)
	}
	return result, nil
}

type mockLinkProvider struct {
	links map[string]*orchestrator.LinkState
}

func (m *mockLinkProvider) Get(_ context.Context, linkID string) (*orchestrator.LinkState, error) {
	l, ok := m.links[linkID]
	if !ok {
		return nil, fmt.Errorf("link not found: %s", linkID)
	}
	return l, nil
}

type mockControlProvider struct {
	controls map[string]*orchestrator.ControlState
}

func (m *mockControlProvider) GetOrCreate(_ context.Context, gatewayID string) (*orchestrator.ControlState, error) {
	c, ok := m.controls[gatewayID]
	if !ok {
		c = &orchestrator.ControlState{GatewayID: gatewayID}
		m.controls[gatewayID] = c
	}
	return c, nil
}

type mockTxProvider struct {
	txs       map[string]*commit.CommitTransaction
	activeTxs []*commit.CommitTransaction
}

func (m *mockTxProvider) GetTransaction(_ context.Context, txID string) (*commit.CommitTransaction, error) {
	tx, ok := m.txs[txID]
	if !ok {
		return nil, &ErrTransactionNotFound{TxID: txID}
	}
	return tx, nil
}

func (m *mockTxProvider) GetActiveTransactions(_ context.Context) ([]*commit.CommitTransaction, error) {
	return m.activeTxs, nil
}

type mockSurvivalProvider struct {
	mode    orchestrator.SurvivalMode
	history []*survival.TransitionRecord
}

func (m *mockSurvivalProvider) GetCurrentMode() orchestrator.SurvivalMode {
	return m.mode
}

func (m *mockSurvivalProvider) GetTransitionHistory(n int) []*survival.TransitionRecord {
	if n >= len(m.history) {
		return m.history
	}
	return m.history[:n]
}

// --- Rapid generators for DiagnosticAggregator ---

func genSessionState(t *rapid.T) *orchestrator.SessionState {
	return &orchestrator.SessionState{
		SessionID:           rapid.StringMatching(`[a-z0-9]{8,16}`).Draw(t, "session_id"),
		UserID:              rapid.StringMatching(`[a-z0-9]{8,16}`).Draw(t, "user_id"),
		ClientID:            rapid.StringMatching(`[a-z0-9]{8,16}`).Draw(t, "client_id"),
		GatewayID:           rapid.StringMatching(`[a-z0-9]{4,8}`).Draw(t, "gateway_id"),
		CurrentPersonaID:    rapid.StringMatching(`[a-z0-9]{8,16}`).Draw(t, "persona_id"),
		CurrentLinkID:       rapid.StringMatching(`[a-z0-9]{8,16}`).Draw(t, "link_id"),
		CurrentSurvivalMode: rapid.SampledFrom(allSurvivalModes).Draw(t, "survival_mode"),
		State:               rapid.SampledFrom(orchestrator.AllSessionPhases).Draw(t, "state"),
	}
}

func genLinkState(t *rapid.T, linkID string) *orchestrator.LinkState {
	return &orchestrator.LinkState{
		LinkID:           linkID,
		Phase:            rapid.SampledFrom(orchestrator.AllLinkPhases).Draw(t, "link_phase"),
		LastSwitchReason: rapid.StringMatching(`[a-z_]{0,20}`).Draw(t, "link_switch_reason"),
	}
}

func genControlState(t *rapid.T, gatewayID string) *orchestrator.ControlState {
	return &orchestrator.ControlState{
		GatewayID:        gatewayID,
		PersonaVersion:   rapid.Uint64Range(0, 1000).Draw(t, "persona_version"),
		LastSwitchReason: rapid.StringMatching(`[a-z_]{0,20}`).Draw(t, "control_switch_reason"),
	}
}

// --- Property 7: Session diagnostic view aggregation correctness ---

// TestProperty7_SessionDiagnosticAggregation verifies DiagnosticAggregator.GetSessionDiagnostic
// returns correct aggregated data from SessionState, LinkState, and ControlState.
//
// Feature: v2-observability, Property 7: Session diagnostic view aggregation correctness
// **Validates: Requirements 7.1, 7.2, 7.3**
func TestProperty7_SessionDiagnosticAggregation(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		session := genSessionState(t)
		link := genLinkState(t, session.CurrentLinkID)
		control := genControlState(t, session.GatewayID)

		sessions := &mockSessionProvider{sessions: map[string]*orchestrator.SessionState{session.SessionID: session}}
		links := &mockLinkProvider{links: map[string]*orchestrator.LinkState{link.LinkID: link}}
		controls := &mockControlProvider{controls: map[string]*orchestrator.ControlState{control.GatewayID: control}}
		txs := &mockTxProvider{txs: map[string]*commit.CommitTransaction{}}
		survProv := &mockSurvivalProvider{mode: orchestrator.SurvivalModeNormal}
		timeline := &mockTimelineStore{}

		agg := NewDiagnosticAggregator(sessions, links, controls, txs, survProv, timeline)

		diag, err := agg.GetSessionDiagnostic(context.Background(), session.SessionID)
		if err != nil {
			t.Fatalf("GetSessionDiagnostic returned error: %v", err)
		}

		if diag.SessionID != session.SessionID {
			t.Fatalf("session_id mismatch: got %s, want %s", diag.SessionID, session.SessionID)
		}
		if diag.CurrentLinkID != session.CurrentLinkID {
			t.Fatalf("current_link_id mismatch: got %s, want %s", diag.CurrentLinkID, session.CurrentLinkID)
		}
		if diag.CurrentLinkPhase != link.Phase {
			t.Fatalf("current_link_phase mismatch: got %s, want %s", diag.CurrentLinkPhase, link.Phase)
		}
		if diag.CurrentPersonaID != session.CurrentPersonaID {
			t.Fatalf("current_persona_id mismatch: got %s, want %s", diag.CurrentPersonaID, session.CurrentPersonaID)
		}
		if diag.CurrentPersonaVersion != control.PersonaVersion {
			t.Fatalf("current_persona_version mismatch: got %d, want %d", diag.CurrentPersonaVersion, control.PersonaVersion)
		}
		if diag.CurrentSurvivalMode != session.CurrentSurvivalMode {
			t.Fatalf("current_survival_mode mismatch: got %s, want %s", diag.CurrentSurvivalMode, session.CurrentSurvivalMode)
		}
		if diag.SessionState != session.State {
			t.Fatalf("session_state mismatch: got %s, want %s", diag.SessionState, session.State)
		}
		if diag.LastSwitchReason != link.LastSwitchReason {
			t.Fatalf("last_switch_reason mismatch: got %s, want %s", diag.LastSwitchReason, link.LastSwitchReason)
		}
		if diag.LastRollbackReason != control.LastSwitchReason {
			t.Fatalf("last_rollback_reason mismatch: got %s, want %s", diag.LastRollbackReason, control.LastSwitchReason)
		}
	})
}

// --- Property 8: System diagnostic view aggregation correctness ---

// TestProperty8_SystemDiagnosticAggregation verifies DiagnosticAggregator.GetSystemDiagnostic
// returns correct aggregated data from SurvivalProvider, SessionProvider, and TxProvider.
//
// Feature: v2-observability, Property 8: System diagnostic view aggregation correctness
// **Validates: Requirements 8.1, 8.2, 8.3**
func TestProperty8_SystemDiagnosticAggregation(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		currentMode := rapid.SampledFrom(allSurvivalModes).Draw(t, "current_mode")

		// Generate random sessions: mix of active and closed
		numSessions := rapid.IntRange(0, 10).Draw(t, "num_sessions")
		sessionMap := make(map[string]*orchestrator.SessionState)
		expectedActiveCount := 0
		expectedLinkSet := make(map[string]struct{})

		for i := 0; i < numSessions; i++ {
			sid := fmt.Sprintf("sess-%d", i)
			state := rapid.SampledFrom(orchestrator.AllSessionPhases).Draw(t, fmt.Sprintf("sess_state_%d", i))
			linkID := fmt.Sprintf("link-%d", rapid.IntRange(0, 5).Draw(t, fmt.Sprintf("link_idx_%d", i)))
			sessionMap[sid] = &orchestrator.SessionState{
				SessionID:     sid,
				CurrentLinkID: linkID,
				State:         state,
				GatewayID:     "gw-1",
			}
			if state != orchestrator.SessionPhaseClosed {
				expectedActiveCount++
				expectedLinkSet[linkID] = struct{}{}
			}
		}

		// Generate optional active transaction
		hasActiveTx := rapid.Bool().Draw(t, "has_active_tx")
		var activeTxs []*commit.CommitTransaction
		var expectedTxInfo *ActiveTxInfo
		if hasActiveTx {
			txType := rapid.SampledFrom(allTxTypes).Draw(t, "active_tx_type")
			txPhase := rapid.SampledFrom([]commit.TxPhase{
				commit.TxPhasePreparing, commit.TxPhaseValidating,
				commit.TxPhaseShadowWriting, commit.TxPhaseFlipping,
				commit.TxPhaseAcknowledging,
			}).Draw(t, "active_tx_phase")
			tx := &commit.CommitTransaction{
				TxID:    uuid.New().String(),
				TxType:  txType,
				TxPhase: txPhase,
			}
			activeTxs = append(activeTxs, tx)
			expectedTxInfo = &ActiveTxInfo{
				TxID:    tx.TxID,
				TxType:  tx.TxType,
				TxPhase: tx.TxPhase,
			}
		}

		sessions := &mockSessionProvider{sessions: sessionMap}
		links := &mockLinkProvider{links: map[string]*orchestrator.LinkState{}}
		controls := &mockControlProvider{controls: map[string]*orchestrator.ControlState{}}
		txProv := &mockTxProvider{txs: map[string]*commit.CommitTransaction{}, activeTxs: activeTxs}
		survProv := &mockSurvivalProvider{mode: currentMode}
		timeline := &mockTimelineStore{}

		agg := NewDiagnosticAggregator(sessions, links, controls, txProv, survProv, timeline)

		diag, err := agg.GetSystemDiagnostic(context.Background())
		if err != nil {
			t.Fatalf("GetSystemDiagnostic returned error: %v", err)
		}

		if diag.CurrentSurvivalMode != currentMode {
			t.Fatalf("current_survival_mode mismatch: got %s, want %s", diag.CurrentSurvivalMode, currentMode)
		}
		if diag.ActiveSessionCount != expectedActiveCount {
			t.Fatalf("active_session_count mismatch: got %d, want %d", diag.ActiveSessionCount, expectedActiveCount)
		}
		if diag.ActiveLinkCount != len(expectedLinkSet) {
			t.Fatalf("active_link_count mismatch: got %d, want %d", diag.ActiveLinkCount, len(expectedLinkSet))
		}

		if hasActiveTx {
			if diag.ActiveTransaction == nil {
				t.Fatal("active_transaction should not be nil when active tx exists")
			}
			if diag.ActiveTransaction.TxID != expectedTxInfo.TxID {
				t.Fatalf("active_transaction.tx_id mismatch: got %s, want %s", diag.ActiveTransaction.TxID, expectedTxInfo.TxID)
			}
			if diag.ActiveTransaction.TxType != expectedTxInfo.TxType {
				t.Fatalf("active_transaction.tx_type mismatch: got %s, want %s", diag.ActiveTransaction.TxType, expectedTxInfo.TxType)
			}
			if diag.ActiveTransaction.TxPhase != expectedTxInfo.TxPhase {
				t.Fatalf("active_transaction.tx_phase mismatch: got %s, want %s", diag.ActiveTransaction.TxPhase, expectedTxInfo.TxPhase)
			}
		} else {
			if diag.ActiveTransaction != nil {
				t.Fatal("active_transaction should be nil when no active tx")
			}
		}
	})
}

// --- Property 9: Transaction diagnostic view and stuck_duration invariant ---

// TestProperty9_TransactionDiagnosticStuckDuration verifies DiagnosticAggregator.GetTransactionDiagnostic
// returns correct stuck_duration: zero for terminal phases, positive for non-terminal.
//
// Feature: v2-observability, Property 9: Transaction diagnostic view and stuck_duration invariant
// **Validates: Requirements 9.1, 9.2, 9.4**
func TestProperty9_TransactionDiagnosticStuckDuration(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		isTerminal := rapid.Bool().Draw(t, "is_terminal")
		txType := rapid.SampledFrom(allTxTypes).Draw(t, "tx_type")

		var txPhase commit.TxPhase
		if isTerminal {
			txPhase = rapid.SampledFrom(terminalPhases).Draw(t, "terminal_phase")
		} else {
			txPhase = rapid.SampledFrom([]commit.TxPhase{
				commit.TxPhasePreparing, commit.TxPhaseValidating,
				commit.TxPhaseShadowWriting, commit.TxPhaseFlipping,
				commit.TxPhaseAcknowledging,
			}).Draw(t, "non_terminal_phase")
		}

		txID := uuid.New().String()
		createdAt := time.Now().UTC().Add(-time.Duration(rapid.IntRange(10, 3600).Draw(t, "age_seconds")) * time.Second)

		tx := &commit.CommitTransaction{
			TxID:      txID,
			TxType:    txType,
			TxPhase:   txPhase,
			CreatedAt: createdAt,
		}
		if isTerminal {
			finishedAt := createdAt.Add(time.Duration(rapid.IntRange(1, 600).Draw(t, "duration_seconds")) * time.Second)
			tx.FinishedAt = &finishedAt
		}

		// Create timeline entries: at least one entry so stuck_duration can be computed
		entryTime := createdAt.Add(time.Duration(rapid.IntRange(1, 5).Draw(t, "entry_offset")) * time.Second)
		timelineEntries := []*TransactionTimelineEntry{
			{
				EntryID:   uuid.New().String(),
				TxID:      txID,
				FromPhase: commit.TxPhasePreparing,
				ToPhase:   txPhase,
				Timestamp: entryTime,
			},
		}

		// Mock timeline store that returns our entries
		tlStore := &mockTimelineStoreWithTxEntries{
			entries: map[string][]*TransactionTimelineEntry{txID: timelineEntries},
		}

		sessions := &mockSessionProvider{sessions: map[string]*orchestrator.SessionState{}}
		links := &mockLinkProvider{links: map[string]*orchestrator.LinkState{}}
		controls := &mockControlProvider{controls: map[string]*orchestrator.ControlState{}}
		txProv := &mockTxProvider{txs: map[string]*commit.CommitTransaction{txID: tx}}
		survProv := &mockSurvivalProvider{mode: orchestrator.SurvivalModeNormal}

		agg := NewDiagnosticAggregator(sessions, links, controls, txProv, survProv, tlStore)

		diag, err := agg.GetTransactionDiagnostic(context.Background(), txID)
		if err != nil {
			t.Fatalf("GetTransactionDiagnostic returned error: %v", err)
		}

		if diag.TxID != txID {
			t.Fatalf("tx_id mismatch: got %s, want %s", diag.TxID, txID)
		}
		if diag.TxType != txType {
			t.Fatalf("tx_type mismatch: got %s, want %s", diag.TxType, txType)
		}
		if diag.CurrentPhase != txPhase {
			t.Fatalf("current_phase mismatch: got %s, want %s", diag.CurrentPhase, txPhase)
		}

		if isTerminal {
			if diag.StuckDuration != 0 {
				t.Fatalf("terminal phase %s: stuck_duration should be 0, got %v", txPhase, diag.StuckDuration)
			}
		} else {
			if diag.StuckDuration <= 0 {
				t.Fatalf("non-terminal phase %s: stuck_duration should be > 0, got %v", txPhase, diag.StuckDuration)
			}
		}
	})
}

// mockTimelineStoreWithTxEntries extends mockTimelineStore with transaction entry support
type mockTimelineStoreWithTxEntries struct {
	mockTimelineStore
	entries map[string][]*TransactionTimelineEntry
}

func (m *mockTimelineStoreWithTxEntries) ListTransactionEntries(_ context.Context, txID string) ([]*TransactionTimelineEntry, error) {
	if entries, ok := m.entries[txID]; ok {
		return entries, nil
	}
	return nil, nil
}

// --- Property 10: Data retention cleanup correctness ---

// mockCleanupAuditStore simulates AuditStore.Cleanup by tracking records and counting deletions.
type mockCleanupAuditStore struct {
	records []*AuditRecord
	deleted int64
}

func (m *mockCleanupAuditStore) Save(_ context.Context, record *AuditRecord) error {
	m.records = append(m.records, record)
	return nil
}

func (m *mockCleanupAuditStore) GetByTxID(_ context.Context, _ string) (*AuditRecord, error) {
	return nil, nil
}

func (m *mockCleanupAuditStore) List(_ context.Context, _ *AuditFilter) ([]*AuditRecord, error) {
	return nil, nil
}

func (m *mockCleanupAuditStore) Cleanup(_ context.Context, retentionDays int) (int64, error) {
	if retentionDays <= 0 {
		return 0, fmt.Errorf("retentionDays must be > 0, got %d", retentionDays)
	}
	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	var kept []*AuditRecord
	var deleted int64
	for _, r := range m.records {
		if r.CreatedAt.Before(cutoff) {
			deleted++
		} else {
			kept = append(kept, r)
		}
	}
	m.records = kept
	m.deleted = deleted
	return deleted, nil
}

// mockCleanupTimelineStore simulates TimelineStore.Cleanup by tracking entries and counting deletions.
type mockCleanupTimelineStore struct {
	sessionEntries  []*SessionTimelineEntry
	linkEntries     []*LinkHealthTimelineEntry
	personaEntries  []*PersonaVersionTimelineEntry
	survivalEntries []*SurvivalModeTimelineEntry
	txEntries       []*TransactionTimelineEntry
}

func (m *mockCleanupTimelineStore) SaveSessionEntry(_ context.Context, e *SessionTimelineEntry) error {
	m.sessionEntries = append(m.sessionEntries, e)
	return nil
}
func (m *mockCleanupTimelineStore) ListSessionEntries(_ context.Context, _ string, _ *TimeRange) ([]*SessionTimelineEntry, error) {
	return nil, nil
}
func (m *mockCleanupTimelineStore) SaveLinkHealthEntry(_ context.Context, e *LinkHealthTimelineEntry) error {
	m.linkEntries = append(m.linkEntries, e)
	return nil
}
func (m *mockCleanupTimelineStore) ListLinkHealthEntries(_ context.Context, _ string, _ *TimeRange) ([]*LinkHealthTimelineEntry, error) {
	return nil, nil
}
func (m *mockCleanupTimelineStore) SavePersonaVersionEntry(_ context.Context, e *PersonaVersionTimelineEntry) error {
	m.personaEntries = append(m.personaEntries, e)
	return nil
}
func (m *mockCleanupTimelineStore) ListPersonaVersionEntries(_ context.Context, _ string, _ *TimeRange) ([]*PersonaVersionTimelineEntry, error) {
	return nil, nil
}
func (m *mockCleanupTimelineStore) ListPersonaVersionEntriesByPersona(_ context.Context, _ string, _ *TimeRange) ([]*PersonaVersionTimelineEntry, error) {
	return nil, nil
}
func (m *mockCleanupTimelineStore) SaveSurvivalModeEntry(_ context.Context, e *SurvivalModeTimelineEntry) error {
	m.survivalEntries = append(m.survivalEntries, e)
	return nil
}
func (m *mockCleanupTimelineStore) ListSurvivalModeEntries(_ context.Context, _ *TimeRange) ([]*SurvivalModeTimelineEntry, error) {
	return nil, nil
}
func (m *mockCleanupTimelineStore) SaveTransactionEntry(_ context.Context, e *TransactionTimelineEntry) error {
	m.txEntries = append(m.txEntries, e)
	return nil
}
func (m *mockCleanupTimelineStore) ListTransactionEntries(_ context.Context, _ string) ([]*TransactionTimelineEntry, error) {
	return nil, nil
}

func (m *mockCleanupTimelineStore) Cleanup(_ context.Context, retentionDays int) (int64, error) {
	if retentionDays <= 0 {
		return 0, fmt.Errorf("retentionDays must be > 0, got %d", retentionDays)
	}
	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	var totalDeleted int64

	// Session entries
	var keptSession []*SessionTimelineEntry
	for _, e := range m.sessionEntries {
		if e.Timestamp.Before(cutoff) {
			totalDeleted++
		} else {
			keptSession = append(keptSession, e)
		}
	}
	m.sessionEntries = keptSession

	// Link health entries
	var keptLink []*LinkHealthTimelineEntry
	for _, e := range m.linkEntries {
		if e.Timestamp.Before(cutoff) {
			totalDeleted++
		} else {
			keptLink = append(keptLink, e)
		}
	}
	m.linkEntries = keptLink

	// Persona version entries
	var keptPersona []*PersonaVersionTimelineEntry
	for _, e := range m.personaEntries {
		if e.Timestamp.Before(cutoff) {
			totalDeleted++
		} else {
			keptPersona = append(keptPersona, e)
		}
	}
	m.personaEntries = keptPersona

	// Survival mode entries
	var keptSurvival []*SurvivalModeTimelineEntry
	for _, e := range m.survivalEntries {
		if e.Timestamp.Before(cutoff) {
			totalDeleted++
		} else {
			keptSurvival = append(keptSurvival, e)
		}
	}
	m.survivalEntries = keptSurvival

	// Transaction entries
	var keptTx []*TransactionTimelineEntry
	for _, e := range m.txEntries {
		if e.Timestamp.Before(cutoff) {
			totalDeleted++
		} else {
			keptTx = append(keptTx, e)
		}
	}
	m.txEntries = keptTx

	return totalDeleted, nil
}

// TestProperty10_DataRetentionCleanupCorrectness verifies that Cleanup(N) deletes records older than N days
// and preserves records within N days, returning the correct count of deleted records.
//
// Feature: v2-observability, Property 10: Data retention cleanup correctness
// **Validates: Requirements 13.1, 13.2, 13.3**
func TestProperty10_DataRetentionCleanupCorrectness(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		retentionDays := rapid.IntRange(1, 365).Draw(t, "retention_days")
		numAuditRecords := rapid.IntRange(0, 20).Draw(t, "num_audit_records")
		numTimelineEntries := rapid.IntRange(0, 20).Draw(t, "num_timeline_entries")

		now := time.Now().UTC()
		cutoff := now.AddDate(0, 0, -retentionDays)

		// Build audit records with random timestamps
		auditStore := &mockCleanupAuditStore{}
		expectedAuditDeleted := int64(0)
		expectedAuditPreserved := int64(0)
		for i := 0; i < numAuditRecords; i++ {
			// Generate age in hours: negative means in the past
			ageHours := rapid.IntRange(0, 24*400).Draw(t, fmt.Sprintf("audit_age_hours_%d", i))
			createdAt := now.Add(-time.Duration(ageHours) * time.Hour)
			record := &AuditRecord{
				AuditID:   fmt.Sprintf("audit-%d", i),
				TxID:      fmt.Sprintf("tx-%d", i),
				TxType:    "persona_switch",
				CreatedAt: createdAt,
			}
			auditStore.records = append(auditStore.records, record)
			if createdAt.Before(cutoff) {
				expectedAuditDeleted++
			} else {
				expectedAuditPreserved++
			}
		}

		// Execute audit cleanup
		deletedAudit, err := auditStore.Cleanup(context.Background(), retentionDays)
		if err != nil {
			t.Fatalf("AuditStore.Cleanup returned error: %v", err)
		}
		if deletedAudit != expectedAuditDeleted {
			t.Fatalf("AuditStore deleted count mismatch: got %d, want %d", deletedAudit, expectedAuditDeleted)
		}
		if int64(len(auditStore.records)) != expectedAuditPreserved {
			t.Fatalf("AuditStore preserved count mismatch: got %d, want %d", len(auditStore.records), expectedAuditPreserved)
		}
		// Verify all remaining records are within retention period
		for _, r := range auditStore.records {
			if r.CreatedAt.Before(cutoff) {
				t.Fatalf("AuditStore: record with created_at %v should have been deleted (cutoff %v)", r.CreatedAt, cutoff)
			}
		}

		// Build timeline entries with random timestamps across all 5 types
		tlStore := &mockCleanupTimelineStore{}
		expectedTLDeleted := int64(0)
		expectedTLPreserved := int64(0)
		for i := 0; i < numTimelineEntries; i++ {
			ageHours := rapid.IntRange(0, 24*400).Draw(t, fmt.Sprintf("tl_age_hours_%d", i))
			ts := now.Add(-time.Duration(ageHours) * time.Hour)
			entryType := rapid.IntRange(0, 4).Draw(t, fmt.Sprintf("tl_type_%d", i))

			switch entryType {
			case 0:
				tlStore.sessionEntries = append(tlStore.sessionEntries, &SessionTimelineEntry{
					EntryID: fmt.Sprintf("se-%d", i), Timestamp: ts,
				})
			case 1:
				tlStore.linkEntries = append(tlStore.linkEntries, &LinkHealthTimelineEntry{
					EntryID: fmt.Sprintf("le-%d", i), Timestamp: ts,
				})
			case 2:
				tlStore.personaEntries = append(tlStore.personaEntries, &PersonaVersionTimelineEntry{
					EntryID: fmt.Sprintf("pe-%d", i), Timestamp: ts,
				})
			case 3:
				tlStore.survivalEntries = append(tlStore.survivalEntries, &SurvivalModeTimelineEntry{
					EntryID: fmt.Sprintf("sme-%d", i), Timestamp: ts,
				})
			case 4:
				tlStore.txEntries = append(tlStore.txEntries, &TransactionTimelineEntry{
					EntryID: fmt.Sprintf("te-%d", i), Timestamp: ts,
				})
			}

			if ts.Before(cutoff) {
				expectedTLDeleted++
			} else {
				expectedTLPreserved++
			}
		}

		// Execute timeline cleanup
		deletedTL, err := tlStore.Cleanup(context.Background(), retentionDays)
		if err != nil {
			t.Fatalf("TimelineStore.Cleanup returned error: %v", err)
		}
		if deletedTL != expectedTLDeleted {
			t.Fatalf("TimelineStore deleted count mismatch: got %d, want %d", deletedTL, expectedTLDeleted)
		}

		// Count remaining timeline entries
		remainingTL := int64(len(tlStore.sessionEntries) + len(tlStore.linkEntries) +
			len(tlStore.personaEntries) + len(tlStore.survivalEntries) + len(tlStore.txEntries))
		if remainingTL != expectedTLPreserved {
			t.Fatalf("TimelineStore preserved count mismatch: got %d, want %d", remainingTL, expectedTLPreserved)
		}

		// Verify all remaining timeline entries are within retention period
		for _, e := range tlStore.sessionEntries {
			if e.Timestamp.Before(cutoff) {
				t.Fatalf("TimelineStore: session entry with timestamp %v should have been deleted", e.Timestamp)
			}
		}
		for _, e := range tlStore.linkEntries {
			if e.Timestamp.Before(cutoff) {
				t.Fatalf("TimelineStore: link entry with timestamp %v should have been deleted", e.Timestamp)
			}
		}
		for _, e := range tlStore.personaEntries {
			if e.Timestamp.Before(cutoff) {
				t.Fatalf("TimelineStore: persona entry with timestamp %v should have been deleted", e.Timestamp)
			}
		}
		for _, e := range tlStore.survivalEntries {
			if e.Timestamp.Before(cutoff) {
				t.Fatalf("TimelineStore: survival entry with timestamp %v should have been deleted", e.Timestamp)
			}
		}
		for _, e := range tlStore.txEntries {
			if e.Timestamp.Before(cutoff) {
				t.Fatalf("TimelineStore: tx entry with timestamp %v should have been deleted", e.Timestamp)
			}
		}
	})
}

// --- Property 11: JSON round-trip for all data structures ---

// snakeCaseKeyRegex matches valid snake_case JSON keys
var snakeCaseKeyRegex = regexp.MustCompile(`^[a-z][a-z0-9]*(_[a-z0-9]+)*$`)

// rfc3339Regex matches RFC 3339 timestamps in JSON strings
var rfc3339Regex = regexp.MustCompile(`"\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:\d{2})"`)

// jsonKeyRegex extracts all JSON keys from marshaled bytes
var jsonKeyRegex = regexp.MustCompile(`"([^"]+)"\s*:`)

// assertSnakeCaseKeys checks that all JSON keys in the marshaled bytes are snake_case.
func assertSnakeCaseKeys(t *rapid.T, data []byte, label string) {
	keys := jsonKeyRegex.FindAllSubmatch(data, -1)
	for _, match := range keys {
		key := string(match[1])
		if !snakeCaseKeyRegex.MatchString(key) {
			t.Fatalf("%s: JSON key %q is not snake_case", label, key)
		}
	}
}

// assertRFC3339TimeFields checks that time fields in JSON are RFC 3339 formatted.
// It parses the JSON string looking for time-like values and validates them.
func assertRFC3339TimeFields(t *rapid.T, data []byte, label string) {
	matches := rfc3339Regex.FindAll(data, -1)
	for _, match := range matches {
		// Strip surrounding quotes
		timeStr := string(match[1 : len(match)-1])
		_, err := time.Parse(time.RFC3339Nano, timeStr)
		if err != nil {
			_, err2 := time.Parse(time.RFC3339, timeStr)
			if err2 != nil {
				t.Fatalf("%s: time value %q is not RFC 3339: %v", label, timeStr, err)
			}
		}
	}
}

// genTime generates a random time.Time for property testing.
func genTime(t *rapid.T, label string) time.Time {
	// Generate a time within a reasonable range (2020-2030)
	year := rapid.IntRange(2020, 2030).Draw(t, label+"_year")
	month := rapid.IntRange(1, 12).Draw(t, label+"_month")
	day := rapid.IntRange(1, 28).Draw(t, label+"_day")
	hour := rapid.IntRange(0, 23).Draw(t, label+"_hour")
	min := rapid.IntRange(0, 59).Draw(t, label+"_min")
	sec := rapid.IntRange(0, 59).Draw(t, label+"_sec")
	return time.Date(year, time.Month(month), day, hour, min, sec, 0, time.UTC)
}

// genAuditRecord generates a random AuditRecord for property testing.
func genAuditRecord(t *rapid.T) *AuditRecord {
	return &AuditRecord{
		AuditID:           uuid.New().String(),
		TxID:              uuid.New().String(),
		TxType:            rapid.SampledFrom(allTxTypes).Draw(t, "ar_tx_type"),
		InitiatedAt:       genTime(t, "ar_initiated"),
		FinishedAt:        genTime(t, "ar_finished"),
		InitiationReason:  rapid.StringMatching(`[a-z_]{0,20}`).Draw(t, "ar_reason"),
		TargetState:       json.RawMessage(`{"key":"value"}`),
		BudgetVerdict:     rapid.SampledFrom([]string{"allow", "deny_and_hold", ""}).Draw(t, "ar_verdict"),
		DenyReason:        rapid.StringMatching(`[a-z_]{0,20}`).Draw(t, "ar_deny"),
		FlipSuccess:       rapid.Bool().Draw(t, "ar_flip"),
		RollbackTriggered: rapid.Bool().Draw(t, "ar_rollback"),
		CreatedAt:         genTime(t, "ar_created"),
	}
}

// genSessionTimelineEntry generates a random SessionTimelineEntry.
func genSessionTimelineEntry(t *rapid.T) *SessionTimelineEntry {
	return &SessionTimelineEntry{
		EntryID:      uuid.New().String(),
		SessionID:    rapid.StringMatching(`[a-z0-9]{8,16}`).Draw(t, "ste_session"),
		FromState:    rapid.SampledFrom(orchestrator.AllSessionPhases).Draw(t, "ste_from"),
		ToState:      rapid.SampledFrom(orchestrator.AllSessionPhases).Draw(t, "ste_to"),
		Reason:       rapid.StringMatching(`[a-z_]{0,20}`).Draw(t, "ste_reason"),
		LinkID:       rapid.StringMatching(`[a-z0-9]{8,16}`).Draw(t, "ste_link"),
		PersonaID:    rapid.StringMatching(`[a-z0-9]{8,16}`).Draw(t, "ste_persona"),
		SurvivalMode: rapid.SampledFrom(allSurvivalModes).Draw(t, "ste_mode"),
		Timestamp:    genTime(t, "ste_ts"),
	}
}

// genLinkHealthTimelineEntry generates a random LinkHealthTimelineEntry.
func genLinkHealthTimelineEntry(t *rapid.T) *LinkHealthTimelineEntry {
	return &LinkHealthTimelineEntry{
		EntryID:     uuid.New().String(),
		LinkID:      rapid.StringMatching(`[a-z0-9]{8,16}`).Draw(t, "lhte_link"),
		HealthScore: rapid.Float64Range(0, 100).Draw(t, "lhte_score"),
		RTTMs:       rapid.Int64Range(0, 10000).Draw(t, "lhte_rtt"),
		LossRate:    rapid.Float64Range(0, 1).Draw(t, "lhte_loss"),
		JitterMs:    rapid.Int64Range(0, 5000).Draw(t, "lhte_jitter"),
		Phase:       rapid.SampledFrom(orchestrator.AllLinkPhases).Draw(t, "lhte_phase"),
		EventType:   rapid.SampledFrom([]string{"health_update", "phase_transition"}).Draw(t, "lhte_event"),
		Timestamp:   genTime(t, "lhte_ts"),
	}
}

// genPersonaVersionTimelineEntry generates a random PersonaVersionTimelineEntry.
func genPersonaVersionTimelineEntry(t *rapid.T) *PersonaVersionTimelineEntry {
	return &PersonaVersionTimelineEntry{
		EntryID:     uuid.New().String(),
		SessionID:   rapid.StringMatching(`[a-z0-9]{8,16}`).Draw(t, "pvte_session"),
		PersonaID:   rapid.StringMatching(`[a-z0-9]{8,16}`).Draw(t, "pvte_persona"),
		FromVersion: rapid.Uint64Range(0, 1000).Draw(t, "pvte_from"),
		ToVersion:   rapid.Uint64Range(0, 1000).Draw(t, "pvte_to"),
		EventType:   rapid.SampledFrom([]string{"switch", "rollback"}).Draw(t, "pvte_event"),
		Timestamp:   genTime(t, "pvte_ts"),
	}
}

// genSurvivalModeTimelineEntry generates a random SurvivalModeTimelineEntry.
func genSurvivalModeTimelineEntry(t *rapid.T) *SurvivalModeTimelineEntry {
	return &SurvivalModeTimelineEntry{
		EntryID:   uuid.New().String(),
		FromMode:  rapid.SampledFrom(allSurvivalModes).Draw(t, "smte_from"),
		ToMode:    rapid.SampledFrom(allSurvivalModes).Draw(t, "smte_to"),
		Triggers:  json.RawMessage(`{"trigger":"test"}`),
		TxID:      rapid.StringMatching(`[a-z0-9]{8,16}`).Draw(t, "smte_tx"),
		Timestamp: genTime(t, "smte_ts"),
	}
}

// genTransactionTimelineEntry generates a random TransactionTimelineEntry.
func genTransactionTimelineEntry(t *rapid.T) *TransactionTimelineEntry {
	return &TransactionTimelineEntry{
		EntryID:   uuid.New().String(),
		TxID:      rapid.StringMatching(`[a-z0-9]{8,16}`).Draw(t, "tte_tx"),
		FromPhase: rapid.SampledFrom(commit.AllTxPhases).Draw(t, "tte_from"),
		ToPhase:   rapid.SampledFrom(commit.AllTxPhases).Draw(t, "tte_to"),
		PhaseData: json.RawMessage(`{"state":"active"}`),
		Timestamp: genTime(t, "tte_ts"),
	}
}

// genSessionDiagnostic generates a random SessionDiagnostic.
func genSessionDiagnostic(t *rapid.T) *SessionDiagnostic {
	return &SessionDiagnostic{
		SessionID:             rapid.StringMatching(`[a-z0-9]{8,16}`).Draw(t, "sd_session"),
		CurrentLinkID:         rapid.StringMatching(`[a-z0-9]{8,16}`).Draw(t, "sd_link"),
		CurrentLinkPhase:      rapid.SampledFrom(orchestrator.AllLinkPhases).Draw(t, "sd_link_phase"),
		CurrentPersonaID:      rapid.StringMatching(`[a-z0-9]{8,16}`).Draw(t, "sd_persona"),
		CurrentPersonaVersion: rapid.Uint64Range(0, 1000).Draw(t, "sd_persona_ver"),
		CurrentSurvivalMode:   rapid.SampledFrom(allSurvivalModes).Draw(t, "sd_mode"),
		SessionState:          rapid.SampledFrom(orchestrator.AllSessionPhases).Draw(t, "sd_state"),
		LastSwitchReason:      rapid.StringMatching(`[a-z_]{0,20}`).Draw(t, "sd_switch"),
		LastRollbackReason:    rapid.StringMatching(`[a-z_]{0,20}`).Draw(t, "sd_rollback"),
	}
}

// genSystemDiagnostic generates a random SystemDiagnostic.
func genSystemDiagnostic(t *rapid.T) *SystemDiagnostic {
	hasActiveTx := rapid.Bool().Draw(t, "sysd_has_tx")
	var activeTx *ActiveTxInfo
	if hasActiveTx {
		activeTx = &ActiveTxInfo{
			TxID:    uuid.New().String(),
			TxType:  rapid.SampledFrom(allTxTypes).Draw(t, "sysd_tx_type"),
			TxPhase: rapid.SampledFrom(commit.AllTxPhases).Draw(t, "sysd_tx_phase"),
		}
	}
	hasSwitchTime := rapid.Bool().Draw(t, "sysd_has_switch_time")
	var switchTime *time.Time
	if hasSwitchTime {
		st := genTime(t, "sysd_switch")
		switchTime = &st
	}
	return &SystemDiagnostic{
		CurrentSurvivalMode:  rapid.SampledFrom(allSurvivalModes).Draw(t, "sysd_mode"),
		LastModeSwitchReason: rapid.StringMatching(`[a-z_]{0,20}`).Draw(t, "sysd_reason"),
		LastModeSwitchTime:   switchTime,
		ActiveSessionCount:   rapid.IntRange(0, 100).Draw(t, "sysd_sessions"),
		ActiveLinkCount:      rapid.IntRange(0, 50).Draw(t, "sysd_links"),
		ActiveTransaction:    activeTx,
	}
}

// genTransactionDiagnostic generates a random TransactionDiagnostic.
func genTransactionDiagnostic(t *rapid.T) *TransactionDiagnostic {
	numPhases := rapid.IntRange(0, 5).Draw(t, "txd_num_phases")
	phaseDurations := make(map[string]time.Duration)
	phaseNames := []string{"preparing", "validating", "shadow_writing", "flipping", "acknowledging"}
	for i := 0; i < numPhases && i < len(phaseNames); i++ {
		dur := time.Duration(rapid.Int64Range(0, int64(10*time.Second)).Draw(t, fmt.Sprintf("txd_dur_%d", i)))
		phaseDurations[phaseNames[i]] = dur
	}
	stuckDur := time.Duration(rapid.Int64Range(0, int64(60*time.Second)).Draw(t, "txd_stuck"))
	return &TransactionDiagnostic{
		TxID:               uuid.New().String(),
		TxType:             rapid.SampledFrom(allTxTypes).Draw(t, "txd_type"),
		CurrentPhase:       rapid.SampledFrom(commit.AllTxPhases).Draw(t, "txd_phase"),
		PhaseDurations:     phaseDurations,
		StuckDuration:      stuckDur,
		TargetSessionID:    rapid.StringMatching(`[a-z0-9]{8,16}`).Draw(t, "txd_session"),
		TargetSurvivalMode: rapid.SampledFrom(allSurvivalModes).Draw(t, "txd_mode"),
	}
}

// jsonRoundTrip marshals v to JSON, checks snake_case keys and RFC 3339 times,
// then unmarshals back into a new value of the same type and returns it.
func jsonRoundTrip[T any](t *rapid.T, v *T, label string) *T {
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("%s: json.Marshal failed: %v", label, err)
	}

	// Check all JSON keys are snake_case
	assertSnakeCaseKeys(t, data, label)

	// Check time fields are RFC 3339
	assertRFC3339TimeFields(t, data, label)

	var result T
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("%s: json.Unmarshal failed: %v", label, err)
	}
	return &result
}

// TestProperty11_JSONRoundTrip verifies JSON marshal → unmarshal produces equivalent objects
// for all data structures, with snake_case keys and RFC 3339 timestamps.
//
// Feature: v2-observability, Property 11: JSON round-trip for all data structures
// **Validates: Requirements 14.1, 14.2, 14.3, 14.4, 14.5**
func TestProperty11_JSONRoundTrip(t *testing.T) {
	// Sub-test: AuditRecord
	t.Run("AuditRecord", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			original := genAuditRecord(t)
			result := jsonRoundTrip(t, original, "AuditRecord")

			if result.AuditID != original.AuditID {
				t.Fatalf("audit_id mismatch: got %s, want %s", result.AuditID, original.AuditID)
			}
			if result.TxID != original.TxID {
				t.Fatalf("tx_id mismatch")
			}
			if result.TxType != original.TxType {
				t.Fatalf("tx_type mismatch")
			}
			if !result.InitiatedAt.Equal(original.InitiatedAt) {
				t.Fatalf("initiated_at mismatch: got %v, want %v", result.InitiatedAt, original.InitiatedAt)
			}
			if !result.FinishedAt.Equal(original.FinishedAt) {
				t.Fatalf("finished_at mismatch")
			}
			if result.FlipSuccess != original.FlipSuccess {
				t.Fatalf("flip_success mismatch")
			}
			if result.RollbackTriggered != original.RollbackTriggered {
				t.Fatalf("rollback_triggered mismatch")
			}
			if result.BudgetVerdict != original.BudgetVerdict {
				t.Fatalf("budget_verdict mismatch")
			}
			if !result.CreatedAt.Equal(original.CreatedAt) {
				t.Fatalf("created_at mismatch")
			}
		})
	})

	// Sub-test: SessionTimelineEntry
	t.Run("SessionTimelineEntry", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			original := genSessionTimelineEntry(t)
			result := jsonRoundTrip(t, original, "SessionTimelineEntry")

			if result.EntryID != original.EntryID {
				t.Fatalf("entry_id mismatch")
			}
			if result.SessionID != original.SessionID {
				t.Fatalf("session_id mismatch")
			}
			if result.FromState != original.FromState {
				t.Fatalf("from_state mismatch")
			}
			if result.ToState != original.ToState {
				t.Fatalf("to_state mismatch")
			}
			if result.SurvivalMode != original.SurvivalMode {
				t.Fatalf("survival_mode mismatch")
			}
			if !result.Timestamp.Equal(original.Timestamp) {
				t.Fatalf("timestamp mismatch: got %v, want %v", result.Timestamp, original.Timestamp)
			}
		})
	})

	// Sub-test: LinkHealthTimelineEntry
	t.Run("LinkHealthTimelineEntry", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			original := genLinkHealthTimelineEntry(t)
			result := jsonRoundTrip(t, original, "LinkHealthTimelineEntry")

			if result.EntryID != original.EntryID {
				t.Fatalf("entry_id mismatch")
			}
			if result.LinkID != original.LinkID {
				t.Fatalf("link_id mismatch")
			}
			if result.HealthScore != original.HealthScore {
				t.Fatalf("health_score mismatch: got %f, want %f", result.HealthScore, original.HealthScore)
			}
			if result.RTTMs != original.RTTMs {
				t.Fatalf("rtt_ms mismatch")
			}
			if result.LossRate != original.LossRate {
				t.Fatalf("loss_rate mismatch")
			}
			if result.JitterMs != original.JitterMs {
				t.Fatalf("jitter_ms mismatch")
			}
			if result.Phase != original.Phase {
				t.Fatalf("phase mismatch")
			}
			if result.EventType != original.EventType {
				t.Fatalf("event_type mismatch")
			}
			if !result.Timestamp.Equal(original.Timestamp) {
				t.Fatalf("timestamp mismatch")
			}
		})
	})

	// Sub-test: PersonaVersionTimelineEntry
	t.Run("PersonaVersionTimelineEntry", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			original := genPersonaVersionTimelineEntry(t)
			result := jsonRoundTrip(t, original, "PersonaVersionTimelineEntry")

			if result.EntryID != original.EntryID {
				t.Fatalf("entry_id mismatch")
			}
			if result.SessionID != original.SessionID {
				t.Fatalf("session_id mismatch")
			}
			if result.PersonaID != original.PersonaID {
				t.Fatalf("persona_id mismatch")
			}
			if result.FromVersion != original.FromVersion {
				t.Fatalf("from_version mismatch: got %d, want %d", result.FromVersion, original.FromVersion)
			}
			if result.ToVersion != original.ToVersion {
				t.Fatalf("to_version mismatch")
			}
			if result.EventType != original.EventType {
				t.Fatalf("event_type mismatch")
			}
			if !result.Timestamp.Equal(original.Timestamp) {
				t.Fatalf("timestamp mismatch")
			}
		})
	})

	// Sub-test: SurvivalModeTimelineEntry
	t.Run("SurvivalModeTimelineEntry", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			original := genSurvivalModeTimelineEntry(t)
			result := jsonRoundTrip(t, original, "SurvivalModeTimelineEntry")

			if result.EntryID != original.EntryID {
				t.Fatalf("entry_id mismatch")
			}
			if result.FromMode != original.FromMode {
				t.Fatalf("from_mode mismatch")
			}
			if result.ToMode != original.ToMode {
				t.Fatalf("to_mode mismatch")
			}
			if string(result.Triggers) != string(original.Triggers) {
				t.Fatalf("triggers mismatch: got %s, want %s", string(result.Triggers), string(original.Triggers))
			}
			if result.TxID != original.TxID {
				t.Fatalf("tx_id mismatch")
			}
			if !result.Timestamp.Equal(original.Timestamp) {
				t.Fatalf("timestamp mismatch")
			}
		})
	})

	// Sub-test: TransactionTimelineEntry
	t.Run("TransactionTimelineEntry", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			original := genTransactionTimelineEntry(t)
			result := jsonRoundTrip(t, original, "TransactionTimelineEntry")

			if result.EntryID != original.EntryID {
				t.Fatalf("entry_id mismatch")
			}
			if result.TxID != original.TxID {
				t.Fatalf("tx_id mismatch")
			}
			if result.FromPhase != original.FromPhase {
				t.Fatalf("from_phase mismatch")
			}
			if result.ToPhase != original.ToPhase {
				t.Fatalf("to_phase mismatch")
			}
			if string(result.PhaseData) != string(original.PhaseData) {
				t.Fatalf("phase_data mismatch")
			}
			if !result.Timestamp.Equal(original.Timestamp) {
				t.Fatalf("timestamp mismatch")
			}
		})
	})

	// Sub-test: SessionDiagnostic
	t.Run("SessionDiagnostic", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			original := genSessionDiagnostic(t)
			result := jsonRoundTrip(t, original, "SessionDiagnostic")

			if result.SessionID != original.SessionID {
				t.Fatalf("session_id mismatch")
			}
			if result.CurrentLinkID != original.CurrentLinkID {
				t.Fatalf("current_link_id mismatch")
			}
			if result.CurrentLinkPhase != original.CurrentLinkPhase {
				t.Fatalf("current_link_phase mismatch")
			}
			if result.CurrentPersonaID != original.CurrentPersonaID {
				t.Fatalf("current_persona_id mismatch")
			}
			if result.CurrentPersonaVersion != original.CurrentPersonaVersion {
				t.Fatalf("current_persona_version mismatch")
			}
			if result.CurrentSurvivalMode != original.CurrentSurvivalMode {
				t.Fatalf("current_survival_mode mismatch")
			}
			if result.SessionState != original.SessionState {
				t.Fatalf("session_state mismatch")
			}
		})
	})

	// Sub-test: SystemDiagnostic
	t.Run("SystemDiagnostic", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			original := genSystemDiagnostic(t)
			result := jsonRoundTrip(t, original, "SystemDiagnostic")

			if result.CurrentSurvivalMode != original.CurrentSurvivalMode {
				t.Fatalf("current_survival_mode mismatch")
			}
			if result.ActiveSessionCount != original.ActiveSessionCount {
				t.Fatalf("active_session_count mismatch")
			}
			if result.ActiveLinkCount != original.ActiveLinkCount {
				t.Fatalf("active_link_count mismatch")
			}
			if original.LastModeSwitchTime != nil {
				if result.LastModeSwitchTime == nil {
					t.Fatalf("last_mode_switch_time should not be nil")
				}
				if !result.LastModeSwitchTime.Equal(*original.LastModeSwitchTime) {
					t.Fatalf("last_mode_switch_time mismatch")
				}
			} else {
				if result.LastModeSwitchTime != nil {
					t.Fatalf("last_mode_switch_time should be nil")
				}
			}
			if original.ActiveTransaction != nil {
				if result.ActiveTransaction == nil {
					t.Fatalf("active_transaction should not be nil")
				}
				if result.ActiveTransaction.TxID != original.ActiveTransaction.TxID {
					t.Fatalf("active_transaction.tx_id mismatch")
				}
				if result.ActiveTransaction.TxType != original.ActiveTransaction.TxType {
					t.Fatalf("active_transaction.tx_type mismatch")
				}
				if result.ActiveTransaction.TxPhase != original.ActiveTransaction.TxPhase {
					t.Fatalf("active_transaction.tx_phase mismatch")
				}
			} else {
				if result.ActiveTransaction != nil {
					t.Fatalf("active_transaction should be nil")
				}
			}
		})
	})

	// Sub-test: TransactionDiagnostic
	t.Run("TransactionDiagnostic", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			original := genTransactionDiagnostic(t)
			result := jsonRoundTrip(t, original, "TransactionDiagnostic")

			if result.TxID != original.TxID {
				t.Fatalf("tx_id mismatch")
			}
			if result.TxType != original.TxType {
				t.Fatalf("tx_type mismatch")
			}
			if result.CurrentPhase != original.CurrentPhase {
				t.Fatalf("current_phase mismatch")
			}
			if result.StuckDuration != original.StuckDuration {
				t.Fatalf("stuck_duration mismatch: got %v, want %v", result.StuckDuration, original.StuckDuration)
			}
			if result.TargetSessionID != original.TargetSessionID {
				t.Fatalf("target_session_id mismatch")
			}
			if result.TargetSurvivalMode != original.TargetSurvivalMode {
				t.Fatalf("target_survival_mode mismatch")
			}
			// Verify phase_durations round-trip
			if len(result.PhaseDurations) != len(original.PhaseDurations) {
				t.Fatalf("phase_durations length mismatch: got %d, want %d", len(result.PhaseDurations), len(original.PhaseDurations))
			}
			for k, v := range original.PhaseDurations {
				rv, ok := result.PhaseDurations[k]
				if !ok {
					t.Fatalf("phase_durations missing key %s", k)
				}
				if rv != v {
					t.Fatalf("phase_durations[%s] mismatch: got %v, want %v", k, rv, v)
				}
			}
		})
	})
}
