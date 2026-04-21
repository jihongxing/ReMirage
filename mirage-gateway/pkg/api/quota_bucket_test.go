package api

import (
	"sync"
	"sync/atomic"
	"testing"
)

// 单元测试：两个用户并发消费，一个耗尽不影响另一个
func TestQuotaBucket_IsolationTwoUsers(t *testing.T) {
	mgr := NewQuotaBucketManager()

	var exhaustedUsers sync.Map
	mgr.SetOnExhausted(func(userID string) {
		exhaustedUsers.Store(userID, true)
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

	// 回调应只触发 userA
	if _, ok := exhaustedUsers.Load("userA"); !ok {
		t.Fatal("应触发 userA 耗尽回调")
	}
	// userB 不应触发
	if _, ok := exhaustedUsers.Load("userB"); ok {
		t.Fatal("不应触发 userB 耗尽回调")
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
