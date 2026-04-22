package budget

import (
	"testing"
	"time"

	"mirage-gateway/pkg/orchestrator/commit"

	"pgregory.net/rapid"
)

// allTxTypes for random generation
var ledgerTxTypes = []commit.TxType{
	commit.TxTypeLinkMigration,
	commit.TxTypeGatewayReassignment,
	commit.TxTypePersonaSwitch,
	commit.TxTypeSurvivalModeSwitch,
}

// genLedgerEntry generates a random LedgerEntry with a timestamp offset from now.
// offsetHours range: [-48, +1] to cover entries older than 24h, within 24h, and within 1h.
func genLedgerEntry(t *rapid.T) *LedgerEntry {
	sessionID := rapid.SampledFrom([]string{"s1", "s2", "s3"}).Draw(t, "sessionID")
	txType := rapid.SampledFrom(ledgerTxTypes).Draw(t, "txType")
	// offset in minutes: -2880 (48h ago) to +30 (30min in future, still "within 1h")
	offsetMin := rapid.IntRange(-2880, 30).Draw(t, "offsetMin")
	ts := time.Now().Add(time.Duration(offsetMin) * time.Minute)
	return &LedgerEntry{
		SessionID:    sessionID,
		TxType:       txType,
		CostEstimate: &CostEstimate{},
		Timestamp:    ts,
	}
}

// TestProperty6_SlidingWindowCountCorrectness verifies SwitchCountInLastHour and
// EntryBurnCountInLastDay return correct counts for random entry sets.
//
// **Validates: Requirements 7.2, 7.3**
func TestProperty6_SlidingWindowCountCorrectness(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		ledger := NewInMemoryLedger()
		n := rapid.IntRange(0, 50).Draw(t, "numEntries")
		entries := make([]*LedgerEntry, n)
		for i := 0; i < n; i++ {
			entries[i] = genLedgerEntry(t)
			ledger.Record(entries[i])
		}

		querySession := rapid.SampledFrom([]string{"s1", "s2", "s3"}).Draw(t, "querySession")
		now := time.Now()
		oneHourAgo := now.Add(-1 * time.Hour)
		oneDayAgo := now.Add(-24 * time.Hour)

		// Expected switch count: LinkMigration + GatewayReassignment within last hour
		expectedSwitch := 0
		for _, e := range entries {
			if e.SessionID == querySession && !e.Timestamp.Before(oneHourAgo) &&
				(e.TxType == commit.TxTypeLinkMigration || e.TxType == commit.TxTypeGatewayReassignment) {
				expectedSwitch++
			}
		}

		// Expected entry burn count: GatewayReassignment within last 24 hours
		expectedBurn := 0
		for _, e := range entries {
			if e.SessionID == querySession && !e.Timestamp.Before(oneDayAgo) &&
				e.TxType == commit.TxTypeGatewayReassignment {
				expectedBurn++
			}
		}

		gotSwitch := ledger.SwitchCountInLastHour(querySession)
		gotBurn := ledger.EntryBurnCountInLastDay(querySession)

		if gotSwitch != expectedSwitch {
			t.Fatalf("SwitchCountInLastHour(%q): got %d, expected %d", querySession, gotSwitch, expectedSwitch)
		}
		if gotBurn != expectedBurn {
			t.Fatalf("EntryBurnCountInLastDay(%q): got %d, expected %d", querySession, gotBurn, expectedBurn)
		}
	})
}

// TestProperty7_LedgerCleanupCorrectness verifies that after Cleanup, no entries
// older than 24h remain and all entries within 24h are preserved.
//
// **Validates: Requirements 7.4**
func TestProperty7_LedgerCleanupCorrectness(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		ledger := NewInMemoryLedger()
		n := rapid.IntRange(0, 50).Draw(t, "numEntries")
		entries := make([]*LedgerEntry, n)
		for i := 0; i < n; i++ {
			entries[i] = genLedgerEntry(t)
			ledger.Record(entries[i])
		}

		ledger.Cleanup()

		now := time.Now()
		oneDayAgo := now.Add(-24 * time.Hour)

		// Count expected survivors: entries with timestamp >= 24h ago
		expectedCount := 0
		for _, e := range entries {
			if !e.Timestamp.Before(oneDayAgo) {
				expectedCount++
			}
		}

		// Verify via ledger internal state
		ledger.mu.Lock()
		remaining := make([]*LedgerEntry, len(ledger.entries))
		copy(remaining, ledger.entries)
		ledger.mu.Unlock()

		if len(remaining) != expectedCount {
			t.Fatalf("after Cleanup: got %d entries, expected %d", len(remaining), expectedCount)
		}

		// Verify no entry older than 24h remains
		for _, e := range remaining {
			if e.Timestamp.Before(oneDayAgo) {
				t.Fatalf("entry with timestamp %v should have been cleaned (cutoff %v)", e.Timestamp, oneDayAgo)
			}
		}
	})
}
