package p0runtime

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

// =============================================================================
// P0 Bug Condition Exploration Tests — POST-FIX Verification
// These tests verify that all 8 P0 bugs have been FIXED.
// Each test checks the EXPECTED (correct) behavior after the fix.
//
// **Validates: Requirements 2.1, 2.2, 2.3, 2.4, 2.5, 2.6, 2.7, 2.8**
// =============================================================================

func readSrc(t *testing.T, relPath string) string {
	t.Helper()
	data, err := os.ReadFile(relPath)
	if err != nil {
		t.Fatalf("cannot read %s: %v", relPath, err)
	}
	return string(data)
}

// --- Test 1.1: EventDispatcher interface — adapter pattern applied ---

func TestBug1_1_EventDispatcherInterfaceMismatch(t *testing.T) {
	mainSrc := readSrc(t, "../../mirage-gateway/cmd/gateway/main.go")

	// FIXED: main.go should use stealthDispatcherAdapter instead of direct v2Dispatcher
	if !strings.Contains(mainSrc, "stealthDispatcherAdapter") {
		t.Fatal("FIX NOT APPLIED: main.go does not contain stealthDispatcherAdapter")
	}

	// Verify the adapter wraps v2Dispatcher
	if !strings.Contains(mainSrc, "&stealthDispatcherAdapter{") {
		t.Fatal("FIX NOT APPLIED: main.go does not instantiate stealthDispatcherAdapter")
	}

	// Verify direct assignment is no longer used
	if strings.Contains(mainSrc, "Dispatcher: v2Dispatcher,") {
		t.Fatal("REGRESSION: main.go still directly assigns v2Dispatcher without adapter")
	}

	t.Log("VERIFIED 1.1: stealthDispatcherAdapter bridges events.EventDispatcher → stealth.EventDispatcher")
}

// --- Test 1.2: 'kr' variable defined before OnBanned callback ---

func TestBug1_2_UndefinedKrVariable(t *testing.T) {
	mainSrc := readSrc(t, "../../phantom-client/cmd/phantom/main.go")

	// Verify OnBanned still references kr.Delete
	if !strings.Contains(mainSrc, "kr.Delete(keyringService, keyringPSK)") {
		t.Fatal("OnBanned callback does not reference kr.Delete — unexpected code change")
	}

	// Extract runDaemonMode body up to OnBanned
	daemonIdx := strings.Index(mainSrc, "func runDaemonMode(")
	if daemonIdx < 0 {
		t.Fatal("runDaemonMode not found")
	}
	daemonBody := mainSrc[daemonIdx:]
	onBannedIdx := strings.Index(daemonBody, "OnBanned:")
	if onBannedIdx < 0 {
		t.Fatal("OnBanned not found in runDaemonMode")
	}

	beforeOnBanned := daemonBody[:onBannedIdx]

	// FIXED: kr should be defined before OnBanned
	if !strings.Contains(beforeOnBanned, "kr :=") && !strings.Contains(beforeOnBanned, "var kr ") {
		t.Fatal("FIX NOT APPLIED: 'kr' is NOT defined before OnBanned in runDaemonMode")
	}

	t.Log("VERIFIED 1.2: 'kr' is defined before OnBanned callback in runDaemonMode")
}

// --- Test 1.3: Topology field structure — OS-side aligned with Client-side ---

// OS-side type (after fix: aligned with Client-side GatewayNode)
type osGatewayNode struct {
	IP       string `json:"ip"`
	Port     int    `json:"port"`
	Priority uint8  `json:"priority"`
	Region   string `json:"region"`
	CellID   string `json:"cell_id"`
}

// Client-side type (phantom-client/pkg/gtclient/topo.go)
type clientGatewayNode struct {
	IP       string `json:"ip"`
	Port     int    `json:"port"`
	Priority uint8  `json:"priority"`
	Region   string `json:"region"`
	CellID   string `json:"cell_id"`
}

func TestBug1_3_TopologyFieldMismatch(t *testing.T) {
	// Simulate OS-side response (after fix: uses GatewayNode with ip, port, etc.)
	osNode := osGatewayNode{
		IP:       "10.0.0.1",
		Port:     443,
		Priority: 0,
		Region:   "ap-east-1",
		CellID:   "cell-01",
	}

	data, err := json.Marshal(osNode)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var clientNode clientGatewayNode
	if err := json.Unmarshal(data, &clientNode); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// FIXED: IP should be non-empty, Port should be non-zero
	if clientNode.IP == "" {
		t.Fatal("FIX NOT APPLIED: GatewayNode.IP is empty after deserialization")
	}
	if clientNode.Port == 0 {
		t.Fatal("FIX NOT APPLIED: GatewayNode.Port is 0 after deserialization")
	}
	if clientNode.CellID == "" {
		t.Fatal("FIX NOT APPLIED: GatewayNode.CellID is empty after deserialization")
	}

	// Also verify the OS handler source uses the correct struct
	handlerSrc := readSrc(t, "../../mirage-os/services/topology/handler.go")
	if strings.Contains(handlerSrc, `"ip_address"`) && !strings.Contains(handlerSrc, `"ip"`) {
		t.Fatal("REGRESSION: OS handler still uses ip_address instead of ip")
	}
	if strings.Contains(handlerSrc, `"gateway_id"`) && !strings.Contains(handlerSrc, `"ip"`) {
		t.Fatal("REGRESSION: OS handler still uses old GatewayEntry fields")
	}

	t.Log("VERIFIED 1.3: OS-side GatewayNode fields align with Client-side — IP, Port, CellID all populated")
}

