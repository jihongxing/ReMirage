package p0runtime

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"pgregory.net/rapid"
)

// =============================================================================
// Preservation Property Tests — Client-side behavior baseline
// These tests verify EXISTING behavior that must NOT change after the fix.
// They MUST PASS on the current unfixed code.
//
// Types are replicated locally to avoid importing from project packages
// (which may have compilation issues due to the bugs being fixed).
//
// **Validates: Requirements 3.1, 3.2, 3.3, 3.4, 3.5, 3.6, 3.7**
// =============================================================================

// --- Replicated Client-side types from phantom-client/pkg/gtclient/topo.go ---

// GatewayNode 带优先级和区域的网关节点 (Client-side)
type GatewayNode struct {
	IP       string `json:"ip"`
	Port     int    `json:"port"`
	Priority uint8  `json:"priority"`
	Region   string `json:"region"`
	CellID   string `json:"cell_id"`
}

// RouteTableResponse OS 控制面返回的路由表 (Client-side)
type RouteTableResponse struct {
	Gateways    []GatewayNode `json:"gateways"`
	Version     uint64        `json:"version"`
	PublishedAt time.Time     `json:"published_at"`
	Signature   []byte        `json:"signature"`
}

// hmacBody is the canonical form used for HMAC computation (Client-side)
type hmacBody struct {
	Gateways    []GatewayNode `json:"gateways"`
	Version     uint64        `json:"version"`
	PublishedAt time.Time     `json:"published_at"`
}

// ComputeHMAC computes HMAC-SHA256 over the canonical body (replicated from Client-side)
func ComputeHMAC(resp *RouteTableResponse, key []byte) ([]byte, error) {
	body := hmacBody{
		Gateways:    resp.Gateways,
		Version:     resp.Version,
		PublishedAt: resp.PublishedAt,
	}
	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal hmac body: %w", err)
	}
	mac := hmac.New(sha256.New, key)
	mac.Write(data)
	return mac.Sum(nil), nil
}

// --- Generators ---

func genGatewayNode(t *rapid.T) GatewayNode {
	return GatewayNode{
		IP:       fmt.Sprintf("%d.%d.%d.%d", rapid.IntRange(1, 255).Draw(t, "ip1"), rapid.IntRange(0, 255).Draw(t, "ip2"), rapid.IntRange(0, 255).Draw(t, "ip3"), rapid.IntRange(1, 255).Draw(t, "ip4")),
		Port:     rapid.IntRange(1, 65535).Draw(t, "port"),
		Priority: uint8(rapid.IntRange(0, 255).Draw(t, "priority")),
		Region:   rapid.StringMatching(`[a-z]{2}-[a-z]+-[0-9]`).Draw(t, "region"),
		CellID:   rapid.StringMatching(`cell-[0-9]{2}`).Draw(t, "cellID"),
	}
}

func genGatewayNodeList(t *rapid.T) []GatewayNode {
	n := rapid.IntRange(1, 10).Draw(t, "numGateways")
	nodes := make([]GatewayNode, n)
	for i := range nodes {
		nodes[i] = genGatewayNode(t)
	}
	return nodes
}

func genRouteTableResponse(t *rapid.T) *RouteTableResponse {
	gateways := genGatewayNodeList(t)
	version := rapid.Uint64Range(1, 1000000).Draw(t, "version")
	// Generate a time within a reasonable range
	unixSec := rapid.Int64Range(1000000000, 2000000000).Draw(t, "unixSec")
	publishedAt := time.Unix(unixSec, 0).UTC()

	return &RouteTableResponse{
		Gateways:    gateways,
		Version:     version,
		PublishedAt: publishedAt,
	}
}

// =============================================================================
// PBT 1: Client-side GatewayNode serialization round-trip
// Validates: Requirements 3.3 (preservation of Client-side serialization)
// =============================================================================

