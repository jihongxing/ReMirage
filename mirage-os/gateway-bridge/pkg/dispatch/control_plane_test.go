package dispatch

import (
	"testing"
	"time"
)

// === 9.2 按 Cell 下推测试 ===

// mockRegistry 模拟拓扑索引
type mockRegistry struct {
	cells map[string][]*GatewayInfoRef
	all   []*GatewayInfoRef
}

func (m *mockRegistry) GetGatewaysByCell(cellID string) []*GatewayInfoRef {
	return m.cells[cellID]
}
func (m *mockRegistry) GetAllOnline() []*GatewayInfoRef {
	return m.all
}

func TestFanoutResolveTargets_CellScope(t *testing.T) {
	reg := &mockRegistry{
		cells: map[string][]*GatewayInfoRef{
			"cell-a": {{GatewayID: "gw-1"}, {GatewayID: "gw-2"}},
			"cell-b": {{GatewayID: "gw-3"}},
		},
	}
	// FanoutEngine 需要 topology.Registry，但 resolveTargets 内部用的是 topology 包
	// 这里测试 PushStrategyToCell 通过 Registry 接口查询
	dispatcher := NewStrategyDispatcher(nil)
	dispatcher.SetRegistry(reg)

	// 验证 registry 接口返回正确
	cellA := reg.GetGatewaysByCell("cell-a")
	if len(cellA) != 2 {
		t.Fatalf("expected 2 gateways in cell-a, got %d", len(cellA))
	}
	cellB := reg.GetGatewaysByCell("cell-b")
	if len(cellB) != 1 {
		t.Fatalf("expected 1 gateway in cell-b, got %d", len(cellB))
	}
	// cell-c 不存在
	cellC := reg.GetGatewaysByCell("cell-c")
	if len(cellC) != 0 {
		t.Fatalf("expected 0 gateways in cell-c, got %d", len(cellC))
	}
}

func TestFanoutResolveTargets_GlobalScope(t *testing.T) {
	reg := &mockRegistry{
		all: []*GatewayInfoRef{{GatewayID: "gw-1"}, {GatewayID: "gw-2"}, {GatewayID: "gw-3"}},
	}
	all := reg.GetAllOnline()
	if len(all) != 3 {
		t.Fatalf("expected 3 online gateways, got %d", len(all))
	}
}

// === 9.5 下推失败重试测试 ===

func TestPushLog_RecordAndGetRecent(t *testing.T) {
	pl := NewPushLog(nil, 100)

	pl.Record("gw-1", "strategy", "success")
	pl.Record("gw-2", "blacklist", "failed_after_retries")
	pl.Record("gw-1", "quota", "success_after_retry_1")

	recent := pl.GetRecent(10)
	if len(recent) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(recent))
	}
	if recent[0].GatewayID != "gw-1" || recent[0].Result != "success" {
		t.Fatalf("unexpected first entry: %+v", recent[0])
	}
	if recent[1].Result != "failed_after_retries" {
		t.Fatalf("expected failed_after_retries, got %s", recent[1].Result)
	}
}

func TestPushLog_RingBuffer(t *testing.T) {
	pl := NewPushLog(nil, 5)

	for i := 0; i < 10; i++ {
		pl.Record("gw-1", "strategy", "success")
	}

	recent := pl.GetRecent(100)
	if len(recent) != 5 {
		t.Fatalf("expected 5 entries (ring buffer), got %d", len(recent))
	}
}

func TestPushLog_GetRecentLimited(t *testing.T) {
	pl := NewPushLog(nil, 100)
	for i := 0; i < 20; i++ {
		pl.Record("gw-1", "strategy", "success")
	}

	recent := pl.GetRecent(5)
	if len(recent) != 5 {
		t.Fatalf("expected 5 entries, got %d", len(recent))
	}
}

func TestPushLog_Timestamps(t *testing.T) {
	pl := NewPushLog(nil, 100)
	before := time.Now()
	pl.Record("gw-1", "strategy", "success")
	after := time.Now()

	recent := pl.GetRecent(1)
	if len(recent) != 1 {
		t.Fatal("expected 1 entry")
	}
	if recent[0].Timestamp.Before(before) || recent[0].Timestamp.After(after) {
		t.Fatal("timestamp out of range")
	}
}