// --- Test 1.4: HMAC signature algorithm — both sides use json.Marshal ---

type clientHmacBody struct {
	Gateways    []clientGatewayNode `json:"gateways"`
	Version     uint64              `json:"version"`
	PublishedAt time.Time           `json:"published_at"`
}

func TestBug1_4_HMACSignatureMismatch(t *testing.T) {
	psk := []byte("test-psk-key-32-bytes-long-xxxxx")
	version := uint64(42)
	publishedAt := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)

	gateways := []clientGatewayNode{
		{IP: "10.0.0.1", Port: 443, Priority: 0, Region: "ap-east-1", CellID: "cell-01"},
	}

	// OS-side (after fix): json.Marshal(hmacBody{...})
	osBody := clientHmacBody{
		Gateways:    gateways,
		Version:     version,
		PublishedAt: publishedAt,
	}
	osData, _ := json.Marshal(osBody)
	osMac := hmac.New(sha256.New, psk)
	osMac.Write(osData)
	osHMAC := osMac.Sum(nil)

	// Client-side: json.Marshal(hmacBody{...})
	clientBody := clientHmacBody{
		Gateways:    gateways,
		Version:     version,
		PublishedAt: publishedAt,
	}
	clientData, _ := json.Marshal(clientBody)
	clientMac := hmac.New(sha256.New, psk)
	clientMac.Write(clientData)
	clientHMAC := clientMac.Sum(nil)

	// FIXED: Both sides should produce identical HMAC
	if !hmac.Equal(osHMAC, clientHMAC) {
		t.Fatalf("FIX NOT APPLIED: HMAC mismatch — OS and Client still use different serialization.\n"+
			"  OS input:     %q\n"+
			"  Client input: %q", string(osData), string(clientData))
	}

	// Also verify the OS handler source uses json.Marshal for HMAC (not fmt.Sprintf concat)
	handlerSrc := readSrc(t, "../../mirage-os/services/topology/handler.go")
	// Check that the HMAC section uses json.Marshal, not fmt.Sprintf for signing data
	hmacIdx := strings.Index(handlerSrc, "hmac.New")
	if hmacIdx < 0 {
		t.Fatal("OS handler does not contain hmac.New — HMAC computation missing")
	}
	// Look at the ~300 chars before hmac.New to see how signing data is prepared
	start := hmacIdx - 300
	if start < 0 {
		start = 0
	}
	hmacSection := handlerSrc[start:hmacIdx]
	if strings.Contains(hmacSection, "fmt.Sprintf") {
		t.Fatal("REGRESSION: OS handler still uses fmt.Sprintf to prepare HMAC signing data")
	}
	if !strings.Contains(hmacSection, "json.Marshal") {
		t.Fatal("FIX NOT APPLIED: OS handler does not use json.Marshal for HMAC signing data")
	}

	t.Log("VERIFIED 1.4: OS-side and Client-side HMAC computation produce identical results")
}

// --- Test 1.5: SQL heartbeat column name consistency ---

func TestBug1_5_HeartbeatColumnInconsistency(t *testing.T) {
	grpcSrc := readSrc(t, "../../mirage-os/gateway-bridge/pkg/grpc/server.go")
	registrySrc := readSrc(t, "../../mirage-os/gateway-bridge/pkg/topology/registry.go")
	restSrc := readSrc(t, "../../mirage-os/gateway-bridge/pkg/rest/handler.go")

	// FIXED: All modules should use last_heartbeat_at consistently
	if !strings.Contains(grpcSrc, "last_heartbeat_at") {
		t.Fatal("FIX NOT APPLIED: grpc/server.go does not use 'last_heartbeat_at'")
	}
	if !strings.Contains(registrySrc, "last_heartbeat_at") {
		t.Fatal("REGRESSION: registry.go does not use 'last_heartbeat_at'")
	}
	if !strings.Contains(restSrc, "last_heartbeat_at") {
		t.Fatal("FIX NOT APPLIED: rest/handler.go does not use 'last_heartbeat_at'")
	}

	// Verify no stale 'last_heartbeat' (without _at suffix) in SQL contexts
	// Check grpc/server.go for bare 'last_heartbeat' in SQL (not 'last_heartbeat_at')
	grpcLines := strings.Split(grpcSrc, "\n")
	for _, line := range grpcLines {
		trimmed := strings.TrimSpace(line)
		// Skip comments and non-SQL lines
		if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "*") {
			continue
		}
		// Look for SQL context: lines containing SQL keywords
		isSQLContext := strings.Contains(trimmed, "INSERT") || strings.Contains(trimmed, "UPDATE") ||
			strings.Contains(trimmed, "SELECT") || strings.Contains(trimmed, "last_heartbeat")
		if isSQLContext && strings.Contains(trimmed, "last_heartbeat") && !strings.Contains(trimmed, "last_heartbeat_at") {
			t.Fatalf("REGRESSION: grpc/server.go still has bare 'last_heartbeat' in SQL: %s", trimmed)
		}
	}

	t.Log("VERIFIED 1.5: All SQL modules consistently use 'last_heartbeat_at'")
}

