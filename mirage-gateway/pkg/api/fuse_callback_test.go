package api

import (
	"testing"
	"time"
)

// TestFuseCallback_ExhaustedTriggersDisconnect 验证配额耗尽时仅断开该用户连接
func TestFuseCallback_ExhaustedTriggersDisconnect(t *testing.T) {
	sessMgr := NewSessionManager()
	sessMgr.Register("sess-a1", "userA", "client1")
	sessMgr.Register("sess-a2", "userA", "client2")
	sessMgr.Register("sess-b1", "userB", "client3")

	// 不传 grpcClient（nil），仅测试断开逻辑
	fc := NewFuseCallback(nil, sessMgr, "gw-1")

	qbm := NewQuotaBucketManager()
	fc.Register(qbm)

	qbm.UpdateQuota("userA", 10)
	qbm.UpdateQuota("userB", 10000)

	// 耗尽 userA 配额
	qbm.Consume("userA", 10)
	// 触发 exhausted 回调（异步 goroutine）
	qbm.Consume("userA", 1)

	// 等待回调执行（最多 1 秒）
	for i := 0; i < 1000; i++ {
		if sessMgr.ActiveSessionCount() <= 1 {
			break
		}
		time.Sleep(time.Millisecond)
	}

	// userA 的会话应被断开
	if sessions := sessMgr.GetActiveSessionsByUser("userA"); len(sessions) != 0 {
		t.Errorf("userA 应无活跃会话，实际有 %d 个", len(sessions))
	}

	// userB 的会话不受影响
	if sessions := sessMgr.GetActiveSessionsByUser("userB"); len(sessions) != 1 {
		t.Errorf("userB 应有 1 个活跃会话，实际有 %d 个", len(sessions))
	}
}

// TestFuseCallback_OnlyAffectsTargetUser 验证熔断隔离：一个用户耗尽不影响另一个
func TestFuseCallback_OnlyAffectsTargetUser(t *testing.T) {
	sessMgr := NewSessionManager()
	sessMgr.Register("sess-1", "user1", "c1")
	sessMgr.Register("sess-2", "user2", "c2")

	fc := NewFuseCallback(nil, sessMgr, "gw-test")

	qbm := NewQuotaBucketManager()
	fc.Register(qbm)

	qbm.UpdateQuota("user1", 100)
	qbm.UpdateQuota("user2", 100)

	// user1 消费完配额
	qbm.Consume("user1", 100)
	qbm.Consume("user1", 1) // 触发熔断

	// 等待回调
	for i := 0; i < 1000; i++ {
		if sessMgr.GetActiveSessionsByUser("user1") == nil {
			break
		}
	}

	// user2 仍可正常消费
	if !qbm.Consume("user2", 50) {
		t.Fatal("user2 应能正常消费")
	}
	if qbm.IsExhausted("user2") {
		t.Fatal("user2 不应被标记为耗尽")
	}
}

// TestDisconnectUser 验证 SessionManager.DisconnectUser 仅断开目标用户
func TestDisconnectUser(t *testing.T) {
	sm := NewSessionManager()
	sm.Register("s1", "alice", "c1")
	sm.Register("s2", "alice", "c2")
	sm.Register("s3", "bob", "c3")

	count := sm.DisconnectUser("alice")
	if count != 2 {
		t.Errorf("应断开 2 个会话，实际 %d", count)
	}
	if sm.ActiveSessionCount() != 1 {
		t.Errorf("应剩 1 个活跃会话，实际 %d", sm.ActiveSessionCount())
	}
	if sessions := sm.GetActiveSessionsByUser("bob"); len(sessions) != 1 {
		t.Errorf("bob 应有 1 个会话，实际 %d", len(sessions))
	}
}
