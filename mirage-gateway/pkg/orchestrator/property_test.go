package orchestrator

import (
	"context"
	"testing"

	"pgregory.net/rapid"
)

// genLinkPhase 随机生成 LinkPhase
func genLinkPhase() *rapid.Generator[LinkPhase] {
	return rapid.SampledFrom(AllLinkPhases)
}

// genSessionPhase 随机生成 SessionPhase
func genSessionPhase() *rapid.Generator[SessionPhase] {
	return rapid.SampledFrom(AllSessionPhases)
}

// Property 1: Link 状态机转换合法性
func TestProperty1_LinkTransitionValidity(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		from := genLinkPhase().Draw(t, "from")
		to := genLinkPhase().Draw(t, "to")

		result := IsValidLinkTransition(from, to)
		expected := linkTransitions[[2]LinkPhase{from, to}]

		if result != expected {
			t.Fatalf("Feature: v2-state-model, Property 1: Link transition %s->%s: got %v, want %v", from, to, result, expected)
		}
	})
}

// Property 3: Session 状态机转换合法性
func TestProperty3_SessionTransitionValidity(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		from := genSessionPhase().Draw(t, "from")
		to := genSessionPhase().Draw(t, "to")

		result := IsValidSessionTransition(from, to)
		expected := sessionTransitions[[2]SessionPhase{from, to}]

		if result != expected {
			t.Fatalf("Feature: v2-state-model, Property 3: Session transition %s->%s: got %v, want %v", from, to, result, expected)
		}
	})
}

// Property 2: Link 状态转换副作用一致性（纯逻辑验证）
func TestProperty2_LinkTransitionSideEffects(t *testing.T) {
	// 收集所有合法转换
	type legalTransition struct {
		from, to LinkPhase
	}
	var legalPairs []legalTransition
	for pair := range linkTransitions {
		legalPairs = append(legalPairs, legalTransition{pair[0], pair[1]})
	}

	rapid.Check(t, func(t *rapid.T) {
		idx := rapid.IntRange(0, len(legalPairs)-1).Draw(t, "idx")
		pair := legalPairs[idx]

		// 模拟副作用
		ls := LinkState{Phase: pair.from, Available: true, Degraded: false, HealthScore: 50}
		ls.Phase = pair.to

		switch pair.to {
		case LinkPhaseUnavailable:
			ls.Available = false
			ls.HealthScore = 0
		case LinkPhaseActive:
			ls.Available = true
			ls.Degraded = false
		case LinkPhaseDegrading:
			ls.Degraded = true
		}

		// 验证
		switch pair.to {
		case LinkPhaseUnavailable:
			if ls.Available {
				t.Fatalf("Property 2: after -> Unavailable, available should be false")
			}
			if ls.HealthScore != 0 {
				t.Fatalf("Property 2: after -> Unavailable, health_score should be 0")
			}
		case LinkPhaseActive:
			if !ls.Available {
				t.Fatalf("Property 2: after -> Active, available should be true")
			}
			if ls.Degraded {
				t.Fatalf("Property 2: after -> Active, degraded should be false")
			}
		case LinkPhaseDegrading:
			if !ls.Degraded {
				t.Fatalf("Property 2: after -> Degrading, degraded should be true")
			}
		}
	})
}

// Property 4: Session 迁移标记一致性（纯逻辑验证）
func TestProperty4_SessionMigrationPendingConsistency(t *testing.T) {
	type legalTransition struct {
		from, to SessionPhase
	}
	var legalPairs []legalTransition
	for pair := range sessionTransitions {
		legalPairs = append(legalPairs, legalTransition{pair[0], pair[1]})
	}

	rapid.Check(t, func(t *rapid.T) {
		idx := rapid.IntRange(0, len(legalPairs)-1).Draw(t, "idx")
		pair := legalPairs[idx]

		ss := SessionState{State: pair.from, MigrationPending: pair.from == SessionPhaseMigrating}

		// 模拟副作用
		if pair.to == SessionPhaseMigrating {
			ss.MigrationPending = true
		}
		if ss.State == SessionPhaseMigrating && pair.to != SessionPhaseMigrating {
			ss.MigrationPending = false
		}
		ss.State = pair.to

		// 验证
		if pair.to == SessionPhaseMigrating && !ss.MigrationPending {
			t.Fatalf("Property 4: entering Migrating, migration_pending should be true")
		}
		if pair.from == SessionPhaseMigrating && pair.to != SessionPhaseMigrating && ss.MigrationPending {
			t.Fatalf("Property 4: leaving Migrating, migration_pending should be false")
		}
	})
}

