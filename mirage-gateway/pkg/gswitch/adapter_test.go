package gswitch

import (
	"testing"
	"time"
)

func newTestManager() *GSwitchManager {
	return NewGSwitchManager(nil, nil)
}

func TestAdapter_GetPoolStats(t *testing.T) {
	mgr := newTestManager()
	mgr.AddDomain("test1.example.com", "1.1.1.1")
	mgr.AddDomain("test2.example.com", "2.2.2.2")

	adapter := NewGSwitchAdapter(mgr)
	stats := adapter.GetPoolStats()

	if stats["standby"] != 2 {
		t.Errorf("expected 2 standby, got %d", stats["standby"])
	}
}

func TestAdapter_IsStandbyPoolEmpty(t *testing.T) {
	mgr := newTestManager()
	adapter := NewGSwitchAdapter(mgr)

	if !adapter.IsStandbyPoolEmpty() {
		t.Error("expected standby pool to be empty")
	}

	mgr.AddDomain("test.example.com", "1.1.1.1")
	if adapter.IsStandbyPoolEmpty() {
		t.Error("expected standby pool to be non-empty")
	}
}

func TestAdapter_OnDomainBurned(t *testing.T) {
	mgr := newTestManager()
	adapter := NewGSwitchAdapter(mgr)

	var burnedDomain string
	adapter.OnDomainBurned(func(domain string, reason string) {
		burnedDomain = domain
	})

	// 添加域名并激活
	mgr.AddDomain("active.example.com", "1.1.1.1")
	mgr.mu.Lock()
	if len(mgr.standbyPool) > 0 {
		mgr.currentDomain = mgr.standbyPool[0]
		mgr.currentDomain.Status = DomainActive
		mgr.standbyPool = mgr.standbyPool[1:]
	}
	mgr.mu.Unlock()

	// 添加备用域名用于逃逸
	mgr.AddDomain("standby.example.com", "2.2.2.2")

	// 触发逃逸
	err := adapter.TriggerEscape("test burn")
	if err != nil {
		t.Fatalf("TriggerEscape failed: %v", err)
	}

	// 等待异步回调
	time.Sleep(100 * time.Millisecond)

	if burnedDomain != "active.example.com" {
		t.Errorf("expected burned domain 'active.example.com', got %q", burnedDomain)
	}
}

func TestAdapter_GetEntryBurnCount(t *testing.T) {
	mgr := newTestManager()
	adapter := NewGSwitchAdapter(mgr)

	// 初始应为 0
	count := adapter.GetEntryBurnCount(1 * time.Hour)
	if count != 0 {
		t.Errorf("expected 0 burn count, got %d", count)
	}
}
