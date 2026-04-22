package gtclient

import (
	"encoding/json"
	"testing"
	"time"

	"pgregory.net/rapid"
)

// --- Generators ---

func genGatewayNode(t *rapid.T, label string) GatewayNode {
	return GatewayNode{
		IP:       rapid.StringMatching(`\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}`).Draw(t, label+"_ip"),
		Port:     rapid.IntRange(1, 65535).Draw(t, label+"_port"),
		Priority: uint8(rapid.IntRange(0, 255).Draw(t, label+"_priority")),
		Region:   rapid.StringMatching(`[a-z]{2}-[a-z]+-\d`).Draw(t, label+"_region"),
		CellID:   rapid.StringMatching(`cell-[0-9]{2}`).Draw(t, label+"_cellid"),
	}
}

func genGatewayNodes(t *rapid.T, label string) []GatewayNode {
	n := rapid.IntRange(1, 10).Draw(t, label+"_count")
	nodes := make([]GatewayNode, n)
	for i := range nodes {
		nodes[i] = genGatewayNode(t, label)
	}
	return nodes
}

func genRouteTableResponse(t *rapid.T) RouteTableResponse {
	return RouteTableResponse{
		Gateways:    genGatewayNodes(t, "gw"),
		Version:     rapid.Uint64Range(1, 1<<32).Draw(t, "version"),
		PublishedAt: time.Unix(rapid.Int64Range(1e9, 2e9).Draw(t, "pub_ts"), 0).UTC(),
		Signature:   rapid.SliceOfN(rapid.Byte(), 32, 32).Draw(t, "sig"),
	}
}

// --- Property 1: RouteTableResponse 序列化往返测试 ---
// Feature: v1-client-productization, Property 1: RouteTableResponse 序列化往返
// **Validates: Requirements 1.2**
func TestProperty_RouteTableResponse_RoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		orig := genRouteTableResponse(t)

		data, err := json.Marshal(orig)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}

		var decoded RouteTableResponse
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}

		// Compare fields
		if len(decoded.Gateways) != len(orig.Gateways) {
			t.Fatalf("gateways count: %d vs %d", len(decoded.Gateways), len(orig.Gateways))
		}
		for i := range orig.Gateways {
			if decoded.Gateways[i] != orig.Gateways[i] {
				t.Fatalf("gateway[%d] mismatch: %+v vs %+v", i, decoded.Gateways[i], orig.Gateways[i])
			}
		}
		if decoded.Version != orig.Version {
			t.Fatalf("version: %d vs %d", decoded.Version, orig.Version)
		}
		if !decoded.PublishedAt.Equal(orig.PublishedAt) {
			t.Fatalf("published_at: %v vs %v", decoded.PublishedAt, orig.PublishedAt)
		}
		if len(decoded.Signature) != len(orig.Signature) {
			t.Fatalf("signature length: %d vs %d", len(decoded.Signature), len(orig.Signature))
		}
		for i := range orig.Signature {
			if decoded.Signature[i] != orig.Signature[i] {
				t.Fatalf("signature[%d] mismatch", i)
			}
		}
	})
}

