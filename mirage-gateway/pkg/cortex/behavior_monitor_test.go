package cortex

import (
	"testing"
	"time"
)

func TestConnProfile_SendRecvRatio_Zero(t *testing.T) {
	cp := &ConnProfile{SendBytes: 0, RecvBytes: 100}
	if got := cp.SendRecvRatio(); got != 0 {
		t.Errorf("SendRecvRatio() = %f, want 0", got)
	}
}

func TestConnProfile_SendRecvRatio_Normal(t *testing.T) {
	cp := &ConnProfile{SendBytes: 1000, RecvBytes: 500}
	got := cp.SendRecvRatio()
	if got != 0.5 {
		t.Errorf("SendRecvRatio() = %f, want 0.5", got)
	}
}

func TestConnProfile_DataEntropy_Empty(t *testing.T) {
	cp := &ConnProfile{}
	if got := cp.DataEntropy(); got != 0 {
		t.Errorf("DataEntropy() = %f, want 0", got)
	}
}

func TestConnProfile_DataEntropy_Uniform(t *testing.T) {
	// 所有相同大小 → 熵为 0
	cp := &ConnProfile{PacketSizes: []int{100, 100, 100, 100}}
	if got := cp.DataEntropy(); got != 0 {
		t.Errorf("DataEntropy() = %f, want 0 for uniform sizes", got)
	}
}

func TestConnProfile_DataEntropy_Varied(t *testing.T) {
	cp := &ConnProfile{PacketSizes: []int{64, 128, 256, 512}}
	got := cp.DataEntropy()
	if got <= 0 {
		t.Errorf("DataEntropy() = %f, want > 0 for varied sizes", got)
	}
}

func TestBehaviorMonitor_RecordPacket_NewConn(t *testing.T) {
	bm := NewBehaviorMonitor(DefaultBaseline(), 0.7, nil)
	bm.RecordPacket("conn1", "10.0.0.1", 128, 0)

	bm.mu.RLock()
	p, ok := bm.profiles["conn1"]
	bm.mu.RUnlock()

	if !ok {
		t.Fatal("expected profile to exist")
	}
	if p.SourceIP != "10.0.0.1" {
		t.Errorf("SourceIP = %s, want 10.0.0.1", p.SourceIP)
	}
	if p.PacketCount != 1 {
		t.Errorf("PacketCount = %d, want 1", p.PacketCount)
	}
	if p.SendBytes != 128 {
		t.Errorf("SendBytes = %d, want 128", p.SendBytes)
	}
}

func TestBehaviorMonitor_RecordPacket_Direction(t *testing.T) {
	bm := NewBehaviorMonitor(DefaultBaseline(), 0.7, nil)
	bm.RecordPacket("conn1", "10.0.0.1", 100, 0) // send
	bm.RecordPacket("conn1", "10.0.0.1", 200, 1) // recv

	bm.mu.RLock()
	p := bm.profiles["conn1"]
	bm.mu.RUnlock()

	if p.SendBytes != 100 {
		t.Errorf("SendBytes = %d, want 100", p.SendBytes)
	}
	if p.RecvBytes != 200 {
		t.Errorf("RecvBytes = %d, want 200", p.RecvBytes)
	}
}

func TestBehaviorMonitor_RecordPacket_CapAt1000(t *testing.T) {
	bm := NewBehaviorMonitor(DefaultBaseline(), 0.7, nil)
	for i := 0; i < 1100; i++ {
		bm.RecordPacket("conn1", "10.0.0.1", i%1500, 0)
	}

	bm.mu.RLock()
	p := bm.profiles["conn1"]
	bm.mu.RUnlock()

	if len(p.PacketSizes) != maxPacketSizes {
		t.Errorf("PacketSizes len = %d, want %d", len(p.PacketSizes), maxPacketSizes)
	}
}

func TestBehaviorMonitor_RemoveConn(t *testing.T) {
	bm := NewBehaviorMonitor(DefaultBaseline(), 0.7, nil)
	bm.RecordPacket("conn1", "10.0.0.1", 128, 0)
	bm.RemoveConn("conn1")

	bm.mu.RLock()
	_, ok := bm.profiles["conn1"]
	bm.mu.RUnlock()

	if ok {
		t.Fatal("expected profile to be removed")
	}
}

func TestBehaviorMonitor_SetOnKick(t *testing.T) {
	bm := NewBehaviorMonitor(DefaultBaseline(), 0.7, nil)
	called := false
	bm.SetOnKick(func(connID string) {
		called = true
	})
	bm.mu.RLock()
	fn := bm.onKick
	bm.mu.RUnlock()
	if fn == nil {
		t.Fatal("expected onKick to be set")
	}
	fn("test")
	if !called {
		t.Fatal("expected onKick to be called")
	}
}

func TestBehaviorMonitor_Evaluate_KicksAnomaly(t *testing.T) {
	bm := NewBehaviorMonitor(DefaultBaseline(), 0.01, nil) // 极低阈值，几乎任何流量都会触发

	var kicked []string
	bm.SetOnKick(func(connID string) {
		kicked = append(kicked, connID)
	})

	// 注入异常流量：全部大包
	bm.mu.Lock()
	bm.profiles["conn1"] = &ConnProfile{
		ConnID:      "conn1",
		SourceIP:    "10.0.0.1",
		StartTime:   time.Now().Add(-10 * time.Second),
		PacketSizes: make([]int, 100),
		SendBytes:   10000,
		RecvBytes:   10000,
	}
	for i := range bm.profiles["conn1"].PacketSizes {
		bm.profiles["conn1"].PacketSizes[i] = 1500
	}
	bm.mu.Unlock()

	bm.evaluate()

	if len(kicked) == 0 {
		t.Fatal("expected anomalous connection to be kicked")
	}
}

func TestBehaviorMonitor_Evaluate_SendOnlyKick(t *testing.T) {
	bm := NewBehaviorMonitor(DefaultBaseline(), 0.99, nil) // 高阈值，不会因偏离度触发

	var kicked []string
	bm.SetOnKick(func(connID string) {
		kicked = append(kicked, connID)
	})

	// 注入只发不收模式
	bm.mu.Lock()
	bm.profiles["conn2"] = &ConnProfile{
		ConnID:      "conn2",
		SourceIP:    "10.0.0.2",
		StartTime:   time.Now().Add(-60 * time.Second), // 持续 60s > 30s
		PacketSizes: []int{64, 64, 64},
		SendBytes:   10000,
		RecvBytes:   100, // ratio = 100/10000 = 0.01 < 0.05
		PacketCount: 100,
	}
	bm.mu.Unlock()

	bm.evaluate()

	if len(kicked) == 0 {
		t.Fatal("expected send-only connection to be kicked")
	}
}
