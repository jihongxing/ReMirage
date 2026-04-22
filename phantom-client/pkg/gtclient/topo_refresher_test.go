package gtclient

import (
	"context"
	"fmt"
	"testing"
	"time"

	"pgregory.net/rapid"
)

// --- Property 6: 拉取失败保留现有拓扑测试 ---
// Feature: v1-client-productization, Property 6: 拉取失败保留现有拓扑
// **Validates: Requirements 1.4**
func TestProperty_PullFailurePreservesTopology(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate initial topology state
		nodes := genGatewayNodes(t, "init")
		version := rapid.Uint64Range(1, 1<<32).Draw(t, "version")
		pubTime := time.Unix(rapid.Int64Range(1e9, 2e9).Draw(t, "pub_ts"), 0).UTC()

		topo := &RuntimeTopology{}
		topo.Update(nodes, version, pubTime)

		// Snapshot before pull
		nodesBefore, versionBefore, updatedBefore := topo.Snapshot()

		// Create a TopoRefresher with a fetcher that always fails
		psk := rapid.SliceOfN(rapid.Byte(), 32, 32).Draw(t, "psk")
		verifier := NewTopoVerifier(psk)

		failFetcher := func(ctx context.Context) (*RouteTableResponse, error) {
			return nil, fmt.Errorf("simulated fetch failure")
		}

		tr := NewTopoRefresher(TopoRefresherConfig{
			Fetcher:  failFetcher,
			Verifier: verifier,
			Topo:     topo,
		})

		// Pull should fail
		err := tr.PullOnce(context.Background())
		if err == nil {
			t.Fatal("expected PullOnce to fail")
		}

		// Verify topology is unchanged
		nodesAfter, versionAfter, updatedAfter := topo.Snapshot()

		if versionAfter != versionBefore {
			t.Fatalf("version changed: %d → %d", versionBefore, versionAfter)
		}
		if !updatedAfter.Equal(updatedBefore) {
			t.Fatalf("updatedAt changed: %v → %v", updatedBefore, updatedAfter)
		}
		if len(nodesAfter) != len(nodesBefore) {
			t.Fatalf("node count changed: %d → %d", len(nodesBefore), len(nodesAfter))
		}
		for i := range nodesBefore {
			if nodesAfter[i] != nodesBefore[i] {
				t.Fatalf("node[%d] changed", i)
			}
		}
	})
}

func TestTopoRefresher_AlertAfter3Failures(t *testing.T) {
	topo := &RuntimeTopology{}
	psk := make([]byte, 32)
	verifier := NewTopoVerifier(psk)

	alertCount := 0
	failFetcher := func(ctx context.Context) (*RouteTableResponse, error) {
		return nil, fmt.Errorf("fail")
	}

	tr := NewTopoRefresher(TopoRefresherConfig{
		Fetcher:  failFetcher,
		Verifier: verifier,
		Topo:     topo,
		OnAlert:  func(msg string) { alertCount++ },
	})

	ctx := context.Background()
	for i := 0; i < 5; i++ {
		tr.PullOnce(ctx)
	}

	// Alert should fire on failures 3, 4, 5
	if alertCount != 3 {
		t.Fatalf("expected 3 alerts, got %d", alertCount)
	}
	if tr.ConsecutiveFailures() != 5 {
		t.Fatalf("expected 5 consecutive failures, got %d", tr.ConsecutiveFailures())
	}
}

func TestTopoRefresher_SuccessResetsBackoff(t *testing.T) {
	topo := &RuntimeTopology{}
	psk := make([]byte, 32)
	verifier := NewTopoVerifier(psk)

	callCount := 0
	fetcher := func(ctx context.Context) (*RouteTableResponse, error) {
		callCount++
		if callCount <= 2 {
			return nil, fmt.Errorf("fail")
		}
		// Return a valid response
		resp := &RouteTableResponse{
			Gateways:    []GatewayNode{{IP: "1.2.3.4", Port: 443, Priority: 0, Region: "us", CellID: "c1"}},
			Version:     uint64(callCount),
			PublishedAt: time.Now().UTC(),
		}
		sig, _ := ComputeHMAC(resp, verifier.hmacKey)
		resp.Signature = sig
		return resp, nil
	}

	tr := NewTopoRefresher(TopoRefresherConfig{
		Fetcher:  fetcher,
		Verifier: verifier,
		Topo:     topo,
	})

	ctx := context.Background()

	// Two failures
	tr.PullOnce(ctx)
	tr.PullOnce(ctx)
	if tr.ConsecutiveFailures() != 2 {
		t.Fatalf("expected 2 failures, got %d", tr.ConsecutiveFailures())
	}

	// Success should reset
	err := tr.PullOnce(ctx)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if tr.ConsecutiveFailures() != 0 {
		t.Fatalf("expected 0 failures after success, got %d", tr.ConsecutiveFailures())
	}
	if tr.backoff.FailCount != 0 {
		t.Fatalf("expected backoff reset, got failCount=%d", tr.backoff.FailCount)
	}
}

func TestTopoRefresher_VerifyFailurePreservesTopo(t *testing.T) {
	topo := &RuntimeTopology{}
	topo.Update(
		[]GatewayNode{{IP: "1.1.1.1", Port: 443, Priority: 0, Region: "us", CellID: "c1"}},
		10, time.Now().UTC(),
	)

	psk := make([]byte, 32)
	verifier := NewTopoVerifier(psk)

	// Fetcher returns a response with bad signature
	fetcher := func(ctx context.Context) (*RouteTableResponse, error) {
		return &RouteTableResponse{
			Gateways:    []GatewayNode{{IP: "2.2.2.2", Port: 443, Priority: 0, Region: "eu", CellID: "c2"}},
			Version:     11,
			PublishedAt: time.Now().Add(time.Hour).UTC(),
			Signature:   []byte("bad-sig"),
		}, nil
	}

	tr := NewTopoRefresher(TopoRefresherConfig{
		Fetcher:  fetcher,
		Verifier: verifier,
		Topo:     topo,
	})

	tr.PullOnce(context.Background())

	// Topology should still have original data
	if topo.Version() != 10 {
		t.Fatalf("expected version 10, got %d", topo.Version())
	}
	if topo.Count() != 1 {
		t.Fatalf("expected 1 node, got %d", topo.Count())
	}
}