// --- Property 10: 单调版本+时间戳接受策略测试 ---
// Feature: v1-client-productization, Property 10: 单调版本+时间戳接受策略
// **Validates: Requirements 2.3, 5.3**
func TestProperty_MonotonicVersionTimestamp(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		psk := rapid.SliceOfN(rapid.Byte(), 32, 64).Draw(t, "psk")
		tv := NewTopoVerifier(psk)

		// Set initial state by accepting a valid response
		baseVersion := rapid.Uint64Range(1, 1<<16).Draw(t, "baseVersion")
		baseTime := time.Unix(rapid.Int64Range(1e9, 1_500_000_000).Draw(t, "baseTime"), 0).UTC()

		initResp := &RouteTableResponse{
			Gateways:    []GatewayNode{{IP: "1.2.3.4", Port: 443, Priority: 0, Region: "us", CellID: "c1"}},
			Version:     baseVersion,
			PublishedAt: baseTime,
		}
		sig, _ := ComputeHMAC(initResp, tv.hmacKey)
		initResp.Signature = sig
		if err := tv.Verify(initResp); err != nil {
			t.Fatalf("init verify: %v", err)
		}

		// Generate new version and time
		newVersion := rapid.Uint64Range(0, baseVersion+100).Draw(t, "newVersion")
		timeDelta := rapid.Int64Range(-3600, 3600).Draw(t, "timeDelta")
		newTime := baseTime.Add(time.Duration(timeDelta) * time.Second)

		newResp := &RouteTableResponse{
			Gateways:    []GatewayNode{{IP: "5.6.7.8", Port: 443, Priority: 1, Region: "eu", CellID: "c2"}},
			Version:     newVersion,
			PublishedAt: newTime,
		}
		newSig, _ := ComputeHMAC(newResp, tv.hmacKey)
		newResp.Signature = newSig

		err := tv.Verify(newResp)

		shouldAccept := newVersion > baseVersion && newTime.After(baseTime)
		if shouldAccept && err != nil {
			t.Fatalf("expected accept (v=%d>%d, t=%v>%v) but got: %v",
				newVersion, baseVersion, newTime, baseTime, err)
		}
		if !shouldAccept && err == nil {
			t.Fatalf("expected reject (v=%d, base=%d, t=%v, baseT=%v) but was accepted",
				newVersion, baseVersion, newTime, baseTime)
		}
	})
}

// --- Property 11: HMAC 签名校验测试 ---
// Feature: v1-client-productization, Property 11: HMAC 签名校验
// **Validates: Requirements 5.2**
func TestProperty_HMACSignatureVerification(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		psk := rapid.SliceOfN(rapid.Byte(), 32, 64).Draw(t, "psk")
		tv := NewTopoVerifier(psk)

		resp := &RouteTableResponse{
			Gateways:    genGatewayNodes(t, "gw"),
			Version:     rapid.Uint64Range(1, 1<<32).Draw(t, "version"),
			PublishedAt: time.Unix(rapid.Int64Range(1e9, 2e9).Draw(t, "pub_ts"), 0).UTC(),
		}

		// Compute correct signature
		sig, err := ComputeHMAC(resp, tv.hmacKey)
		if err != nil {
			t.Fatalf("compute hmac: %v", err)
		}
		resp.Signature = sig

		// Valid signature should pass
		if err := tv.Verify(resp); err != nil {
			t.Fatalf("valid signature rejected: %v", err)
		}

		// Now tamper with the response body and verify it fails
		// Create a fresh verifier to avoid version/time state issues
		tv2 := NewTopoVerifier(psk)

		tamperedResp := &RouteTableResponse{
			Gateways:    resp.Gateways,
			Version:     resp.Version + 1, // tamper version
			PublishedAt: resp.PublishedAt.Add(time.Second),
			Signature:   sig, // keep old signature
		}

		if err := tv2.Verify(tamperedResp); err == nil {
			t.Fatal("tampered response should have failed signature check")
		}

		// Also test: wrong key should fail
		wrongPsk := rapid.SliceOfN(rapid.Byte(), 32, 64).Draw(t, "wrongPsk")
		tv3 := NewTopoVerifier(wrongPsk)
		wrongResp := &RouteTableResponse{
			Gateways:    resp.Gateways,
			Version:     resp.Version + 2,
			PublishedAt: resp.PublishedAt.Add(2 * time.Second),
			Signature:   sig,
		}
		if err := tv3.Verify(wrongResp); err == nil {
			t.Fatal("wrong key should have failed signature check")
		}
	})
}

// --- Property 5: RuntimeTopology 更新正确性测试 ---
// Feature: v1-client-productization, Property 5: RuntimeTopology 更新正确性
// **Validates: Requirements 1.3, 3.3**
func TestProperty_RuntimeTopology_UpdateCorrectness(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		nodes := genGatewayNodes(t, "node")
		version := rapid.Uint64Range(1, 1<<32).Draw(t, "version")
		pubTime := time.Unix(rapid.Int64Range(1e9, 2e9).Draw(t, "pub_ts"), 0).UTC()

		rt := &RuntimeTopology{}
		rt.Update(nodes, version, pubTime)

		// Count should equal input length
		if rt.Count() != len(nodes) {
			t.Fatalf("count: %d vs %d", rt.Count(), len(nodes))
		}

		// Version should match
		if rt.Version() != version {
			t.Fatalf("version: %d vs %d", rt.Version(), version)
		}

		// IsEmpty should be false
		if rt.IsEmpty() {
			t.Fatal("should not be empty after update")
		}

		// Every node should be retrievable via AllByPriority
		all := rt.AllByPriority("")
		if len(all) != len(nodes) {
			t.Fatalf("AllByPriority count: %d vs %d", len(all), len(nodes))
		}
	})
}

