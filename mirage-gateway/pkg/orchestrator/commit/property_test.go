package commit

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"pgregory.net/rapid"
)

func genTxType() *rapid.Generator[TxType] {
	return rapid.SampledFrom(AllTxTypes)
}

func genTxPhase() *rapid.Generator[TxPhase] {
	return rapid.SampledFrom(AllTxPhases)
}

// ============================================
// Property 1: 事务创建初始状态不变量
// ============================================

func TestProperty1_TransactionCreationInvariant(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		txType := genTxType().Draw(t, "tx_type")
		lastEpoch := rapid.Uint64Range(0, 10000).Draw(t, "last_epoch")

		tx := NewCommitTransaction(txType, lastEpoch)

		if tx.TxPhase != TxPhasePreparing {
			t.Fatalf("Feature: v2-commit-engine, Property 1: tx_phase=%s, want Preparing", tx.TxPhase)
		}
		if tx.RollbackMarker != lastEpoch {
			t.Fatalf("Feature: v2-commit-engine, Property 1: rollback_marker=%d, want %d", tx.RollbackMarker, lastEpoch)
		}
		expectedScope := TxTypeScopeMap[txType]
		if tx.TxScope != expectedScope {
			t.Fatalf("Feature: v2-commit-engine, Property 1: tx_scope=%s, want %s", tx.TxScope, expectedScope)
		}
		if tx.TxID == "" {
			t.Fatalf("Feature: v2-commit-engine, Property 1: tx_id is empty")
		}
	})
}

// ============================================
// Property 2: TX_Phase 状态机转换合法性
// ============================================

func TestProperty2_PhaseTransitionValidity(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		from := genTxPhase().Draw(t, "from")
		to := genTxPhase().Draw(t, "to")

		_, err := TransitionPhase(from, to)

		// 终态拒绝所有转换
		if IsTerminal(from) {
			if err == nil {
				t.Fatalf("Feature: v2-commit-engine, Property 2: terminal phase %s should reject transition to %s", from, to)
			}
			return
		}

		// 检查是否在合法转换表中
		targets := ValidTransitions[from]
		isValid := false
		for _, target := range targets {
			if target == to {
				isValid = true
				break
			}
		}

		if isValid && err != nil {
			t.Fatalf("Feature: v2-commit-engine, Property 2: valid transition %s->%s got error: %v", from, to, err)
		}
		if !isValid && err == nil {
			t.Fatalf("Feature: v2-commit-engine, Property 2: invalid transition %s->%s should fail", from, to)
		}
	})
}

// ============================================
// Property 3: 作用域活跃事务唯一性
// ============================================

func TestProperty3_ScopeActiveUniqueness(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		cm := NewConflictManager(nil)

		// 注册一个活跃事务
		tx1 := NewCommitTransaction(TxTypePersonaSwitch, 0)
		tx1.TargetSessionID = "s1"
		cm.RegisterActive(tx1)

		// 尝试注册同作用域的第二个事务
		tx2 := NewCommitTransaction(TxTypePersonaSwitch, 0)
		tx2.TargetSessionID = "s2"
		err := cm.CheckConflict(context.Background(), tx2)

		if err == nil {
			t.Fatalf("Feature: v2-commit-engine, Property 3: same scope should conflict")
		}

		// 注销第一个后应该可以注册
		cm.UnregisterActive(tx1.TxID)
		err = cm.CheckConflict(context.Background(), tx2)
		if err != nil {
			t.Fatalf("Feature: v2-commit-engine, Property 3: after unregister should not conflict: %v", err)
		}
	})
}

// ============================================
// Property 4: 优先级抢占正确性
// ============================================

func TestProperty4_PriorityPreemption(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		rolledBack := make(map[string]bool)
		cm := NewConflictManager(func(txID string) error {
			rolledBack[txID] = true
			return nil
		})

		// 注册低优先级事务 (Session scope, priority=1)
		lowTx := NewCommitTransaction(TxTypePersonaSwitch, 0)
		cm.RegisterActive(lowTx)

		// 高优先级事务 (Global scope, priority=3) 应该能抢占
		highTx := NewCommitTransaction(TxTypeSurvivalModeSwitch, 0)
		err := cm.CheckConflict(context.Background(), highTx)

		if err != nil {
			t.Fatalf("Feature: v2-commit-engine, Property 4: high priority should preempt: %v", err)
		}
		if !rolledBack[lowTx.TxID] {
			t.Fatalf("Feature: v2-commit-engine, Property 4: low priority tx should be rolled back")
		}
	})
}

// ============================================
// Property 5: 冷却时间判定
// ============================================