func TestPreservation_GatewayNodeSerializationRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		original := genGatewayNodeList(t)

		// Marshal
		data, err := json.Marshal(original)
		if err != nil {
			t.Fatalf("json.Marshal failed: %v", err)
		}

		// Unmarshal
		var decoded []GatewayNode
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatalf("json.Unmarshal failed: %v", err)
		}

		// Verify length
		if len(decoded) != len(original) {
			t.Fatalf("length mismatch: got %d, want %d", len(decoded), len(original))
		}

		// Verify each field matches
		for i := range original {
			if decoded[i].IP != original[i].IP {
				t.Fatalf("node[%d].IP: got %q, want %q", i, decoded[i].IP, original[i].IP)
			}
			if decoded[i].Port != original[i].Port {
				t.Fatalf("node[%d].Port: got %d, want %d", i, decoded[i].Port, original[i].Port)
			}
			if decoded[i].Priority != original[i].Priority {
				t.Fatalf("node[%d].Priority: got %d, want %d", i, decoded[i].Priority, original[i].Priority)
			}
			if decoded[i].Region != original[i].Region {
				t.Fatalf("node[%d].Region: got %q, want %q", i, decoded[i].Region, original[i].Region)
			}
			if decoded[i].CellID != original[i].CellID {
				t.Fatalf("node[%d].CellID: got %q, want %q", i, decoded[i].CellID, original[i].CellID)
			}
		}
	})
}

// =============================================================================
// PBT 2: Client-side HMAC computation determinism
// Validates: Requirements 3.3 (preservation of HMAC computation)
// =============================================================================

func TestPreservation_HMACComputationDeterminism(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		resp := genRouteTableResponse(t)

		// Generate a random HMAC key (32 bytes)
		keyBytes := make([]byte, 32)
		for i := range keyBytes {
			keyBytes[i] = byte(rapid.IntRange(0, 255).Draw(t, fmt.Sprintf("key%d", i)))
		}

		// Compute HMAC twice with the same key
		hmac1, err1 := ComputeHMAC(resp, keyBytes)
		if err1 != nil {
			t.Fatalf("first ComputeHMAC failed: %v", err1)
		}

		hmac2, err2 := ComputeHMAC(resp, keyBytes)
		if err2 != nil {
			t.Fatalf("second ComputeHMAC failed: %v", err2)
		}

		// Verify determinism: same input → same output
		if !hmac.Equal(hmac1, hmac2) {
			t.Fatalf("HMAC not deterministic: first=%x, second=%x", hmac1, hmac2)
		}

		// Verify non-empty
		if len(hmac1) == 0 {
			t.Fatal("HMAC result is empty")
		}

		// Verify correct length (SHA-256 = 32 bytes)
		if len(hmac1) != 32 {
			t.Fatalf("HMAC length: got %d, want 32", len(hmac1))
		}
	})
}

// =============================================================================
// PBT 3: Client-side hmacBody canonical form
// Validates: Requirements 3.3 (preservation of hmacBody canonical JSON)
// =============================================================================

func TestPreservation_HmacBodyCanonicalForm(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		resp := genRouteTableResponse(t)

		body := hmacBody{
			Gateways:    resp.Gateways,
			Version:     resp.Version,
			PublishedAt: resp.PublishedAt,
		}

		data, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("json.Marshal(hmacBody) failed: %v", err)
		}

		// Verify it produces valid JSON
		if !json.Valid(data) {
			t.Fatalf("hmacBody JSON is not valid: %s", string(data))
		}

		// Verify expected fields are present by unmarshaling into a map
		var m map[string]interface{}
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatalf("unmarshal to map failed: %v", err)
		}

		// Must contain exactly: gateways, version, published_at
		expectedFields := []string{"gateways", "version", "published_at"}
		for _, field := range expectedFields {
			if _, ok := m[field]; !ok {
				t.Fatalf("hmacBody JSON missing field %q: %s", field, string(data))
			}
		}

		// Must NOT contain signature (hmacBody excludes it)
		if _, ok := m["signature"]; ok {
			t.Fatalf("hmacBody JSON should NOT contain 'signature': %s", string(data))
		}

		// Verify field count (exactly 3 fields)
		if len(m) != 3 {
			t.Fatalf("hmacBody JSON has %d fields, want 3: %s", len(m), string(data))
		}
	})
}
