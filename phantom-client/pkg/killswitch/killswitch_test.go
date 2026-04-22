package killswitch

import (
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"pgregory.net/rapid"
)

// mockPlatform records route operations for testing.
type mockPlatform struct {
	mu           sync.Mutex
	defaultGW    string
	defaultIface string
	hostRoutes   map[string]bool // ip -> exists
	defaultDel   bool
	tunDefault   bool
	ops          []string
}

func newMockPlatform() *mockPlatform {
	return &mockPlatform{
		defaultGW:    "192.168.1.1",
		defaultIface: "eth0",
		hostRoutes:   make(map[string]bool),
	}
}

func (m *mockPlatform) GetDefaultGateway() (string, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ops = append(m.ops, "GetDefaultGateway")
	return m.defaultGW, m.defaultIface, nil
}

func (m *mockPlatform) DeleteDefaultRoute() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ops = append(m.ops, "DeleteDefaultRoute")
	m.defaultDel = true
	return nil
}

func (m *mockPlatform) AddDefaultRoute(tunName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ops = append(m.ops, fmt.Sprintf("AddDefaultRoute(%s)", tunName))
	m.tunDefault = true
	return nil
}

func (m *mockPlatform) AddHostRoute(ip, gateway, iface string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ops = append(m.ops, fmt.Sprintf("AddHostRoute(%s)", ip))
	m.hostRoutes[ip] = true
	return nil
}

func (m *mockPlatform) DeleteHostRoute(ip string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ops = append(m.ops, fmt.Sprintf("DeleteHostRoute(%s)", ip))
	delete(m.hostRoutes, ip)
	return nil
}

func (m *mockPlatform) RestoreDefaultRoute(gateway, iface string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ops = append(m.ops, "RestoreDefaultRoute")
	m.defaultDel = false
	m.tunDefault = false
	return nil
}

func (m *mockPlatform) hasHostRoute(ip string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.hostRoutes[ip]
}

// Property 8: 路由原子性 — during UpdateGatewayRoute, at least one /32 route always exists
func TestProperty_RouteAtomicity(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		mock := newMockPlatform()
		ks := NewKillSwitchWithPlatform("mirage0", mock)

		initialIP := rapid.StringMatching(`\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}`).Draw(t, "initialIP")
		if err := ks.Activate(initialIP); err != nil {
			t.Fatal(err)
		}

		// Verify initial host route exists
		if !mock.hasHostRoute(initialIP) {
			t.Fatal("initial host route missing after activate")
		}

		// Perform multiple gateway switches
		nSwitches := rapid.IntRange(1, 10).Draw(t, "nSwitches")
		currentIP := initialIP
		for i := 0; i < nSwitches; i++ {
			newIP := rapid.StringMatching(`\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}`).Draw(t, fmt.Sprintf("newIP_%d", i))

			// Before update: current route exists
			if !mock.hasHostRoute(currentIP) {
				t.Fatalf("host route for %s missing before update %d", currentIP, i)
			}

			err := ks.UpdateGatewayRoute(newIP)
			if err != nil {
				t.Fatal(err)
			}

			// After update: new route exists
			if !mock.hasHostRoute(newIP) {
				t.Fatalf("host route for %s missing after update %d", newIP, i)
			}

			currentIP = newIP
		}
	})
}

