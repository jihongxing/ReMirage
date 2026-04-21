package api

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
)

// =============================================================================
// 9.1 多用户共享 Gateway 配额隔离集成测试
// =============================================================================

func TestIntegration_MultiUserQuotaIsolation(t *testing.T) {
	// Setup: QuotaBucketManager + SessionManager + UserTrafficCounter
	quotaMgr := NewQuotaBucketManager()
	sessMgr := NewSessionManager()
	trafficCounter := NewUserTrafficCounter()

	var exhaustedUsers sync.Map
	quotaMgr.SetOnExhausted(func(userID string) {
		exhaustedUsers.Store(userID, true)
	})

	// 注册 2 个用户的会话
	sessMgr.Register("sess-A1", "userA", "clientA")
	sessMgr.Register("sess-B1", "userB", "clientB")

	// 用户 A: 1000 bytes 配额, 用户 B: 10000 bytes 配额
	quotaMgr.UpdateQuota("userA", 1000)
	quotaMgr.UpdateQuota("userB", 10000)

	var wg sync.WaitGroup
	var userAConsumed, userBConsumed uint64

	// 用户 A 并发消费 200 bytes × 10 = 2000（超过 1000，应部分失败）
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			uid := sessMgr.GetUserID("sess-A1")
			if uid == "" {
				t.Error("sess-A1 应能查到 userA")
				return
			}
			if quotaMgr.Consume(uid, 200) {
				atomic.AddUint64(&userAConsumed, 200)
				trafficCounter.Add(uid, "sess-A1", 200, 0)
			}
		}()
	}

	// 用户 B 并发消费 500 bytes × 10 = 5000（不超过 10000，应全部成功）
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			uid := sessMgr.GetUserID("sess-B1")
			if uid == "" {
				t.Error("sess-B1 应能查到 userB")
				return
			}
			if quotaMgr.Consume(uid, 500) {
				atomic.AddUint64(&userBConsumed, 500)
				trafficCounter.Add(uid, "sess-B1", 500, 0)
			}
		}()
	}

	wg.Wait()

	// 验证：用户 A 最多消费 1000
	if userAConsumed > 1000 {
		t.Fatalf("userA 消费超限: %d > 1000", userAConsumed)
	}

	// 验证：用户 B 应全部成功消费 5000
	if userBConsumed != 5000 {
		t.Fatalf("userB 应消费 5000，实际: %d", userBConsumed)
	}

	// 验证：用户 A 应被标记耗尽
	if !quotaMgr.IsExhausted("userA") {
		t.Fatal("userA 应被标记为耗尽")
	}

	// 验证：用户 B 不应耗尽
	if quotaMgr.IsExhausted("userB") {
		t.Fatal("userB 不应被标记为耗尽")
	}

	// 验证：耗尽回调只触发 userA
	if _, ok := exhaustedUsers.Load("userA"); !ok {
		t.Fatal("应触发 userA 耗尽回调")
	}
	if _, ok := exhaustedUsers.Load("userB"); ok {
		t.Fatal("不应触发 userB 耗尽回调")
	}

	// 验证：TrafficCounter 有正确的按用户统计
	stats := trafficCounter.Flush()
	statsMap := make(map[string]uint64)
	for _, s := range stats {
		statsMap[s.UserID] = s.BusinessBytes
	}
	if statsMap["userA"] != userAConsumed {
		t.Fatalf("TrafficCounter userA 应为 %d，实际: %d", userAConsumed, statsMap["userA"])
	}
	if statsMap["userB"] != 5000 {
		t.Fatalf("TrafficCounter userB 应为 5000，实际: %d", statsMap["userB"])
	}
}

// =============================================================================
// 9.2 计费精确归属集成测试
// =============================================================================

