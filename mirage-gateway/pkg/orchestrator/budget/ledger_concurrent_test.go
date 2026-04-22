package budget

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"mirage-gateway/pkg/orchestrator/commit"
)

func TestInMemoryLedger_ConcurrentSafety(t *testing.T) {
	ledger := NewInMemoryLedger()
	const goroutines = 10
	const opsPerGoroutine = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			sessionID := fmt.Sprintf("session-%d", id)

			for j := 0; j < opsPerGoroutine; j++ {
				// Record entries
				ledger.Record(&LedgerEntry{
					SessionID: sessionID,
					TxType:    commit.TxTypeLinkMigration,
					CostEstimate: &CostEstimate{
						BandwidthCost: 0.1,
						LatencyCost:   0.1,
						SwitchCost:    0.1,
						TotalCost:     0.3,
					},
					Timestamp: time.Now(),
				})

				ledger.Record(&LedgerEntry{
					SessionID: sessionID,
					TxType:    commit.TxTypeGatewayReassignment,
					CostEstimate: &CostEstimate{
						SwitchCost:      0.1,
						EntryBurnCost:   0.1,
						GatewayLoadCost: 0.1,
						TotalCost:       0.3,
					},
					Timestamp: time.Now(),
				})

				// Read operations
				_ = ledger.SwitchCountInLastHour(sessionID)
				_ = ledger.EntryBurnCountInLastDay(sessionID)

				// Cleanup
				if j%20 == 0 {
					ledger.Cleanup()
				}
			}
		}(i)
	}

	wg.Wait()
}