// 激活/解除序列测试
func TestActivateDeactivateSequence(t *testing.T) {
	mock := newMockPlatform()
	ks := NewKillSwitchWithPlatform("mirage0", mock)

	if ks.IsActivated() {
		t.Fatal("should not be activated initially")
	}

	// Activate
	if err := ks.Activate("10.0.0.1"); err != nil {
		t.Fatal(err)
	}
	if !ks.IsActivated() {
		t.Fatal("should be activated")
	}

	// Verify operation sequence
	expected := []string{
		"GetDefaultGateway",
		"DeleteDefaultRoute",
		"AddDefaultRoute(mirage0)",
		"AddHostRoute(10.0.0.1)",
	}
	if len(mock.ops) != len(expected) {
		t.Fatalf("expected %d ops, got %d: %v", len(expected), len(mock.ops), mock.ops)
	}
	for i, op := range expected {
		if mock.ops[i] != op {
			t.Fatalf("op %d: expected %q, got %q", i, op, mock.ops[i])
		}
	}

	// Double activate should fail
	if err := ks.Activate("10.0.0.2"); err == nil {
		t.Fatal("double activate should fail")
	}

	// Deactivate
	if err := ks.Deactivate(); err != nil {
		t.Fatal(err)
	}
	if ks.IsActivated() {
		t.Fatal("should not be activated after deactivate")
	}
}

// Gateway 切换测试
func TestUpdateGatewayRoute(t *testing.T) {
	mock := newMockPlatform()
	ks := NewKillSwitchWithPlatform("mirage0", mock)

	if err := ks.Activate("10.0.0.1"); err != nil {
		t.Fatal(err)
	}

	if err := ks.UpdateGatewayRoute("10.0.0.2"); err != nil {
		t.Fatal(err)
	}

	// Old route should be gone, new route should exist
	if mock.hasHostRoute("10.0.0.1") {
		t.Fatal("old host route should be deleted")
	}
	if !mock.hasHostRoute("10.0.0.2") {
		t.Fatal("new host route should exist")
	}
}

// 未激活时更新应失败
func TestUpdateWithoutActivate(t *testing.T) {
	mock := newMockPlatform()
	ks := NewKillSwitchWithPlatform("mirage0", mock)

	if err := ks.UpdateGatewayRoute("10.0.0.1"); err == nil {
		t.Fatal("update without activate should fail")
	}
}

// 事务切换测试：PreAdd → Commit
func TestTransactionalSwitch(t *testing.T) {
	mock := newMockPlatform()
	ks := NewKillSwitchWithPlatform("mirage0", mock)

	if err := ks.Activate("10.0.0.1"); err != nil {
		t.Fatal(err)
	}

	// PreAdd new route
	if err := ks.PreAddHostRoute("10.0.0.2"); err != nil {
		t.Fatal(err)
	}
	if !mock.hasHostRoute("10.0.0.1") {
		t.Fatal("old route should still exist after PreAdd")
	}
	if !mock.hasHostRoute("10.0.0.2") {
		t.Fatal("new route should exist after PreAdd")
	}

	// Commit: delete old
	if err := ks.CommitSwitch("10.0.0.1", "10.0.0.2"); err != nil {
		t.Fatal(err)
	}
	if mock.hasHostRoute("10.0.0.1") {
		t.Fatal("old route should be deleted after Commit")
	}
	if !mock.hasHostRoute("10.0.0.2") {
		t.Fatal("new route should still exist after Commit")
	}
}

// 事务回滚测试：PreAdd → Rollback
func TestTransactionalRollback(t *testing.T) {
	mock := newMockPlatform()
	ks := NewKillSwitchWithPlatform("mirage0", mock)

	if err := ks.Activate("10.0.0.1"); err != nil {
		t.Fatal(err)
	}

	// PreAdd new route
	if err := ks.PreAddHostRoute("10.0.0.2"); err != nil {
		t.Fatal(err)
	}

	// Rollback: remove pre-added route
	if err := ks.RollbackPreAdd("10.0.0.2"); err != nil {
		t.Fatal(err)
	}
	if !mock.hasHostRoute("10.0.0.1") {
		t.Fatal("old route should still exist after Rollback")
	}
	if mock.hasHostRoute("10.0.0.2") {
		t.Fatal("pre-added route should be removed after Rollback")
	}
}

// 未激活时 PreAdd 应失败
func TestPreAddWithoutActivate(t *testing.T) {
	mock := newMockPlatform()
	ks := NewKillSwitchWithPlatform("mirage0", mock)

	if err := ks.PreAddHostRoute("10.0.0.1"); err == nil {
		t.Fatal("PreAdd without activate should fail")
	}
}