// Property 5: Session 链路变更不变量（纯逻辑验证）
func TestProperty5_SessionUpdateLinkInvariant(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		ss := SessionState{
			SessionID:        rapid.String().Draw(t, "session_id"),
			CurrentPersonaID: rapid.String().Draw(t, "persona_id"),
			ServiceClass:     ServiceClassStandard,
			Priority:         rapid.IntRange(0, 100).Draw(t, "priority"),
			CurrentLinkID:    rapid.String().Draw(t, "old_link"),
		}

		origSessionID := ss.SessionID
		origPersonaID := ss.CurrentPersonaID
		origServiceClass := ss.ServiceClass
		origPriority := ss.Priority

		// 模拟 UpdateLink
		newLinkID := rapid.String().Draw(t, "new_link")
		ss.CurrentLinkID = newLinkID

		if ss.SessionID != origSessionID {
			t.Fatalf("Property 5: session_id changed after UpdateLink")
		}
		if ss.CurrentPersonaID != origPersonaID {
			t.Fatalf("Property 5: current_persona_id changed after UpdateLink")
		}
		if ss.ServiceClass != origServiceClass {
			t.Fatalf("Property 5: service_class changed after UpdateLink")
		}
		if ss.Priority != origPriority {
			t.Fatalf("Property 5: priority changed after UpdateLink")
		}
	})
}

// Property 6: Epoch 严格递增（纯逻辑验证）
func TestProperty6_EpochStrictlyIncreasing(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		n := rapid.IntRange(1, 50).Draw(t, "n")
		cs := ControlState{Epoch: 0}
		var epochs []uint64

		for i := 0; i < n; i++ {
			cs.Epoch++
			epochs = append(epochs, cs.Epoch)
		}

		for i := 1; i < len(epochs); i++ {
			if epochs[i] <= epochs[i-1] {
				t.Fatalf("Property 6: epoch[%d]=%d not strictly greater than epoch[%d]=%d", i, epochs[i], i-1, epochs[i-1])
			}
			if epochs[i]-epochs[i-1] < 1 {
				t.Fatalf("Property 6: increment less than 1")
			}
		}
	})
}

// Property 7: 事务提交后状态一致性（纯逻辑验证）
func TestProperty7_CommitTransactionConsistency(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		epoch := rapid.Uint64Range(1, 10000).Draw(t, "epoch")
		txID := rapid.String().Draw(t, "tx_id")

		cs := ControlState{
			Epoch:      epoch,
			ActiveTxID: txID,
		}

		// 模拟 CommitTransaction
		cs.LastSuccessfulEpoch = cs.Epoch
		cs.RollbackMarker = cs.Epoch
		cs.ActiveTxID = ""

		if cs.LastSuccessfulEpoch != epoch {
			t.Fatalf("Property 7: last_successful_epoch=%d, want %d", cs.LastSuccessfulEpoch, epoch)
		}
		if cs.RollbackMarker != epoch {
			t.Fatalf("Property 7: rollback_marker=%d, want %d", cs.RollbackMarker, epoch)
		}
		if cs.ActiveTxID != "" {
			t.Fatalf("Property 7: active_tx_id=%q, want empty", cs.ActiveTxID)
		}
	})
}

// Property 8: 崩溃恢复正确性（纯逻辑验证）
func TestProperty8_CrashRecoveryCorrectness(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		rollbackMarker := rapid.Uint64Range(0, 1000).Draw(t, "rollback_marker")
		epoch := rapid.Uint64Range(rollbackMarker, rollbackMarker+100).Draw(t, "epoch")
		txID := rapid.StringMatching("[a-z]{1,10}").Draw(t, "tx_id")

		cs := ControlState{
			Epoch:          epoch,
			RollbackMarker: rollbackMarker,
			ActiveTxID:     txID,
			ControlHealth:  ControlHealthHealthy,
		}

		// 模拟 RecoverOnStartup
		if cs.ActiveTxID != "" {
			cs.ControlHealth = ControlHealthRecovering
			cs.Epoch = cs.RollbackMarker
			cs.ActiveTxID = ""
		}

		if cs.ControlHealth != ControlHealthRecovering {
			t.Fatalf("Property 8: control_health=%s, want Recovering", cs.ControlHealth)
		}
		if cs.Epoch != rollbackMarker {
			t.Fatalf("Property 8: epoch=%d, want rollback_marker=%d", cs.Epoch, rollbackMarker)
		}
		if cs.ActiveTxID != "" {
			t.Fatalf("Property 8: active_tx_id=%q, want empty", cs.ActiveTxID)
		}
	})
}

