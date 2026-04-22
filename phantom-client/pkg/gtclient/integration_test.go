package gtclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"phantom-client/pkg/entitlement"
	"phantom-client/pkg/persist"
	"phantom-client/pkg/token"
)

// =============================================================================
// Integration Test: Full flow mock
// Provisioning → Daemon startup → Topo pull → Entitlement sync → Reconnect degradation
// =============================================================================

// mockOSControlPlane simulates the OS control plane HTTP server.
type mockOSControlPlane struct {
	mu              sync.Mutex
	topoVersion     uint64
	topoGateways    []GatewayNode
	topoSignature   []byte
	topoPublishedAt time.Time
	entitlement     *entitlement.Entitlement
	topoCallCount   atomic.Int32
	entCallCount    atomic.Int32
	failTopo        bool
	failEnt         bool
}

func newMockOSControlPlane() *mockOSControlPlane {
	return &mockOSControlPlane{
		topoVersion: 1,
		topoGateways: []GatewayNode{
			{IP: "10.0.0.1", Port: 443, Priority: 0, Region: "ap-east-1", CellID: "cell-01"},
			{IP: "10.0.0.2", Port: 443, Priority: 1, Region: "ap-east-1", CellID: "cell-02"},
		},
		topoPublishedAt: time.Now().UTC(),
		entitlement: &entitlement.Entitlement{
			ExpiresAt:      time.Now().Add(30 * 24 * time.Hour),
			QuotaRemaining: 100 * 1024 * 1024 * 1024, // 100GB
			ServiceClass:   entitlement.ClassPlatinum,
			Banned:         false,
			FetchedAt:      time.Now(),
		},
	}
}