// Property: 事务切换原子性 — 任意时刻至少有一条 /32 路由存在
func TestProperty_TransactionalAtomicity(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		mock := newMockPlatform()
		ks := NewKillSwitchWithPlatform("mirage0", mock)

		initialIP := rapid.StringMatching(`\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}`).Draw(t, "initialIP")
		if err := ks.Activate(initialIP); err != nil {
			t.Fatal(err)
		}

		currentIP := initialIP
		nSwitches := rapid.IntRange(1, 10).Draw(t, "nSwitches")
		for i := 0; i < nSwitches; i++ {
			newIP := rapid.StringMatching(`\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}`).Draw(t, fmt.Sprintf("newIP_%d", i))

			// PreAdd: both routes exist
			if err := ks.PreAddHostRoute(newIP); err != nil {
				t.Fatal(err)
			}
			if !mock.hasHostRoute(currentIP) {
				t.Fatalf("old route %s missing after PreAdd", currentIP)
			}
			if !mock.hasHostRoute(newIP) {
				t.Fatalf("new route %s missing after PreAdd", newIP)
			}

			// Commit: only new route
			if err := ks.CommitSwitch(currentIP, newIP); err != nil {
				t.Fatal(err)
			}
			if !mock.hasHostRoute(newIP) {
				t.Fatalf("new route %s missing after Commit", newIP)
			}

			currentIP = newIP
		}
	})
}

// Feature: v1-client-productization, Property 4: RouteState 序列化往返
// **Validates: Requirements 14.4**
func TestProperty_RouteStateRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		original := &RouteState{
			OriginalGW:    rapid.StringMatching(`\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}`).Draw(t, "originalGW"),
			OriginalIface: rapid.StringMatching(`[a-z]{2,6}[0-9]`).Draw(t, "originalIface"),
			CurrentGWIP:   rapid.StringMatching(`\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}`).Draw(t, "currentGWIP"),
			TUNName:       rapid.StringMatching(`[a-z]{3,8}[0-9]`).Draw(t, "tunName"),
			ActivatedAt:   time.Unix(rapid.Int64Range(0, 2000000000).Draw(t, "activatedAt"), 0).UTC(),
		}

		// Save to temp file
		dir, err := os.MkdirTemp("", "killswitch-test-*")
		if err != nil {
			t.Fatalf("MkdirTemp: %v", err)
		}
		defer os.RemoveAll(dir)
		path := dir + "/route-state.json"

		if err := SaveRouteState(path, original); err != nil {
			t.Fatalf("SaveRouteState: %v", err)
		}

		// Load back
		loaded, err := LoadRouteState(path)
		if err != nil {
			t.Fatalf("LoadRouteState: %v", err)
		}

		// Verify round-trip equivalence
		if original.OriginalGW != loaded.OriginalGW {
			t.Fatalf("OriginalGW mismatch: %q vs %q", original.OriginalGW, loaded.OriginalGW)
		}
		if original.OriginalIface != loaded.OriginalIface {
			t.Fatalf("OriginalIface mismatch: %q vs %q", original.OriginalIface, loaded.OriginalIface)
		}
		if original.CurrentGWIP != loaded.CurrentGWIP {
			t.Fatalf("CurrentGWIP mismatch: %q vs %q", original.CurrentGWIP, loaded.CurrentGWIP)
		}
		if original.TUNName != loaded.TUNName {
			t.Fatalf("TUNName mismatch: %q vs %q", original.TUNName, loaded.TUNName)
		}
		if !original.ActivatedAt.Equal(loaded.ActivatedAt) {
			t.Fatalf("ActivatedAt mismatch: %v vs %v", original.ActivatedAt, loaded.ActivatedAt)
		}
	})
}

