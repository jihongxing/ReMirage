package strategy

import (
	"testing"

	"pgregory.net/rapid"
)

// Feature: v1-tiered-service, Task 6.4: 恢复排序后 Diamond 在前，Standard 在后
// **Validates: Requirements 5**
func TestProperty_RecoverySortPriority(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		numSessions := rapid.IntRange(1, 50).Draw(t, "num_sessions")
		sessions := make([]GatewaySessionWithLevel, numSessions)
		for i := range sessions {
			sessions[i] = GatewaySessionWithLevel{
				UserID:    rapid.StringMatching(`user-[a-z]{4}`).Draw(t, "user_id"),
				GatewayID: rapid.StringMatching(`gw-[a-z]{4}`).Draw(t, "gw_id"),
				CellLevel: rapid.IntRange(1, 3).Draw(t, "cell_level"),
			}
		}

		SortSessionsByPriority(sessions)

		// Property: sorted in descending order of CellLevel
		for i := 1; i < len(sessions); i++ {
			if sessions[i].CellLevel > sessions[i-1].CellLevel {
				t.Fatalf("sort invariant violated at index %d: level %d > level %d",
					i, sessions[i].CellLevel, sessions[i-1].CellLevel)
			}
		}

		// Property: if Diamond exists, it must be at the front
		if len(sessions) > 0 {
			hasDiamond := false
			for _, s := range sessions {
				if s.CellLevel == 3 {
					hasDiamond = true
					break
				}
			}
			if hasDiamond && sessions[0].CellLevel != 3 {
				t.Fatal("Diamond user should be at the front after sorting")
			}
		}
	})
}
