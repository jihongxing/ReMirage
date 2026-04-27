package api

import (
	"sync"
	"testing"
	"time"

	"pgregory.net/rapid"
)

// =============================================================================
// Preservation Property Tests — F-P1-06 配额桶正常消费行为基线保全
//
// 这些测试在未修复代码上必须 PASS，确认正常路径行为作为基线。
// 修复后重新运行，确认无回归。
// =============================================================================

// TestPreservation_NormalConsume_BalanceDecrements 属性测试：
// 对所有 initialQuota > consumeAmount > 0 的场景，
// Consume 返回 true 且余额 == initialQuota - consumeAmount。
//
// **Validates: Requirements 3.9**
func TestPreservation_NormalConsume_BalanceDecrements(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		consumeAmount := rapid.Uint64Range(1, 999_999).Draw(t, "consumeAmount")
		// initialQuota 严格大于 consumeAmount，确保不触发耗尽
		initialQuota := rapid.Uint64Range(consumeAmount+1, consumeAmount+1_000_000).Draw(t, "initialQuota")

		mgr := NewQuotaBucketManager()
		mgr.UpdateQuota("user", initialQuota)

		ok := mgr.Consume("user", consumeAmount)
		if !ok {
			t.Fatalf("Consume(%d, %d) 应返回 true，实际返回 false", initialQuota, consumeAmount)
		}

		remaining, exists := mgr.GetRemaining("user")
		if !exists {
			t.Fatal("用户应存在")
		}

		expected := initialQuota - consumeAmount
		if remaining != expected {
			t.Fatalf("余额应为 %d，实际: %d (initialQuota=%d, consumeAmount=%d)",
				expected, remaining, initialQuota, consumeAmount)
		}

		// 正常消费后不应标记耗尽
		if mgr.IsExhausted("user") {
			t.Fatalf("消费后仍有余额 %d，不应标记耗尽", remaining)
		}
	})
}

// TestPreservation_UnknownUser_Rejected 属性测试：
// 对所有未知用户，Consume 返回 false。
//
// **Validates: Requirements 3.9**
func TestPreservation_UnknownUser_Rejected(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		userID := rapid.StringMatching(`[a-z]{3,20}`).Draw(t, "userID")
		amount := rapid.Uint64Range(1, 1_000_000).Draw(t, "amount")

		mgr := NewQuotaBucketManager()
		// 不调用 UpdateQuota — userID 为未知用户

		ok := mgr.Consume(userID, amount)
		if ok {
			t.Fatalf("未知用户 %q Consume(%d) 应返回 false", userID, amount)
		}
	})
}

// TestPreservation_OverConsume_Rejected_ExhaustionTriggered 属性测试：
// 对所有 consumeAmount > initialQuota > 0 的场景，
// Consume 返回 false 且触发耗尽回调。
//
// **Validates: Requirements 3.9**
func TestPreservation_OverConsume_Rejected_ExhaustionTriggered(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		initialQuota := rapid.Uint64Range(1, 999_999).Draw(t, "initialQuota")
		// consumeAmount 严格大于 initialQuota
		consumeAmount := rapid.Uint64Range(initialQuota+1, initialQuota+1_000_000).Draw(t, "consumeAmount")

		mgr := NewQuotaBucketManager()

		var callbackMu sync.Mutex
		callbackUserID := ""
		callbackDone := make(chan struct{}, 1)

		mgr.SetOnExhausted(func(userID string) {
			callbackMu.Lock()
			callbackUserID = userID
			callbackMu.Unlock()
			select {
			case callbackDone <- struct{}{}:
			default:
			}
		})

		mgr.UpdateQuota("user", initialQuota)

		ok := mgr.Consume("user", consumeAmount)
		if ok {
			t.Fatalf("Consume(%d) 超过配额 %d 应返回 false", consumeAmount, initialQuota)
		}

		// 应标记耗尽
		if !mgr.IsExhausted("user") {
			t.Fatal("超额消费后应标记耗尽")
		}

		// 等待回调触发
		select {
		case <-callbackDone:
			callbackMu.Lock()
			uid := callbackUserID
			callbackMu.Unlock()
			if uid != "user" {
				t.Fatalf("回调 userID 应为 user，实际: %s", uid)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("onExhausted 回调未在 2 秒内触发")
		}
	})
}
