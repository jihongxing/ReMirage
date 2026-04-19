package killswitch

import (
	"fmt"
	"sync"
	"testing"

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