func TestIntegration_BillingAttribution(t *testing.T) {
	// Setup: SessionManager + UserTrafficCounter，模拟同一 Gateway 上 2 个用户
	sessMgr := NewSessionManager()
	trafficCounter := NewUserTrafficCounter()

	// 同一 Gateway 上注册 2 个用户的会话
	sessMgr.Register("sess-X1", "userX", "clientX")
	sessMgr.Register("sess-X2", "userX", "clientX2") // userX 有 2 个会话
	sessMgr.Register("sess-Y1", "userY", "clientY")

	// 模拟流量：userX 的 2 个会话各产生流量
	trafficCounter.Add("userX", "sess-X1", 1000, 200)
	trafficCounter.Add("userX", "sess-X2", 500, 100)
	// userY 产生流量
	trafficCounter.Add("userY", "sess-Y1", 3000, 600)

	// Flush 获取各用户流量快照
	stats := trafficCounter.Flush()

	if len(stats) != 2 {
		t.Fatalf("应有 2 个用户的流量统计，实际: %d", len(stats))
	}

	statsMap := make(map[string]*TrafficStats)
	for _, s := range stats {
		statsMap[s.UserID] = s
	}

	// 验证 userX 流量合并（同一 user_id 的多个会话流量累加）
	xStats := statsMap["userX"]
	if xStats == nil {
		t.Fatal("应有 userX 的流量统计")
	}
	if xStats.BusinessBytes != 1500 {
		t.Fatalf("userX business_bytes 应为 1500，实际: %d", xStats.BusinessBytes)
	}
	if xStats.DefenseBytes != 300 {
		t.Fatalf("userX defense_bytes 应为 300，实际: %d", xStats.DefenseBytes)
	}

	// 验证 userY 流量独立
	yStats := statsMap["userY"]
	if yStats == nil {
		t.Fatal("应有 userY 的流量统计")
	}
	if yStats.BusinessBytes != 3000 {
		t.Fatalf("userY business_bytes 应为 3000，实际: %d", yStats.BusinessBytes)
	}
	if yStats.DefenseBytes != 600 {
		t.Fatalf("userY defense_bytes 应为 600，实际: %d", yStats.DefenseBytes)
	}

	// 验证序列号唯一且单调递增
	seq1 := trafficCounter.NextSeqNum()
	seq2 := trafficCounter.NextSeqNum()
	if seq2 <= seq1 {
		t.Fatalf("序列号应单调递增: seq1=%d, seq2=%d", seq1, seq2)
	}

	// 验证 Flush 后计数器已重置
	stats2 := trafficCounter.Flush()
	if len(stats2) != 0 {
		t.Fatalf("Flush 后不应有非零流量，实际: %d 条", len(stats2))
	}
}

// =============================================================================
// 9.3 幂等上报集成测试
// =============================================================================

func TestIntegration_IdempotentReporting(t *testing.T) {
	trafficCounter := NewUserTrafficCounter()

	// 验证序列号从 0 开始，单调递增
	if trafficCounter.CurrentSeqNum() != 0 {
		t.Fatalf("初始序列号应为 0，实际: %d", trafficCounter.CurrentSeqNum())
	}

	seq1 := trafficCounter.NextSeqNum()
	seq2 := trafficCounter.NextSeqNum()
	seq3 := trafficCounter.NextSeqNum()

	if seq1 != 1 || seq2 != 2 || seq3 != 3 {
		t.Fatalf("序列号应为 1,2,3，实际: %d,%d,%d", seq1, seq2, seq3)
	}

	// 添加流量并 Flush
	trafficCounter.Add("user1", "sess1", 1000, 200)
	stats1 := trafficCounter.Flush()
	if len(stats1) != 1 || stats1[0].BusinessBytes != 1000 {
		t.Fatal("第一次 Flush 应返回 user1 的 1000 bytes")
	}

	// 再次 Flush 同一数据 → 应返回空（计数器已重置）
	stats2 := trafficCounter.Flush()
	if len(stats2) != 0 {
		t.Fatalf("重复 Flush 不应返回数据，实际: %d 条", len(stats2))
	}

	// 序列号在 Flush 后继续递增（不重置）
	seq4 := trafficCounter.NextSeqNum()
	if seq4 != 4 {
		t.Fatalf("Flush 后序列号应继续递增，期望 4，实际: %d", seq4)
	}

	// 测试 SaveSeqNum / LoadSeqNum 持久化
	tmpDir := t.TempDir()
	seqFile := filepath.Join(tmpDir, "seq.dat")

	// 保存当前序列号
	if err := trafficCounter.SaveSeqNum(seqFile); err != nil {
		t.Fatalf("SaveSeqNum 失败: %v", err)
	}

	// 创建新的 counter 并恢复
	newCounter := NewUserTrafficCounter()
	if err := newCounter.LoadSeqNum(seqFile); err != nil {
		t.Fatalf("LoadSeqNum 失败: %v", err)
	}

	// 恢复后序列号应从保存的值继续
	if newCounter.CurrentSeqNum() != 4 {
		t.Fatalf("恢复后序列号应为 4，实际: %d", newCounter.CurrentSeqNum())
	}

	nextSeq := newCounter.NextSeqNum()
	if nextSeq != 5 {
		t.Fatalf("恢复后下一个序列号应为 5，实际: %d", nextSeq)
	}

	// 测试文件不存在时 LoadSeqNum 不报错（从 0 开始）
	emptyCounter := NewUserTrafficCounter()
	if err := emptyCounter.LoadSeqNum(filepath.Join(tmpDir, "nonexistent.dat")); err != nil {
		t.Fatalf("不存在的文件应不报错: %v", err)
	}
	if emptyCounter.CurrentSeqNum() != 0 {
		t.Fatalf("文件不存在时序列号应为 0，实际: %d", emptyCounter.CurrentSeqNum())
	}

	// 测试损坏文件（< 8 bytes）
	corruptFile := filepath.Join(tmpDir, "corrupt.dat")
	if err := os.WriteFile(corruptFile, []byte{0x01, 0x02}, 0600); err != nil {
		t.Fatalf("写入损坏文件失败: %v", err)
	}
	corruptCounter := NewUserTrafficCounter()
	if err := corruptCounter.LoadSeqNum(corruptFile); err != nil {
		t.Fatalf("损坏文件应不报错: %v", err)
	}
	if corruptCounter.CurrentSeqNum() != 0 {
		t.Fatalf("损坏文件时序列号应为 0，实际: %d", corruptCounter.CurrentSeqNum())
	}
}