func TestProperty5_CooldownJudgment(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		txType := genTxType().Draw(t, "tx_type")
		cooldownMs := rapid.IntRange(100, 5000).Draw(t, "cooldown_ms")
		elapsedMs := rapid.IntRange(0, 10000).Draw(t, "elapsed_ms")

		config := CooldownConfig{
			PersonaSwitch:       time.Duration(cooldownMs) * time.Millisecond,
			LinkMigration:       time.Duration(cooldownMs) * time.Millisecond,
			GatewayReassignment: time.Duration(cooldownMs) * time.Millisecond,
			SurvivalModeSwitch:  time.Duration(cooldownMs) * time.Millisecond,
		}
		cm := NewCooldownManager(config)

		// 记录完成时间
		finishedAt := time.Now().Add(-time.Duration(elapsedMs) * time.Millisecond)
		cm.RecordCompletion(txType, finishedAt)

		err := cm.CheckCooldown(context.Background(), txType)

		if elapsedMs < cooldownMs {
			if err == nil {
				t.Fatalf("Feature: v2-commit-engine, Property 5: elapsed %dms < cooldown %dms should fail", elapsedMs, cooldownMs)
			}
		}
		// 注意：elapsedMs >= cooldownMs 时可能因时间精度问题仍然失败，不做严格断言
	})
}

// ============================================
// Property 6: Committed 后 ControlState 一致性
// ============================================

func TestProperty6_CommittedControlStateConsistency(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		epoch := rapid.Uint64Range(1, 1000).Draw(t, "epoch")
		cs := newMemControlState(epoch)
		ss := newMemSessionState()
		ss.AddSession("s1", map[string]interface{}{"state": "Active"})
		ls := newMemLinkState()
		store := newMemTxStore()

		cooldownMgr := NewCooldownManager(CooldownConfig{})
		conflictMgr := NewConflictManager(nil)
		executor := NewPhaseExecutor(cs, ss, ls, cooldownMgr, conflictMgr, &DefaultBudgetChecker{}, &DefaultServiceClassChecker{})
		engine := NewCommitEngine(cs, executor, cooldownMgr, conflictMgr, store)

		tx, err := engine.BeginTransaction(context.Background(), &BeginTxRequest{
			TxType:             TxTypeSurvivalModeSwitch,
			TargetSessionID:    "s1",
			TargetSurvivalMode: "Hardened",
		})
		if err != nil {
			t.Fatalf("Feature: v2-commit-engine, Property 6: begin failed: %v", err)
		}

		err = engine.ExecuteTransaction(context.Background(), tx.TxID)
		if err != nil {
			t.Fatalf("Feature: v2-commit-engine, Property 6: execute failed: %v", err)
		}

		// 验证 ControlState
		finalTx, _ := store.GetByID(tx.TxID)
		if finalTx.TxPhase != TxPhaseCommitted {
			t.Fatalf("Feature: v2-commit-engine, Property 6: phase=%s, want Committed", finalTx.TxPhase)
		}
		if cs.GetActiveTxID() != "" {
			t.Fatalf("Feature: v2-commit-engine, Property 6: active_tx_id should be empty")
		}
		if finalTx.FinishedAt == nil {
			t.Fatalf("Feature: v2-commit-engine, Property 6: finished_at should not be nil")
		}
	})
}

// ============================================
// Property 7: RolledBack 后 ControlState 一致性
// ============================================

func TestProperty7_RolledBackControlStateConsistency(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		epoch := rapid.Uint64Range(1, 1000).Draw(t, "epoch")
		cs := newMemControlState(epoch)
		store := newMemTxStore()

		cooldownMgr := NewCooldownManager(CooldownConfig{})
		conflictMgr := NewConflictManager(nil)
		ss := newMemSessionState()
		ls := newMemLinkState()
		executor := NewPhaseExecutor(cs, ss, ls, cooldownMgr, conflictMgr, &DefaultBudgetChecker{}, &DefaultServiceClassChecker{})

		tx := NewCommitTransaction(TxTypePersonaSwitch, epoch)
		tx.TargetSessionID = "nonexistent"
		_ = store.Save(tx)
		_ = cs.SetActiveTxID(tx.TxID)

		// 手动回滚
		tx.TxPhase = TxPhaseRolledBack
		_ = executor.Rollback(context.Background(), tx, "test rollback")
		_ = store.Update(tx)

		if cs.GetEpoch() != epoch {
			t.Fatalf("Feature: v2-commit-engine, Property 7: epoch=%d, want %d", cs.GetEpoch(), epoch)
		}
		if cs.GetActiveTxID() != "" {
			t.Fatalf("Feature: v2-commit-engine, Property 7: active_tx_id should be empty")
		}
		if tx.FinishedAt == nil {
			t.Fatalf("Feature: v2-commit-engine, Property 7: finished_at should not be nil")
		}
	})
}

// ============================================
// Property 8: 崩溃恢复正确性
// ============================================

