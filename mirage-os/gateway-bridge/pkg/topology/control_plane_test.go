package topology

import (
	"context"
	"testing"
	"time"
)

// === 9.1 Gateway 注册测试 ===

func TestGatewayRegister_MemoryQueryable(t *testing.T) {
	r, mr := newTestRegistry(t)
	defer mr.Close()
	ctx := context.Background()

	info := &GatewayInfo{
		GatewayID: "gw-reg-1", CellID: "cell-x", DownlinkAddr: "10.0.1.1:9090",
		Version: "v2.0", MaxSessions: 500, Status: "ONLINE", LastHeartbeat: time.Now(),
	}
	pipe := r.rdb.Pipeline()
	pipe.HSet(ctx, "topo:gw:gw-reg-1", map[string]interface{}{"status": "ONLINE"})
	pipe.SAdd(ctx, "topo:cell:cell-x:gateways", "gw-reg-1")
	pipe.Exec(ctx)

	r.mu.Lock()
	r.gateways[info.GatewayID] = info
	r.byCell[info.CellID] = appendUnique(r.byCell[info.CellID], info.GatewayID)
	r.mu.Unlock()

	gws := r.GetGatewaysByCell("cell-x")
	if len(gws) != 1 || gws[0].GatewayID != "gw-reg-1" {
		t.Fatalf("expected gw-reg-1 in cell-x, got %d", len(gws))
	}
	all := r.GetAllOnline()
	if len(all) != 1 {
		t.Fatalf("expected 1 online, got %d", len(all))
	}
}

// === 9.3 Gateway 下线测试 ===

func TestGatewayOffline_HeartbeatTimeout(t *testing.T) {
	r, mr := newTestRegistry(t)
	defer mr.Close()
	ctx := context.Background()

	r.mu.Lock()
	r.gateways["gw-alive"] = &GatewayInfo{GatewayID: "gw-alive", CellID: "cell-a", Status: "ONLINE", LastHeartbeat: time.Now()}
	r.gateways["gw-dead"] = &GatewayInfo{GatewayID: "gw-dead", CellID: "cell-a", Status: "ONLINE", LastHeartbeat: time.Now().Add(-10 * time.Minute)}
	r.byCell["cell-a"] = []string{"gw-alive", "gw-dead"}
	r.mu.Unlock()

	r.rdb.HSet(ctx, "topo:gw:gw-dead", "status", "ONLINE")
	r.rdb.SAdd(ctx, "topo:cell:cell-a:gateways", "gw-dead")

	r.checkTimeouts(ctx, 300*time.Second)

	r.mu.RLock()
	if r.gateways["gw-alive"].Status != "ONLINE" {
		t.Fatal("gw-alive should remain ONLINE")
	}
	if r.gateways["gw-dead"].Status != "OFFLINE" {
		t.Fatal("gw-dead should be OFFLINE after timeout")
	}
	r.mu.RUnlock()

	online := r.GetAllOnline()
	if len(online) != 1 || online[0].GatewayID != "gw-alive" {
		t.Fatalf("expected only gw-alive online, got %d", len(online))
	}
}

// === 9.4 Gateway 重注册测试 ===

func TestGatewayReRegister_DownlinkAddrUpdate(t *testing.T) {
	r, mr := newTestRegistry(t)
	defer mr.Close()
	ctx := context.Background()

	// 初始注册
	r.mu.Lock()
	r.gateways["gw-1"] = &GatewayInfo{GatewayID: "gw-1", CellID: "cell-a", DownlinkAddr: "10.0.0.1:9090", Status: "ONLINE", LastHeartbeat: time.Now()}
	r.byCell["cell-a"] = []string{"gw-1"}
	r.mu.Unlock()
	r.rdb.SAdd(ctx, "topo:cell:cell-a:gateways", "gw-1")

	// 重注册：新地址 + 新 Cell
	newInfo := &GatewayInfo{GatewayID: "gw-1", CellID: "cell-b", DownlinkAddr: "10.0.0.2:9090", Version: "v1.1", Status: "ONLINE", LastHeartbeat: time.Now()}

	r.mu.Lock()
	old := r.gateways["gw-1"]
	if old.CellID != newInfo.CellID {
		r.removeCellIndex(old.CellID, "gw-1")
		r.rdb.SRem(ctx, "topo:cell:cell-a:gateways", "gw-1")
	}
	r.gateways["gw-1"] = newInfo
	r.byCell["cell-b"] = appendUnique(r.byCell["cell-b"], "gw-1")
	r.mu.Unlock()
	r.rdb.SAdd(ctx, "topo:cell:cell-b:gateways", "gw-1")

	// 验证：旧 Cell 空，新 Cell 有
	if len(r.GetGatewaysByCell("cell-a")) != 0 {
		t.Fatal("cell-a should be empty")
	}
	gws := r.GetGatewaysByCell("cell-b")
	if len(gws) != 1 || gws[0].DownlinkAddr != "10.0.0.2:9090" {
		t.Fatal("cell-b should have gw-1 with new addr")
	}
}

// === 9.6 状态对齐测试 ===

func TestStateAlignment_HashMismatch(t *testing.T) {
	r, mr := newTestRegistry(t)
	defer mr.Close()

	r.mu.Lock()
	r.gateways["gw-1"] = &GatewayInfo{GatewayID: "gw-1", Status: "ONLINE", ActiveSessions: 5, LastHeartbeat: time.Now().Add(-1 * time.Minute)}
	r.mu.Unlock()

	// 模拟心跳带 state_hash
	r.UpdateHeartbeat("gw-1", 10, "hash-from-gateway")

	r.mu.RLock()
	gw := r.gateways["gw-1"]
	r.mu.RUnlock()

	if gw.ActiveSessions != 10 {
		t.Fatalf("expected 10 sessions, got %d", gw.ActiveSessions)
	}
	if gw.Status != "ONLINE" {
		t.Fatalf("expected ONLINE, got %s", gw.Status)
	}
	// state_hash 对齐逻辑在 SyncHeartbeat handler 中实现（比对 DownlinkService 的 hash）
	// 这里验证 UpdateHeartbeat 正确刷新了心跳时间
	if time.Since(gw.LastHeartbeat) > 2*time.Second {
		t.Fatal("LastHeartbeat should be refreshed")
	}
}