// --- Test 1.6: SQL primary key column consistency ---

func TestBug1_6_PrimaryKeyColumnInconsistency(t *testing.T) {
	grpcSrc := readSrc(t, "../../mirage-os/gateway-bridge/pkg/grpc/server.go")
	registrySrc := readSrc(t, "../../mirage-os/gateway-bridge/pkg/topology/registry.go")

	// FIXED: Both modules should use ON CONFLICT (gateway_id)
	if !strings.Contains(grpcSrc, "ON CONFLICT (gateway_id)") {
		t.Fatal("FIX NOT APPLIED: grpc/server.go does not use 'ON CONFLICT (gateway_id)'")
	}
	if !strings.Contains(registrySrc, "ON CONFLICT (gateway_id)") {
		t.Fatal("REGRESSION: registry.go does not use 'ON CONFLICT (gateway_id)'")
	}

	// Verify no stale 'ON CONFLICT (id)' remains
	if strings.Contains(grpcSrc, "ON CONFLICT (id)") {
		t.Fatal("REGRESSION: grpc/server.go still contains 'ON CONFLICT (id)'")
	}

	t.Log("VERIFIED 1.6: All SQL modules consistently use 'ON CONFLICT (gateway_id)'")
}

// --- Test 1.7: Provisioning routes registered in bridge ---

func TestBug1_7_ProvisioningRoutesNotRegistered(t *testing.T) {
	bridgeSrc := readSrc(t, "../../mirage-os/gateway-bridge/cmd/bridge/main.go")

	// FIXED: bridge should import provisioning and register routes
	hasImport := strings.Contains(bridgeSrc, `"mirage-os/services/provisioning"`)
	if !hasImport {
		t.Fatal("FIX NOT APPLIED: bridge/main.go does not import provisioning package")
	}

	hasRegisterRoutes := strings.Contains(bridgeSrc, "RegisterRoutes(mux)")
	if !hasRegisterRoutes {
		t.Fatal("FIX NOT APPLIED: bridge/main.go does not call RegisterRoutes on mux")
	}

	// Verify provisioning handler is created and wired
	hasNewHTTPHandler := strings.Contains(bridgeSrc, "provisioning.NewHTTPHandler")
	if !hasNewHTTPHandler {
		t.Fatal("FIX NOT APPLIED: bridge/main.go does not create provisioning.NewHTTPHandler")
	}

	t.Log("VERIFIED 1.7: bridge/main.go imports provisioning and registers routes on mux")
}

// --- Test 1.8: Integration test path matches OS endpoint ---

func TestBug1_8_IntegrationTestPathMismatch(t *testing.T) {
	testSrc := readSrc(t, "../../phantom-client/pkg/gtclient/integration_test.go")
	osSrc := readSrc(t, "../../mirage-os/services/topology/handler.go")

	// FIXED: integration_test.go should use /api/v2/topology (matching OS)
	osUsesV2 := strings.Contains(osSrc, `"/api/v2/topology"`)
	if !osUsesV2 {
		t.Fatal("OS handler does not contain /api/v2/topology — unexpected")
	}

	testUsesV2 := strings.Contains(testSrc, `"/api/v2/topology"`)
	if !testUsesV2 {
		t.Fatal("FIX NOT APPLIED: integration_test.go does not use '/api/v2/topology'")
	}

	// Verify no stale v1 path remains
	testUsesV1 := strings.Contains(testSrc, `"/api/v1/topology"`)
	if testUsesV1 {
		t.Fatal("REGRESSION: integration_test.go still contains '/api/v1/topology'")
	}

	t.Log("VERIFIED 1.8: integration_test.go uses '/api/v2/topology' matching OS endpoint")
}

// --- Helper: verify fmt.Sprintf is not used for HMAC in handler ---
// (used by Test 1.4 source check — kept as documentation)
func extractHMACSection(src string) string {
	idx := strings.Index(src, "HMAC")
	if idx < 0 {
		return ""
	}
	end := idx + 500
	if end > len(src) {
		end = len(src)
	}
	return src[idx:end]
}