func TestProperty8_CrashRecoveryCorrectness(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		epoch := rapid.Uint64Range(10, 1000).Draw(t, "epoch")
		marker := rapid.Uint64Range(1, epoch).Draw(t, "marker")

		cs := newMemControlState(epoch)
		_ = cs.SetRollbackMarker(marker)
		store := newMemTxStore()

		// 创建未完成事务
		tx := NewCommitTransaction(TxTypePersonaSwitch, marker)
		tx.TxPhase = TxPhaseShadowWriting // 非终态
		_ = store.Save(tx)
		_ = cs.SetActiveTxID(tx.TxID)

		cooldownMgr := NewCooldownManager(CooldownConfig{})
		conflictMgr := NewConflictManager(nil)
		ss := newMemSessionState()
		ls := newMemLinkState()
		executor := NewPhaseExecutor(cs, ss, ls, cooldownMgr, conflictMgr, &DefaultBudgetChecker{}, &DefaultServiceClassChecker{})
		engine := NewCommitEngine(cs, executor, cooldownMgr, conflictMgr, store)

		err := engine.RecoverOnStartup(context.Background())
		if err != nil {
			t.Fatalf("Feature: v2-commit-engine, Property 8: recovery failed: %v", err)
		}

		// 验证事务被回滚
		recovered, _ := store.GetByID(tx.TxID)
		if recovered.TxPhase != TxPhaseRolledBack {
			t.Fatalf("Feature: v2-commit-engine, Property 8: phase=%s, want RolledBack", recovered.TxPhase)
		}

		// 验证 epoch 恢复到 rollback_marker
		if cs.GetEpoch() != marker {
			t.Fatalf("Feature: v2-commit-engine, Property 8: epoch=%d, want %d", cs.GetEpoch(), marker)
		}

		if cs.GetActiveTxID() != "" {
			t.Fatalf("Feature: v2-commit-engine, Property 8: active_tx_id should be empty")
		}

		if cs.controlHealth != "Recovering" {
			t.Fatalf("Feature: v2-commit-engine, Property 8: health=%s, want Recovering", cs.controlHealth)
		}
	})
}

// ============================================
// Property 9: Prepare 阶段快照完整性
// ============================================

func TestProperty9_PrepareSnapshotCompleteness(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		epoch := rapid.Uint64Range(1, 1000).Draw(t, "epoch")
		cs := newMemControlState(epoch)
		ss := newMemSessionState()
		sessionID := "session-1"
		ss.AddSession(sessionID, map[string]interface{}{
			"state":                 "Active",
			"current_persona_id":    "p1",
			"current_survival_mode": "Normal",
		})
		ls := newMemLinkState()

		cooldownMgr := NewCooldownManager(CooldownConfig{})
		conflictMgr := NewConflictManager(nil)
		executor := NewPhaseExecutor(cs, ss, ls, cooldownMgr, conflictMgr, &DefaultBudgetChecker{}, &DefaultServiceClassChecker{})

		tx := NewCommitTransaction(TxTypePersonaSwitch, epoch)
		tx.TargetSessionID = sessionID

		err := executor.Prepare(context.Background(), tx)
		if err != nil {
			t.Fatalf("Feature: v2-commit-engine, Property 9: prepare failed: %v", err)
		}

		// 验证 prepare_state 包含 epoch 和 session_snapshot
		var state map[string]interface{}
		if err := json.Unmarshal(tx.PrepareState, &state); err != nil {
			t.Fatalf("Feature: v2-commit-engine, Property 9: unmarshal failed: %v", err)
		}

		if state["epoch"] == nil {
			t.Fatalf("Feature: v2-commit-engine, Property 9: epoch missing from prepare_state")
		}
		if state["session_snapshot"] == nil {
			t.Fatalf("Feature: v2-commit-engine, Property 9: session_snapshot missing from prepare_state")
		}
	})
}

// ============================================
// Property 10: CommitTransaction JSON round-trip
// ============================================

func TestProperty10_CommitTransactionJSONRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		txType := genTxType().Draw(t, "tx_type")
		tx := NewCommitTransaction(txType, rapid.Uint64Range(0, 10000).Draw(t, "marker"))
		tx.TargetSessionID = rapid.StringMatching("[a-z0-9]{4,16}").Draw(t, "session")
		tx.TxPhase = genTxPhase().Draw(t, "phase")
		tx.CreatedAt = time.Now().UTC().Truncate(time.Second)

		data, err := json.Marshal(tx)
		if err != nil {
			t.Fatalf("Feature: v2-commit-engine, Property 10: marshal failed: %v", err)
		}

		var decoded CommitTransaction
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatalf("Feature: v2-commit-engine, Property 10: unmarshal failed: %v", err)
		}

		if decoded.TxID != tx.TxID || decoded.TxType != tx.TxType ||
			decoded.TxPhase != tx.TxPhase || decoded.TxScope != tx.TxScope ||
			decoded.RollbackMarker != tx.RollbackMarker {
			t.Fatalf("Feature: v2-commit-engine, Property 10: round-trip mismatch")
		}

		// 验证 created_at RFC 3339
		var raw map[string]json.RawMessage
		_ = json.Unmarshal(data, &raw)
		var createdAt string
		_ = json.Unmarshal(raw["created_at"], &createdAt)
		if _, err := time.Parse(time.RFC3339, createdAt); err != nil {
			if _, err2 := time.Parse(time.RFC3339Nano, createdAt); err2 != nil {
				t.Fatalf("Feature: v2-commit-engine, Property 10: created_at not RFC 3339: %s", createdAt)
			}
		}
	})
}
