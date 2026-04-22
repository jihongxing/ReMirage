package dispatch

import (
	"testing"
	"time"
)

// ============================================================
// 9.5 下推失败重试测试（PushLog 记录验证）
// ============================================================

func TestPushLogRecordAndGetRecent(t *testing.T) {
	pl := NewPushLog(nil, 100) // nil DB，仅内存

	// 记录多条下推结果，包含失败重试
	pl.Record("gw-1", "strategy", "success")
	pl.Record("gw-2", "strategy", "failed_after_retries")
	pl.Record("gw-3", "blacklist", "success_after_retry_2")
	pl.Record("gw-2", "quota", "failed_after_retries")

	// 获取最近 10 条
	entries := pl.GetRecent(10)
	if len(entries) != 4 {
		t.Fatalf("expected 4 entries, got %d", len(entries))
	}

	// 验证失败记录存在
	failCount := 0
	for _, e := range entries {
		if e.Result == "failed_after_retries" {
			failCount++
		}
	}
	if failCount != 2 {
		t.Fatalf("expected 2 failed_after_retries entries, got %d", failCount)
	}

	// 验证记录内容
	if entries[1].GatewayID != "gw-2" || entries[1].CommandType != "strategy" {
		t.Fatalf("entry[1]: expected gw-2/strategy, got %s/%s", entries[1].GatewayID, entries[1].CommandType)
	}
}

func TestPushLogGetRecentLimit(t *testing.T) {
	pl := NewPushLog(nil, 100)

	for i := 0; i < 20; i++ {
		pl.Record("gw-1", "strategy", "success")
	}

	// 只取最近 5 条
	entries := pl.GetRecent(5)
	if len(entries) != 5 {
		t.Fatalf("expected 5 entries, got %d", len(entries))
	}
}

func TestPushLogRingBuffer(t *testing.T) {
	pl := NewPushLog(nil, 5) // maxSize=5

	for i := 0; i < 10; i++ {
		pl.Record("gw-1", "strategy", "success")
	}

	// 环形缓冲应只保留最近 5 条
	entries := pl.GetRecent(100)
	if len(entries) != 5 {
		t.Fatalf("expected 5 entries (ring buffer), got %d", len(entries))
	}
}

func TestPushLogTimestamp(t *testing.T) {
	pl := NewPushLog(nil, 100)

	before := time.Now()
	pl.Record("gw-1", "blacklist", "failed_after_retries")
	after := time.Now()

	entries := pl.GetRecent(1)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	ts := entries[0].Timestamp
	if ts.Before(before) || ts.After(after) {
		t.Fatalf("timestamp %v not in range [%v, %v]", ts, before, after)
	}
}

// ============================================================
// FanoutEngine resolveTargets 测试
// ============================================================

// mockRegistry 实现 FanoutEngine 所需的 Registry 接口（通过直接引用 topology.Registry）
// 由于 FanoutEngine 直接依赖 topology.Registry，这里测试 PushLog 部分即可
// FanoutEngine 的 resolveTargets 逻辑已在 topology 包的 Cell 隔离测试中覆盖
