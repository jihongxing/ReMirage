package gtclient

import (
	"testing"
	"time"

	"phantom-client/pkg/token"

	"pgregory.net/rapid"
)

func genGatewayEndpoint(t *rapid.T, label string) token.GatewayEndpoint {
	return token.GatewayEndpoint{
		IP:     rapid.StringMatching(`\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}`).Draw(t, label+"_ip"),
		Port:   rapid.IntRange(1, 65535).Draw(t, label+"_port"),
		Region: rapid.StringMatching(`[a-z]{2}-[a-z]+-\d`).Draw(t, label+"_region"),
	}
}

func genBootstrapPool(t *rapid.T) []token.GatewayEndpoint {
	n := rapid.IntRange(1, 10).Draw(t, "bp_count")
	pool := make([]token.GatewayEndpoint, n)
	for i := range pool {
		pool[i] = genGatewayEndpoint(t, "bp")
	}
	return pool
}

// --- Property 7: BootstrapPool 不可变性测试 ---
// Feature: v1-client-productization, Property 7: BootstrapPool 不可变性
// **Validates: Requirements 4.1, 4.3**
func TestProperty_BootstrapPoolImmutability(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate initial bootstrap pool
		pool := genBootstrapPool(t)

		config := &token.BootstrapConfig{
			BootstrapPool: pool,
			PreSharedKey:  make([]byte, 32),
		}

		c := NewGTunnelClient(config)
		defer c.Close()

		// Snapshot the bootstrap pool right after creation
		initialBP := c.BootstrapPool()

		// Perform a series of RuntimeTopology updates
		numUpdates := rapid.IntRange(1, 20).Draw(t, "numUpdates")
		for i := 0; i < numUpdates; i++ {
			nodes := genGatewayNodes(t, "rt")
			version := uint64(i + 1)
			pubTime := time.Unix(int64(1e9+i), 0).UTC()
			c.runtimeTopo.Update(nodes, version, pubTime)
		}

		// Also mutate the original config pool to verify isolation
		if len(config.BootstrapPool) > 0 {
			config.BootstrapPool[0].IP = "mutated.ip"
		}

		// Verify bootstrap pool is unchanged
		currentBP := c.BootstrapPool()
		if len(currentBP) != len(initialBP) {
			t.Fatalf("bootstrap pool length changed: %d → %d", len(initialBP), len(currentBP))
		}
		for i := range initialBP {
			if currentBP[i] != initialBP[i] {
				t.Fatalf("bootstrap pool[%d] changed: %+v → %+v", i, initialBP[i], currentBP[i])
			}
		}
	})
}

func TestDualPool_RuntimeTopoInitEmpty(t *testing.T) {
	config := makeTestConfig()
	c := NewGTunnelClient(config)
	defer c.Close()

	if !c.runtimeTopo.IsEmpty() {
		t.Fatal("runtimeTopo should be empty on init")
	}
	if len(c.BootstrapPool()) == 0 {
		t.Fatal("bootstrapPool should not be empty")
	}
}

func TestDualPool_BootstrapPoolIsolation(t *testing.T) {
	config := makeTestConfig()
	c := NewGTunnelClient(config)
	defer c.Close()

	// Mutate original config
	config.BootstrapPool[0].IP = "changed"

	bp := c.BootstrapPool()
	if bp[0].IP == "changed" {
		t.Fatal("bootstrapPool should be isolated from config mutations")
	}
}