func (m *mockOSControlPlane) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/api/v2/topology":
		m.topoCallCount.Add(1)
		m.mu.Lock()
		defer m.mu.Unlock()
		if m.failTopo {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		resp := RouteTableResponse{
			Gateways:    m.topoGateways,
			Version:     m.topoVersion,
			PublishedAt: m.topoPublishedAt,
			Signature:   m.topoSignature,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)

	case "/api/v2/entitlement":
		m.entCallCount.Add(1)
		m.mu.Lock()
		defer m.mu.Unlock()
		if m.failEnt {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(m.entitlement)

	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

// TestIntegration_ProvisionToReconnect tests the full flow:
// 1. Provisioning: persist config + store keyring
// 2. Daemon startup: load config, create GTunnelClient with dual pools
// 3. Topo pull: TopoRefresher fetches from mock OS, writes to RuntimeTopology
// 4. Entitlement sync: EntitlementManager fetches from mock OS
// 5. Reconnect degradation: L1 → L2 → L3 fallback
func TestIntegration_ProvisionToReconnect(t *testing.T) {
	// --- Phase 1: Provisioning (simulate) ---
	psk := make([]byte, 32)
	for i := range psk {
		psk[i] = byte(i)
	}
	authKey := make([]byte, 16)
	for i := range authKey {
		authKey[i] = byte(i + 100)
	}

	bootstrapPool := []token.GatewayEndpoint{
		{IP: "192.168.1.1", Port: 443, Region: "bootstrap-region"},
		{IP: "192.168.1.2", Port: 443, Region: "bootstrap-region"},
	}

	// Simulate PersistConfig save/load
	tmpDir := t.TempDir()
	persistCfg := &persist.PersistConfig{
		BootstrapPool:   bootstrapPool,
		CertFingerprint: "abc123",
		UserID:          "user-001",
		OSEndpoint:      "", // will be set after mock server starts
	}
	configPath := tmpDir + "/config.json"
	if err := persist.Save(configPath, persistCfg); err != nil {
		t.Fatalf("persist save: %v", err)
	}
	loaded, err := persist.Load(configPath)
	if err != nil {
		t.Fatalf("persist load: %v", err)
	}
	if len(loaded.BootstrapPool) != 2 {
		t.Fatalf("expected 2 bootstrap nodes, got %d", len(loaded.BootstrapPool))
	}

	// Simulate keyring store/load
	kr := persist.NewKeyring()
	if err := kr.Store("phantom-client", "psk", psk); err != nil {
		t.Fatalf("keyring store psk: %v", err)
	}
	if err := kr.Store("phantom-client", "auth_key", authKey); err != nil {
		t.Fatalf("keyring store auth_key: %v", err)
	}
	loadedPSK, err := kr.Load("phantom-client", "psk")
	if err != nil {
		t.Fatalf("keyring load psk: %v", err)
	}
	if len(loadedPSK) != 32 {
		t.Fatalf("expected 32-byte PSK, got %d", len(loadedPSK))
	}

	t.Log("Phase 1 (Provisioning): PASS — config persisted, keyring stored")

	// --- Phase 2: Daemon startup (simulate) ---
	config := &token.BootstrapConfig{
		BootstrapPool:   loaded.BootstrapPool,
		PreSharedKey:    loadedPSK,
		AuthKey:         authKey,
		CertFingerprint: loaded.CertFingerprint,
		UserID:          loaded.UserID,
	}

	client := NewGTunnelClient(config)
	defer client.Close()

	// Verify dual pool separation
	bp := client.BootstrapPool()
	if len(bp) != 2 {
		t.Fatalf("expected 2 bootstrap pool nodes, got %d", len(bp))
	}
	if !client.RuntimeTopo().IsEmpty() {
		t.Fatal("expected empty runtime topo at startup")
	}

	t.Log("Phase 2 (Daemon startup): PASS — GTunnelClient created with dual pools")

	// --- Phase 3: Topo pull ---
	mockOS := newMockOSControlPlane()
	// Compute valid HMAC signature for the mock response
	mockResp := &RouteTableResponse{
		Gateways:    mockOS.topoGateways,
		Version:     mockOS.topoVersion,
		PublishedAt: mockOS.topoPublishedAt,
	}
	sig, err := ComputeHMAC(mockResp, psk[:32])
	if err != nil {
		t.Fatalf("compute hmac: %v", err)
	}
	mockOS.mu.Lock()
	mockOS.topoSignature = sig
	mockOS.mu.Unlock()

	server := httptest.NewServer(mockOS)
	defer server.Close()

	// Create TopoRefresher with mock fetcher
	verifier := NewTopoVerifier(psk)
	topoRefresher := NewTopoRefresher(TopoRefresherConfig{
		Fetcher: func(ctx context.Context) (*RouteTableResponse, error) {
			req, _ := http.NewRequestWithContext(ctx, "GET", server.URL+"/api/v2/topology", nil)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return nil, err
			}
			defer resp.Body.Close()
			if resp.StatusCode != 200 {
				return nil, fmt.Errorf("status %d", resp.StatusCode)
			}
			var result RouteTableResponse
			json.NewDecoder(resp.Body).Decode(&result)
			return &result, nil
		},
		Verifier: verifier,
		Topo:     client.RuntimeTopo(),
		Interval: 1 * time.Second,
	})

	// Wire TopoRefresher into client (Task 11.1)
	client.SetTopoRefresher(topoRefresher)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Single pull
	if err := topoRefresher.PullOnce(ctx); err != nil {
		t.Fatalf("topo pull: %v", err)
	}

	if client.RuntimeTopo().Count() != 2 {
		t.Fatalf("expected 2 runtime topo nodes, got %d", client.RuntimeTopo().Count())
	}
	if client.RuntimeTopo().Version() != 1 {
		t.Fatalf("expected topo version 1, got %d", client.RuntimeTopo().Version())
	}

	// Verify bootstrap pool unchanged after topo update
	bp2 := client.BootstrapPool()
	if len(bp2) != 2 || bp2[0].IP != "192.168.1.1" {
		t.Fatal("bootstrap pool was modified after topo update")
	}

	t.Log("Phase 3 (Topo pull): PASS — RuntimeTopology updated, BootstrapPool unchanged")

	// --- Phase 4: Entitlement sync ---
	var entChangeCount atomic.Int32
	var bannedCalled atomic.Bool

	entMgr := entitlement.NewEntitlementManager(entitlement.EntitlementConfig{
		Fetcher: func(ctx context.Context) (*entitlement.Entitlement, error) {
			req, _ := http.NewRequestWithContext(ctx, "GET", server.URL+"/api/v2/entitlement", nil)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return nil, err
			}
			defer resp.Body.Close()
			if resp.StatusCode != 200 {
				return nil, fmt.Errorf("status %d", resp.StatusCode)
			}
			var ent entitlement.Entitlement
			json.NewDecoder(resp.Body).Decode(&ent)
			ent.FetchedAt = time.Now()
			return &ent, nil
		},
		Interval: 1 * time.Second,
		OnChange: func(old, new_ *entitlement.Entitlement) {
			entChangeCount.Add(1)
		},
		OnBanned: func() {
			bannedCalled.Store(true)
		},
	})

	// Single fetch
	entMgr.FetchOnce(ctx)
	ent := entMgr.Current()
	if ent == nil {
		t.Fatal("expected entitlement after fetch")
	}
	if ent.ServiceClass != entitlement.ClassPlatinum {
		t.Fatalf("expected Platinum, got %s", ent.ServiceClass)
	}
	if ent.Banned {
		t.Fatal("expected not banned")
	}
	if entChangeCount.Load() != 1 {
		t.Fatalf("expected 1 change event, got %d", entChangeCount.Load())
	}

	t.Log("Phase 4 (Entitlement sync): PASS — entitlement fetched, change event fired")

	// --- Phase 5: Reconnect degradation ---
	// Test degradation level tracking
	if client.DegradationLevel() != L1_Normal {
		t.Fatalf("expected L1_Normal, got %s", client.DegradationLevel())
	}

	// Test setDegradation
	var degradEvents []DegradationEvent
	var degradMu sync.Mutex
	client.SetOnDegradation(func(event DegradationEvent) {
		degradMu.Lock()
		degradEvents = append(degradEvents, event)
		degradMu.Unlock()
	})

	client.setDegradation(L2_Degraded, "runtime topo exhausted", 3)
	if client.DegradationLevel() != L2_Degraded {
		t.Fatalf("expected L2_Degraded, got %s", client.DegradationLevel())
	}

	time.Sleep(5 * time.Millisecond) // ensure measurable duration for recovery event

	client.setDegradation(L3_LastResort, "bootstrap pool exhausted", 5)
	if client.DegradationLevel() != L3_LastResort {
		t.Fatalf("expected L3_LastResort, got %s", client.DegradationLevel())
	}

	time.Sleep(5 * time.Millisecond) // ensure measurable duration for recovery event

	// Recovery
	client.setDegradation(L1_Normal, "reconnected via runtime topo", 1)
	if client.DegradationLevel() != L1_Normal {
		t.Fatalf("expected L1_Normal after recovery, got %s", client.DegradationLevel())
	}

	degradMu.Lock()
	if len(degradEvents) != 3 {
		t.Fatalf("expected 3 degradation events, got %d", len(degradEvents))
	}
	// Last event should be recovery with Duration > 0
	lastEvent := degradEvents[2]
	if lastEvent.Level != L1_Normal {
		t.Fatalf("expected recovery to L1_Normal, got %s", lastEvent.Level)
	}
	if lastEvent.Duration <= 0 {
		t.Fatal("expected positive recovery duration")
	}
	degradMu.Unlock()

	t.Log("Phase 5 (Reconnect degradation): PASS — degradation tracking works correctly")

	// --- Phase 6: Banned scenario (Task 11.2) ---
	mockOS.mu.Lock()
	mockOS.entitlement.Banned = true
	mockOS.mu.Unlock()

	entMgr.FetchOnce(ctx)
	if !bannedCalled.Load() {
		t.Fatal("expected banned callback to be called")
	}

	t.Log("Phase 6 (Banned scenario): PASS — banned callback fired")

	// --- Phase 7: Grace window + read-only mode (Task 11.3) ---
	graceWindow := entitlement.NewGraceWindow(100 * time.Millisecond) // short for testing
	graceWindow.RecordSuccess(ent)

	var readOnlyCalled atomic.Bool
	var recoveredCalled atomic.Bool

	graceMgr := entitlement.NewEntitlementManager(entitlement.EntitlementConfig{
		Fetcher: func(ctx context.Context) (*entitlement.Entitlement, error) {
			return nil, fmt.Errorf("control plane unreachable")
		},
		Interval: 50 * time.Millisecond,
		Grace:    graceWindow,
		OnReadOnly: func() {
			readOnlyCalled.Store(true)
		},
		OnRecovered: func() {
			recoveredCalled.Store(true)
		},
	})

	// Wait for grace to expire
	time.Sleep(150 * time.Millisecond)

	// Fetch should fail and trigger read-only
	graceMgr.FetchOnce(ctx)
	if !graceMgr.IsReadOnly() {
		t.Fatal("expected read-only mode after grace expiry")
	}
	if !readOnlyCalled.Load() {
		t.Fatal("expected onReadOnly callback")
	}

	t.Log("Phase 7 (Grace window): PASS — read-only mode entered after grace expiry")

	// --- Phase 8: ControlledDisconnect (Task 11.2) ---
	client2 := NewGTunnelClient(config)
	defer client2.Close()
	client2.ControlledDisconnect("test banned")
	if client2.State() != StateStopped {
		t.Fatalf("expected StateStopped after ControlledDisconnect, got %s", client2.State())
	}

	t.Log("Phase 8 (ControlledDisconnect): PASS — client stopped gracefully")

	// --- Phase 9: Verify topo call count ---
	if mockOS.topoCallCount.Load() < 1 {
		t.Fatal("expected at least 1 topo call")
	}
	if mockOS.entCallCount.Load() < 1 {
		t.Fatal("expected at least 1 entitlement call")
	}

	t.Log("Phase 9 (Call counts): PASS — mock OS received expected calls")
	t.Log("Integration test: ALL PHASES PASSED")
}