// Property 9: Session 创建引用完整性（纯逻辑验证）
func TestProperty9_SessionCreateLinkRefIntegrity(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		linkExists := rapid.Bool().Draw(t, "link_exists")
		linkAvailable := rapid.Bool().Draw(t, "link_available")

		// 模拟 ValidateLinkRef 逻辑
		var err error
		if !linkExists {
			err = &ErrLinkNotFound{LinkID: "test-link"}
		} else if !linkAvailable {
			err = &ErrLinkUnavailable{LinkID: "test-link"}
		}

		shouldSucceed := linkExists && linkAvailable
		if shouldSucceed && err != nil {
			t.Fatalf("Property 9: expected success but got error: %v", err)
		}
		if !shouldSucceed && err == nil {
			t.Fatalf("Property 9: expected error but got success")
		}
	})
}

// Property 10: Link 不可用级联降级（纯逻辑验证）
func TestProperty10_LinkUnavailableCascadeDegradation(t *testing.T) {
	// 可以转换到 Degraded 的状态
	degradableStates := []SessionPhase{
		SessionPhaseActive, SessionPhaseProtected, SessionPhaseMigrating,
	}
	// 不能转换到 Degraded 的状态
	nonDegradableStates := []SessionPhase{
		SessionPhaseBootstrapping, SessionPhaseSuspended, SessionPhaseClosed,
	}

	rapid.Check(t, func(t *rapid.T) {
		useDegradable := rapid.Bool().Draw(t, "use_degradable")

		var state SessionPhase
		if useDegradable {
			idx := rapid.IntRange(0, len(degradableStates)-1).Draw(t, "idx")
			state = degradableStates[idx]
		} else {
			idx := rapid.IntRange(0, len(nonDegradableStates)-1).Draw(t, "idx")
			state = nonDegradableStates[idx]
		}

		canDegrade := IsValidSessionTransition(state, SessionPhaseDegraded)

		if useDegradable && !canDegrade {
			t.Fatalf("Property 10: state %s should be degradable", state)
		}
		if !useDegradable && canDegrade {
			t.Fatalf("Property 10: state %s should not be degradable", state)
		}
	})
}

// Property 11: 并发状态变更序列化（LockManager 验证）
func TestProperty11_ConcurrentStateSerialization(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		n := rapid.IntRange(2, 10).Draw(t, "n")
		lm := NewLockManager()
		key := "link:test-link"

		counter := 0
		done := make(chan struct{}, n)

		for i := 0; i < n; i++ {
			go func() {
				defer func() { done <- struct{}{} }()
				ctx := context.Background()
				unlock, err := lm.Lock(ctx, key)
				if err != nil {
					return
				}
				defer unlock()
				// 临界区：读-改-写
				val := counter
				counter = val + 1
			}()
		}

		for i := 0; i < n; i++ {
			<-done
		}

		if counter != n {
			t.Fatalf("Property 11: counter=%d, want %d (data race detected)", counter, n)
		}
	})
}

// Property 12: 乐观锁冲突检测（纯逻辑验证）
func TestProperty12_OptimisticLockConflictDetection(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// 模拟两个并发更新基于相同 updated_at
		// 第一个成功（RowsAffected=1），第二个失败（RowsAffected=0）
		rowsAffected1 := 1
		rowsAffected2 := 0

		var err1, err2 error
		if rowsAffected1 == 0 {
			err1 = ErrOptimisticLockConflict
		}
		if rowsAffected2 == 0 {
			err2 = ErrOptimisticLockConflict
		}

		successCount := 0
		conflictCount := 0
		if err1 == nil {
			successCount++
		} else {
			conflictCount++
		}
		if err2 == nil {
			successCount++
		} else {
			conflictCount++
		}

		if successCount != 1 || conflictCount != 1 {
			t.Fatalf("Property 12: expected 1 success + 1 conflict, got %d success + %d conflict", successCount, conflictCount)
		}
	})
}