// failableMockPlatform is a mock Platform that can inject failure at a specific step.
type failableMockPlatform struct {
	mu           sync.Mutex
	defaultGW    string
	defaultIface string
	hostRoutes   map[string]bool
	defaultDel   bool
	tunDefault   bool

	failAtStep int // 0=never fail, 1=GetDefaultGateway, 2=DeleteDefaultRoute, 3=AddDefaultRoute, 4=AddHostRoute
	stepCount  int
}

func newFailableMockPlatform(failAt int) *failableMockPlatform {
	return &failableMockPlatform{
		defaultGW:    "192.168.1.1",
		defaultIface: "eth0",
		hostRoutes:   make(map[string]bool),
		failAtStep:   failAt,
	}
}

func (m *failableMockPlatform) mayFail(step int) error {
	m.stepCount++
	if m.failAtStep == step {
		return fmt.Errorf("injected failure at step %d", step)
	}
	return nil
}

func (m *failableMockPlatform) GetDefaultGateway() (string, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.mayFail(1); err != nil {
		return "", "", err
	}
	return m.defaultGW, m.defaultIface, nil
}

func (m *failableMockPlatform) DeleteDefaultRoute() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.mayFail(2); err != nil {
		return err
	}
	m.defaultDel = true
	return nil
}

func (m *failableMockPlatform) AddDefaultRoute(tunName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.mayFail(3); err != nil {
		return err
	}
	m.tunDefault = true
	return nil
}

func (m *failableMockPlatform) AddHostRoute(ip, gateway, iface string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.mayFail(4); err != nil {
		return err
	}
	m.hostRoutes[ip] = true
	return nil
}

func (m *failableMockPlatform) DeleteHostRoute(ip string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.hostRoutes, ip)
	return nil
}

func (m *failableMockPlatform) RestoreDefaultRoute(gateway, iface string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.defaultDel = false
	m.tunDefault = false
	return nil
}

func (m *failableMockPlatform) snapshot() (bool, bool, map[string]bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	routes := make(map[string]bool)
	for k, v := range m.hostRoutes {
		routes[k] = v
	}
	return m.defaultDel, m.tunDefault, routes
}

// Feature: v1-client-productization, Property 17: KillSwitch 事务回滚
// **Validates: Requirements 14.1, 14.2**
func TestProperty_KillSwitchTransactionRollback(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Inject failure at step 2, 3, or 4 (step 1 is GetDefaultGateway which fails before any mutation)
		failStep := rapid.IntRange(2, 4).Draw(t, "failStep")

		mock := newFailableMockPlatform(failStep)
		ks := NewKillSwitchWithPlatform("mirage0", mock)

		// Capture pre-transaction state
		preDefaultDel, preTunDefault, preHostRoutes := mock.snapshot()

		// Attempt Activate — should fail at the injected step
		gatewayIP := rapid.StringMatching(`\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}`).Draw(t, "gatewayIP")
		err := ks.Activate(gatewayIP)
		if err == nil {
			t.Fatal("expected Activate to fail with injected failure")
		}

		// Verify post-transaction state equals pre-transaction state
		postDefaultDel, postTunDefault, postHostRoutes := mock.snapshot()

		if preDefaultDel != postDefaultDel {
			t.Fatalf("defaultDel changed: pre=%v post=%v (failStep=%d)", preDefaultDel, postDefaultDel, failStep)
		}
		if preTunDefault != postTunDefault {
			t.Fatalf("tunDefault changed: pre=%v post=%v (failStep=%d)", preTunDefault, postTunDefault, failStep)
		}
		if len(preHostRoutes) != len(postHostRoutes) {
			t.Fatalf("hostRoutes count changed: pre=%d post=%d (failStep=%d)", len(preHostRoutes), len(postHostRoutes), failStep)
		}
		for k := range preHostRoutes {
			if !postHostRoutes[k] {
				t.Fatalf("hostRoute %s lost after rollback (failStep=%d)", k, failStep)
			}
		}

		// KillSwitch should NOT be activated
		if ks.IsActivated() {
			t.Fatal("kill switch should not be activated after failed Activate")
		}
	})
}