// =============================================================================
// 9.4 会话生命周期集成测试
// =============================================================================

func TestIntegration_SessionLifecycle(t *testing.T) {
	sessMgr := NewSessionManager()

	// 1. 注册会话 → 验证 GetUserID 正常工作
	sessMgr.Register("sess-1", "user-alpha", "client-1")
	if uid := sessMgr.GetUserID("sess-1"); uid != "user-alpha" {
		t.Fatalf("注册后 GetUserID 应返回 user-alpha，实际: %q", uid)
	}

	// 2. 验证 ActiveSessionCount
	if count := sessMgr.ActiveSessionCount(); count != 1 {
		t.Fatalf("注册 1 个会话后 ActiveSessionCount 应为 1，实际: %d", count)
	}

	// 3. 注册同一用户的第二个会话
	sessMgr.Register("sess-2", "user-alpha", "client-2")
	if count := sessMgr.ActiveSessionCount(); count != 2 {
		t.Fatalf("注册 2 个会话后 ActiveSessionCount 应为 2，实际: %d", count)
	}

	// 4. 验证 GetActiveSessionsByUser
	sessions := sessMgr.GetActiveSessionsByUser("user-alpha")
	if len(sessions) != 2 {
		t.Fatalf("user-alpha 应有 2 个活跃会话，实际: %d", len(sessions))
	}

	// 5. 注册另一个用户的会话
	sessMgr.Register("sess-3", "user-beta", "client-3")
	if count := sessMgr.ActiveSessionCount(); count != 3 {
		t.Fatalf("注册 3 个会话后 ActiveSessionCount 应为 3，实际: %d", count)
	}

	// 6. 注销 sess-1 → 验证 GetUserID 返回空
	info := sessMgr.Unregister("sess-1")
	if info == nil {
		t.Fatal("Unregister 应返回被注销的会话信息")
	}
	if info.UserID != "user-alpha" {
		t.Fatalf("注销的会话应属于 user-alpha，实际: %q", info.UserID)
	}
	if uid := sessMgr.GetUserID("sess-1"); uid != "" {
		t.Fatalf("注销后 GetUserID 应返回空，实际: %q", uid)
	}

	// 7. 验证 ActiveSessionCount 减少
	if count := sessMgr.ActiveSessionCount(); count != 2 {
		t.Fatalf("注销 1 个后 ActiveSessionCount 应为 2，实际: %d", count)
	}

	// 8. user-alpha 现在只剩 1 个会话
	sessions = sessMgr.GetActiveSessionsByUser("user-alpha")
	if len(sessions) != 1 {
		t.Fatalf("user-alpha 注销 1 个后应剩 1 个会话，实际: %d", len(sessions))
	}
	if sessions[0] != "sess-2" {
		t.Fatalf("user-alpha 剩余会话应为 sess-2，实际: %q", sessions[0])
	}

	// 9. user-beta 不受影响
	if uid := sessMgr.GetUserID("sess-3"); uid != "user-beta" {
		t.Fatalf("user-beta 的会话不应受影响，实际: %q", uid)
	}

	// 10. 注销所有会话
	sessMgr.Unregister("sess-2")
	sessMgr.Unregister("sess-3")
	if count := sessMgr.ActiveSessionCount(); count != 0 {
		t.Fatalf("全部注销后 ActiveSessionCount 应为 0，实际: %d", count)
	}

	// 11. user-alpha 无活跃会话
	sessions = sessMgr.GetActiveSessionsByUser("user-alpha")
	if len(sessions) != 0 {
		t.Fatalf("全部注销后 user-alpha 不应有活跃会话，实际: %d", len(sessions))
	}

	// 12. 重复注销不 panic
	info = sessMgr.Unregister("sess-1")
	if info != nil {
		t.Fatal("重复注销应返回 nil")
	}

	// 13. GetSession 验证
	sessMgr.Register("sess-4", "user-gamma", "client-4")
	sessInfo := sessMgr.GetSession("sess-4")
	if sessInfo == nil {
		t.Fatal("GetSession 应返回会话信息")
	}
	if sessInfo.UserID != "user-gamma" || sessInfo.ClientID != "client-4" {
		t.Fatalf("GetSession 返回信息不正确: %+v", sessInfo)
	}
	if sessInfo.ConnectedAt.IsZero() {
		t.Fatal("ConnectedAt 不应为零值")
	}
}
