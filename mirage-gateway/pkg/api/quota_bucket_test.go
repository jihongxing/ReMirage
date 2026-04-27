package api

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"pgregory.net/rapid"
)

// 单元测试：两个用户并发消费，一个耗尽不影响另一个
// 确定性版本：使用 channel 等待回调完成，避免时序依赖
func TestQuotaBucket_IsolationTwoUsers(t *testing.T) {
	mgr := NewQuotaBucketManager()

	var exhaustedMu sync.Mutex
	exhaustedUsers := make(map[string]bool)
	callbackDone := make(chan string, 2) // 缓冲足够容纳可能的回调

	mgr.SetOnExhausted(func(userID string) {
		exhaustedMu.Lock()
		exhaustedUsers[userID] = true
		exhaustedMu.Unlock()
		callbackDone <- userID
	})

	// 用户 A: 1000 bytes, 用户 B: 5000 bytes
	mgr.UpdateQuota("userA", 1000)
	mgr.UpdateQuota("userB", 5000)

	var wg sync.WaitGroup
	var userAConsumed, userBConsumed uint64

	// 用户 A 并发消费 200 bytes × 10 = 2000（超过 1000，应耗尽）
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if mgr.Consume("userA", 200) {
				atomic.AddUint64(&userAConsumed, 200)
			}
		}()
	}

	// 用户 B 并发消费 200 bytes × 10 = 2000（不超过 5000，应全部成功）
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if mgr.Consume("userB", 200) {
				atomic.AddUint64(&userBConsumed, 200)
			}
		}()
	}

	wg.Wait()

	// 用户 A 最多消费 1000
	if userAConsumed > 1000 {
		t.Fatalf("userA 消费超限: %d > 1000", userAConsumed)
	}
	// 用户 B 应全部成功消费 2000
	if userBConsumed != 2000 {
		t.Fatalf("userB 应消费 2000，实际: %d", userBConsumed)
	}

	// 用户 A 应被标记耗尽
	if !mgr.IsExhausted("userA") {
		t.Fatal("userA 应被标记为耗尽")
	}
	// 用户 B 不应耗尽
	if mgr.IsExhausted("userB") {
		t.Fatal("userB 不应被标记为耗尽")
	}

	// 确定性等待回调完成（替代 sync.Map 的即时检查）
	select {
	case uid := <-callbackDone:
		if uid != "userA" {
			t.Fatalf("耗尽回调应为 userA，实际: %s", uid)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("userA 耗尽回调未在 2 秒内触发")
	}

	// userB 不应触发回调（短暂等待确认无额外回调）
	select {
	case uid := <-callbackDone:
		if uid == "userB" {
			t.Fatal("不应触发 userB 耗尽回调")
		}
		// 如果是 userA 的重复回调（不应发生），也记录
		t.Logf("收到额外回调: %s（可忽略）", uid)
	case <-time.After(100 * time.Millisecond):
		// 预期：无额外回调，正常
	}
}

// 单元测试：UpdateQuota 重置耗尽状态
func TestQuotaBucket_UpdateResetsExhausted(t *testing.T) {
	mgr := NewQuotaBucketManager()
	mgr.UpdateQuota("user1", 100)

	// 消费完
	mgr.Consume("user1", 100)
	if !mgr.IsExhausted("user1") {
		// 消费 100 后尝试再消费触发耗尽
		mgr.Consume("user1", 1)
	}

	// 重新下发配额
	mgr.UpdateQuota("user1", 500)
	if mgr.IsExhausted("user1") {
		t.Fatal("UpdateQuota 后应重置耗尽状态")
	}

	// 应能继续消费
	if !mgr.Consume("user1", 200) {
		t.Fatal("重置后应能消费")
	}
}

// 单元测试：未知用户消费被拒绝
func TestQuotaBucket_UnknownUserRejected(t *testing.T) {
	mgr := NewQuotaBucketManager()
	if mgr.Consume("unknown", 1) {
		t.Fatal("未知用户应被拒绝")
	}
}

// 单元测试：全局桶兼容
func TestQuotaBucket_GlobalBucketCompat(t *testing.T) {
	mgr := NewQuotaBucketManager()
	mgr.UpdateQuota(GlobalBucketKey, 1000)

	if !mgr.Consume(GlobalBucketKey, 500) {
		t.Fatal("全局桶消费应成功")
	}

	remaining, ok := mgr.GetRemaining(GlobalBucketKey)
	if !ok || remaining != 500 {
		t.Fatalf("全局桶剩余应为 500，实际: %d", remaining)
	}
}

// 单元测试：GetSummaries 返回所有用户
func TestQuotaBucket_GetSummaries(t *testing.T) {
	mgr := NewQuotaBucketManager()
	mgr.UpdateQuota("u1", 1000)
	mgr.UpdateQuota("u2", 2000)
	mgr.Consume("u1", 300)

	summaries := mgr.GetSummaries()
	if len(summaries) != 2 {
		t.Fatalf("应返回 2 个摘要，实际: %d", len(summaries))
	}

	found := map[string]uint64{}
	for _, s := range summaries {
		found[s.UserId] = s.RemainingBytes
	}
	if found["u1"] != 700 {
		t.Fatalf("u1 剩余应为 700，实际: %d", found["u1"])
	}
	if found["u2"] != 2000 {
		t.Fatalf("u2 剩余应为 2000，实际: %d", found["u2"])
	}
}

// =============================================================================
// Feature: phase3-operational-baseline, Property 1: QuotaBucket 用户隔离
// 验证: 需求 5.3, 6.1, 6.3
// =============================================================================

func TestProperty_QuotaBucketIsolation(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		quotaA := rapid.Uint64Range(1, 100000).Draw(t, "quotaA")
		quotaB := rapid.Uint64Range(1, 100000).Draw(t, "quotaB")
		consumeCount := rapid.IntRange(1, 20).Draw(t, "consumeCount")

		mgr := NewQuotaBucketManager()
		mgr.UpdateQuota("userA", quotaA)
		mgr.UpdateQuota("userB", quotaB)

		// 记录 userB 初始状态
		remainingBBefore, _ := mgr.GetRemaining("userB")

		// 耗尽 userA 配额
		for i := 0; i < consumeCount; i++ {
			amount := rapid.Uint64Range(1, quotaA).Draw(t, fmt.Sprintf("consumeA_%d", i))
			mgr.Consume("userA", amount)
		}
		// 确保 userA 耗尽
		mgr.Consume("userA", quotaA+1)

		// 验证 userB 的 RemainingBytes 不减少
		remainingBAfter, ok := mgr.GetRemaining("userB")
		if !ok {
			t.Fatal("userB 应存在")
		}
		if remainingBAfter != remainingBBefore {
			t.Fatalf("userB RemainingBytes 不应变化: before=%d, after=%d", remainingBBefore, remainingBAfter)
		}

		// 验证 userB 的 Exhausted 标志保持为 0
		if mgr.IsExhausted("userB") {
			t.Fatal("userB 不应被标记为耗尽")
		}

		// 验证 userB 的 Consume() 在自身配额范围内继续成功
		smallAmount := uint64(1)
		if quotaB > 1 {
			smallAmount = rapid.Uint64Range(1, quotaB).Draw(t, "consumeB")
		}
		if !mgr.Consume("userB", smallAmount) {
			t.Fatalf("userB 应能消费 %d bytes（配额 %d）", smallAmount, quotaB)
		}
	})
}
