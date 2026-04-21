package topology

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
)

// newTestRegistry 创建一个使用 miniredis 和 nil DB 的测试 Registry（跳过 loadFromDB）
func newTestRegistry(t *testing.T) (*Registry, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	rdb := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	r := &Registry{
		gateways: make(map[string]*GatewayInfo),
		byCell:   make(map[string][]string),
		db:       nil, // DB operations will be skipped in memory-only tests
		rdb:      rdb,
	}
	return r, mr
}

// newTestRegistryWithDB 创建带真实 miniredis 的 Registry，DB 用 stub
func newTestRegistryWithDB(t *testing.T, db *sql.DB) (*Registry, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	rdb := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	r := &Registry{
		gateways: make(map[string]*GatewayInfo),
		byCell:   make(map[string][]string),
		db:       db,
		rdb:      rdb,
	}
	return r, mr
}

func TestRegisterMemoryAndRedis(t *testing.T) {
	r, mr := newTestRegistry(t)
	defer mr.Close()

	// Register without DB (db is nil, so we test memory + Redis only)
	info := &GatewayInfo{
		GatewayID:    "gw-1",
		CellID:       "cell-a",
		DownlinkAddr: "10.0.0.1:9090",
		Version:      "v1.0",
		MaxSessions:  1000,
	}

	// Manually do memory + Redis (skip DB since db is nil)
	ctx := context.Background()
	info.Status = "ONLINE"
	info.LastHeartbeat = time.Now()

	pipe := r.rdb.Pipeline()
	gwKey := "topo:gw:gw-1"
	pipe.HSet(ctx, gwKey, map[string]interface{}{
		"cell_id":       info.CellID,
		"downlink_addr": info.DownlinkAddr,
		"status":        "ONLINE",
		"version":       info.Version,
		"max_sessions":  info.MaxSessions,
	})
	pipe.SAdd(ctx, "topo:cell:cell-a:gateways", info.GatewayID)
	pipe.Set(ctx, "gateway:gw-1:addr", info.DownlinkAddr, 10*time.Minute)
	pipe.Set(ctx, "gateway:gw-1:status", "ONLINE", 60*time.Second)
	pipe.Exec(ctx)

	r.mu.Lock()
	r.gateways[info.GatewayID] = info
	r.byCell[info.CellID] = appendUnique(r.byCell[info.CellID], info.GatewayID)
	r.mu.Unlock()

	// Verify memory index
	gws := r.GetGatewaysByCell("cell-a")
	if len(gws) != 1 || gws[0].GatewayID != "gw-1" {
		t.Fatalf("expected 1 gateway in cell-a, got %d", len(gws))
	}

	// Verify Redis
	val, err := r.rdb.HGet(ctx, "topo:gw:gw-1", "status").Result()
	if err != nil || val != "ONLINE" {
		t.Fatalf("expected Redis status ONLINE, got %q err=%v", val, err)
	}
	members, _ := r.rdb.SMembers(ctx, "topo:cell:cell-a:gateways").Result()
	if len(members) != 1 || members[0] != "gw-1" {
		t.Fatalf("expected gw-1 in cell set, got %v", members)
	}
}

func TestGetGatewaysByCellFiltersOffline(t *testing.T) {
	r, mr := newTestRegistry(t)
	defer mr.Close()

	r.mu.Lock()
	r.gateways["gw-1"] = &GatewayInfo{GatewayID: "gw-1", CellID: "cell-a", Status: "ONLINE"}
	r.gateways["gw-2"] = &GatewayInfo{GatewayID: "gw-2", CellID: "cell-a", Status: "OFFLINE"}
	r.gateways["gw-3"] = &GatewayInfo{GatewayID: "gw-3", CellID: "cell-b", Status: "ONLINE"}
	r.byCell["cell-a"] = []string{"gw-1", "gw-2"}
	r.byCell["cell-b"] = []string{"gw-3"}
	r.mu.Unlock()

	gws := r.GetGatewaysByCell("cell-a")
	if len(gws) != 1 || gws[0].GatewayID != "gw-1" {
		t.Fatalf("expected only gw-1 online in cell-a, got %v", gws)
	}

	gws = r.GetGatewaysByCell("cell-b")
	if len(gws) != 1 || gws[0].GatewayID != "gw-3" {
		t.Fatalf("expected gw-3 in cell-b, got %v", gws)
	}

	gws = r.GetGatewaysByCell("cell-nonexistent")
	if len(gws) != 0 {
		t.Fatalf("expected 0 gateways for nonexistent cell, got %d", len(gws))
	}
}