// --- Property 8: 优先级有序网关选择测试 ---
// Feature: v1-client-productization, Property 8: 优先级有序网关选择
// **Validates: Requirements 1.5**
func TestProperty_PriorityOrderedGatewaySelection(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		nodes := genGatewayNodes(t, "node")

		rt := &RuntimeTopology{}
		rt.Update(nodes, 1, time.Now().UTC())

		all := rt.AllByPriority("")

		// Verify sorted by priority ascending
		for i := 1; i < len(all); i++ {
			if all[i].Priority < all[i-1].Priority {
				t.Fatalf("not sorted: priority[%d]=%d < priority[%d]=%d",
					i, all[i].Priority, i-1, all[i-1].Priority)
			}
		}

		// NextByPriority should return the lowest priority node
		if len(all) > 0 {
			first, err := rt.NextByPriority("")
			if err != nil {
				t.Fatalf("NextByPriority: %v", err)
			}
			if first.Priority != all[0].Priority {
				t.Fatalf("NextByPriority returned priority %d, expected %d",
					first.Priority, all[0].Priority)
			}
		}

		// Exclude test: excluding an IP should not return that IP
		if len(nodes) > 0 {
			excludeIP := nodes[0].IP
			filtered := rt.AllByPriority(excludeIP)
			for _, n := range filtered {
				if n.IP == excludeIP {
					t.Fatalf("excluded IP %s still present", excludeIP)
				}
			}
		}
	})
}

// --- Unit Tests ---

func TestTopoVerifier_EmptyGateways(t *testing.T) {
	tv := NewTopoVerifier(make([]byte, 32))
	resp := &RouteTableResponse{
		Gateways:    []GatewayNode{},
		Version:     1,
		PublishedAt: time.Now().UTC(),
		Signature:   make([]byte, 32),
	}
	err := tv.Verify(resp)
	if err == nil {
		t.Fatal("expected error for empty gateways")
	}
}

func TestTopoVerifier_PauseAfter3Failures(t *testing.T) {
	psk := make([]byte, 32)
	for i := range psk {
		psk[i] = byte(i)
	}
	tv := NewTopoVerifier(psk)

	for i := 0; i < 3; i++ {
		resp := &RouteTableResponse{
			Gateways:    []GatewayNode{{IP: "1.2.3.4", Port: 443, Priority: 0, Region: "us", CellID: "c1"}},
			Version:     uint64(i + 1),
			PublishedAt: time.Now().Add(time.Duration(i+1) * time.Second).UTC(),
			Signature:   []byte("bad-signature-that-wont-match!!"), // wrong sig
		}
		_ = tv.Verify(resp)
	}

	if !tv.IsPaused() {
		t.Fatal("expected verifier to be paused after 3 signature failures")
	}
}

func TestRuntimeTopology_Empty(t *testing.T) {
	rt := &RuntimeTopology{}
	if !rt.IsEmpty() {
		t.Fatal("expected empty")
	}
	if rt.Count() != 0 {
		t.Fatal("expected count 0")
	}
	if rt.Version() != 0 {
		t.Fatal("expected version 0")
	}
	_, err := rt.NextByPriority("")
	if err == nil {
		t.Fatal("expected error from empty topology")
	}
}

func TestRuntimeTopology_UpdateIsolation(t *testing.T) {
	rt := &RuntimeTopology{}
	nodes := []GatewayNode{
		{IP: "1.1.1.1", Port: 443, Priority: 2},
		{IP: "2.2.2.2", Port: 443, Priority: 0},
	}
	rt.Update(nodes, 1, time.Now().UTC())

	// Mutate original slice — should not affect topology
	nodes[0].IP = "9.9.9.9"
	all := rt.AllByPriority("")
	for _, n := range all {
		if n.IP == "9.9.9.9" {
			t.Fatal("topology should be isolated from original slice mutation")
		}
	}
}
