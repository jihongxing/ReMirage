package api

import (
	"sync"
	"testing"
	"time"
)

// TestConsume_ExactExhaustion 确定性单元测试：验证 initialQuota == consumeAmount 时
// Consume 返回 true 且 IsExhausted 返回 true、onExhausted 回调触发。
//
// Bug Condition C9: 当 remaining == bytes 且 CAS 成功时，Exhausted 标记未设置为 1。
// 预期在未修复代码上确定性 FAIL。
//
// **Validates: Requirements 1.9**
func TestConsume_ExactExhaustion(t *testing.T) {
	mgr := NewQuotaBucketManager()

	var callbackCalled sync.WaitGroup
	callbackCalled.Add(1)
	callbackUserID := ""

	mgr.SetOnExhausted(func(userID string) {
		callbackUserID = userID
		callbackCalled.Done()
	})

	// 设置配额 100，消费 100 — 恰好耗尽
	mgr.UpdateQuota("testuser", 100)

	ok := mgr.Consume("testuser", 100)
	if !ok {
		t.Fatal("Consume(100, 100) 应返回 true")
	}

	// 验证剩余为 0
	remaining, exists := mgr.GetRemaining("testuser")
	if !exists {
		t.Fatal("用户应存在")
	}
	if remaining != 0 {
		t.Fatalf("剩余应为 0，实际: %d", remaining)
	}

	// 核心断言：IsExhausted 应返回 true
	// Bug: 未修复代码中 Consume(100,100) CAS 成功后直接返回 true，
	// 未设置 Exhausted=1，导致 IsExhausted 返回 false
	if !mgr.IsExhausted("testuser") {
		t.Fatal("Consume(100, 100) 后 IsExhausted 应返回 true，但返回了 false — 确认 Bug Condition C9")
	}

	// 验证回调触发
	done := make(chan struct{})
	go func() {
		callbackCalled.Wait()
		close(done)
	}()

	select {
	case <-done:
		if callbackUserID != "testuser" {
			t.Fatalf("回调 userID 应为 testuser，实际: %s", callbackUserID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("onExhausted 回调未在 2 秒内触发 — 确认 Bug Condition C9")
	}
}