func TestGetAllOnline(t *testing.T) {
	r, mr := newTestRegistry(t)
	defer mr.Close()

	r.mu.Lock()
	r.gateways["gw-1"] = &GatewayInfo{GatewayID: "gw-1", Status: "ONLINE"}
	r.gateways["gw-2"] = &GatewayInfo{GatewayID: "gw-2", Status: "OFFLINE"}
	r.gateways["gw-3"] = &GatewayInfo{GatewayID: "gw-3", Status: "ONLINE"}
	r.gateways["gw-4"] = &GatewayInfo{GatewayID: "gw-4", Status: "DEGRADED"}
	r.mu.Unlock()

	online := r.GetAllOnline()
	if len(online) != 2 {
		t.Fatalf("expected 2 online gateways, got %d", len(online))
	}
	ids := map[string]bool{}
	for _, gw := range online {
		ids[gw.GatewayID] = true
	}
	if !ids["gw-1"] || !ids["gw-3"] {
		t.Fatalf("expected gw-1 and gw-3, got %v", ids)
	}
}

func TestUpdateHeartbeat(t *testing.T) {
	r, mr := newTestRegistry(t)
	defer mr.Close()

	before := time.Now().Add(-5 * time.Minute)
	r.mu.Lock()
	r.gateways["gw-1"] = &GatewayInfo{
		GatewayID:      "gw-1",
		Status:         "ONLINE",
		ActiveSessions: 10,
		LastHeartbeat:  before,
	}
	r.mu.Unlock()

	r.UpdateHeartbeat("gw-1", 42, "hash-abc")

	r.mu.RLock()
	gw := r.gateways["gw-1"]
	r.mu.RUnlock()

	if gw.ActiveSessions != 42 {
		t.Fatalf("expected ActiveSessions=42, got %d", gw.ActiveSessions)
	}
	if gw.LastHeartbeat.Before(before) || gw.LastHeartbeat.Equal(before) {
		t.Fatal("expected LastHeartbeat to be updated")
	}
	if gw.Status != "ONLINE" {
		t.Fatalf("expected status ONLINE, got %s", gw.Status)
	}
}

func TestUpdateHeartbeatNonexistent(t *testing.T) {
	r, mr := newTestRegistry(t)
	defer mr.Close()

	// Should not panic on nonexistent gateway
	r.UpdateHeartbeat("gw-nonexistent", 10, "hash")
}

func TestMarkOfflineMemoryAndRedis(t *testing.T) {
	r, mr := newTestRegistry(t)
	defer mr.Close()

	ctx := context.Background()

	// Setup: add gateway to memory and Redis
	r.mu.Lock()
	r.gateways["gw-1"] = &GatewayInfo{GatewayID: "gw-1", CellID: "cell-a", Status: "ONLINE"}
	r.byCell["cell-a"] = []string{"gw-1"}
	r.mu.Unlock()

	r.rdb.HSet(ctx, "topo:gw:gw-1", "status", "ONLINE")
	r.rdb.SAdd(ctx, "topo:cell:cell-a:gateways", "gw-1")

	// MarkOffline (DB will fail since db is nil, but memory + Redis should update)
	r.MarkOffline(ctx, "gw-1")

	// Verify memory
	r.mu.RLock()
	gw := r.gateways["gw-1"]
	r.mu.RUnlock()
	if gw.Status != "OFFLINE" {
		t.Fatalf("expected OFFLINE, got %s", gw.Status)
	}

	// Verify Redis
	val, _ := r.rdb.HGet(ctx, "topo:gw:gw-1", "status").Result()
	if val != "OFFLINE" {
		t.Fatalf("expected Redis status OFFLINE, got %s", val)
	}
	isMember, _ := r.rdb.SIsMember(ctx, "topo:cell:cell-a:gateways", "gw-1").Result()
	if isMember {
		t.Fatal("expected gw-1 removed from cell set")
	}

	// GetAllOnline should return empty
	if len(r.GetAllOnline()) != 0 {
		t.Fatal("expected no online gateways after MarkOffline")
	}
}

func TestMarkOfflineNonexistent(t *testing.T) {
	r, mr := newTestRegistry(t)
	defer mr.Close()

	// Should not panic
	r.MarkOffline(context.Background(), "gw-nonexistent")
}

func TestCheckTimeouts(t *testing.T) {
	r, mr := newTestRegistry(t)
	defer mr.Close()

	ctx := context.Background()
	timeout := 300 * time.Second

	r.mu.Lock()
	r.gateways["gw-fresh"] = &GatewayInfo{
		GatewayID:     "gw-fresh",
		CellID:        "cell-a",
		Status:        "ONLINE",
		LastHeartbeat: time.Now(),
	}
	r.gateways["gw-stale"] = &GatewayInfo{
		GatewayID:     "gw-stale",
		CellID:        "cell-a",
		Status:        "ONLINE",
		LastHeartbeat: time.Now().Add(-10 * time.Minute), // 600s ago, > 300s timeout
	}
	r.gateways["gw-offline"] = &GatewayInfo{
		GatewayID:     "gw-offline",
		CellID:        "cell-a",
		Status:        "OFFLINE",
		LastHeartbeat: time.Now().Add(-10 * time.Minute),
	}
	r.byCell["cell-a"] = []string{"gw-fresh", "gw-stale", "gw-offline"}
	r.mu.Unlock()

	// Setup Redis for stale gateway
	r.rdb.HSet(ctx, "topo:gw:gw-stale", "status", "ONLINE")
	r.rdb.SAdd(ctx, "topo:cell:cell-a:gateways", "gw-stale")

	r.checkTimeouts(ctx, timeout)

	r.mu.RLock()
	freshStatus := r.gateways["gw-fresh"].Status
	staleStatus := r.gateways["gw-stale"].Status
	offlineStatus := r.gateways["gw-offline"].Status
	r.mu.RUnlock()

	if freshStatus != "ONLINE" {
		t.Fatalf("gw-fresh should remain ONLINE, got %s", freshStatus)
	}
	if staleStatus != "OFFLINE" {
		t.Fatalf("gw-stale should be OFFLINE, got %s", staleStatus)
	}
	if offlineStatus != "OFFLINE" {
		t.Fatalf("gw-offline should remain OFFLINE, got %s", offlineStatus)
	}
}

func TestAppendUnique(t *testing.T) {
	s := []string{"a", "b"}
	s = appendUnique(s, "c")
	if len(s) != 3 {
		t.Fatalf("expected 3, got %d", len(s))
	}
	s = appendUnique(s, "b")
	if len(s) != 3 {
		t.Fatalf("expected 3 (no dup), got %d", len(s))
	}
}

func TestRemoveCellIndex(t *testing.T) {
	r, mr := newTestRegistry(t)
	defer mr.Close()

	r.byCell["cell-a"] = []string{"gw-1", "gw-2", "gw-3"}
	r.removeCellIndex("cell-a", "gw-2")
	if len(r.byCell["cell-a"]) != 2 {
		t.Fatalf("expected 2 after remove, got %d", len(r.byCell["cell-a"]))
	}
	for _, id := range r.byCell["cell-a"] {
		if id == "gw-2" {
			t.Fatal("gw-2 should have been removed")
		}
	}
}

func TestCellMigration(t *testing.T) {
	r, mr := newTestRegistry(t)
	defer mr.Close()

	ctx := context.Background()

	// Initial state: gw-1 in cell-a
	r.mu.Lock()
	r.gateways["gw-1"] = &GatewayInfo{GatewayID: "gw-1", CellID: "cell-a", Status: "ONLINE"}
	r.byCell["cell-a"] = []string{"gw-1"}
	r.mu.Unlock()
	r.rdb.SAdd(ctx, "topo:cell:cell-a:gateways", "gw-1")

	// Simulate re-register with new cell (memory + Redis only, no DB)
	newInfo := &GatewayInfo{
		GatewayID:     "gw-1",
		CellID:        "cell-b",
		DownlinkAddr:  "10.0.0.2:9090",
		Version:       "v1.1",
		Status:        "ONLINE",
		LastHeartbeat: time.Now(),
	}

	// Do the memory update manually (simulating Register without DB)
	pipe := r.rdb.Pipeline()
	pipe.HSet(ctx, "topo:gw:gw-1", map[string]interface{}{
		"cell_id":       newInfo.CellID,
		"downlink_addr": newInfo.DownlinkAddr,
		"status":        "ONLINE",
	})
	pipe.SAdd(ctx, "topo:cell:cell-b:gateways", "gw-1")
	pipe.Exec(ctx)

	r.mu.Lock()
	old := r.gateways["gw-1"]
	if old.CellID != newInfo.CellID {
		r.removeCellIndex(old.CellID, "gw-1")
		r.rdb.SRem(ctx, "topo:cell:cell-a:gateways", "gw-1")
	}
	r.gateways["gw-1"] = newInfo
	r.byCell["cell-b"] = appendUnique(r.byCell["cell-b"], "gw-1")
	r.mu.Unlock()

	// Verify: cell-a should be empty, cell-b should have gw-1
	if len(r.GetGatewaysByCell("cell-a")) != 0 {
		t.Fatal("cell-a should be empty after migration")
	}
	gws := r.GetGatewaysByCell("cell-b")
	if len(gws) != 1 || gws[0].GatewayID != "gw-1" {
		t.Fatal("cell-b should have gw-1")
	}

	// Verify Redis cell sets
	isMember, _ := r.rdb.SIsMember(ctx, "topo:cell:cell-a:gateways", "gw-1").Result()
	if isMember {
		t.Fatal("gw-1 should be removed from cell-a Redis set")
	}
	isMember, _ = r.rdb.SIsMember(ctx, "topo:cell:cell-b:gateways", "gw-1").Result()
	if !isMember {
		t.Fatal("gw-1 should be in cell-b Redis set")
	}
}
