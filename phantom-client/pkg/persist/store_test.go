package persist

import (
	"os"
	"path/filepath"
	"testing"

	"phantom-client/pkg/token"

	"pgregory.net/rapid"
)

// Feature: v1-client-productization, Property 3: PersistConfig 序列化往返
// **Validates: Requirements 9.3**
func TestProperty3_PersistConfig_RoundTrip(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Generate arbitrary PersistConfig
		poolSize := rapid.IntRange(0, 5).Draw(rt, "poolSize")
		pool := make([]token.GatewayEndpoint, poolSize)
		for i := range pool {
			pool[i] = token.GatewayEndpoint{
				IP:     rapid.StringMatching(`\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}`).Draw(rt, "ip"),
				Port:   rapid.IntRange(1, 65535).Draw(rt, "port"),
				Region: rapid.StringMatching(`[a-z]{2}-[a-z]+-\d`).Draw(rt, "region"),
			}
		}

		hasEntitlement := rapid.Bool().Draw(rt, "hasEntitlement")
		var lastEnt *LastEntitlement
		if hasEntitlement {
			lastEnt = &LastEntitlement{
				ExpiresAt:      rapid.StringMatching(`2025-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z`).Draw(rt, "expiresAt"),
				QuotaRemaining: rapid.Int64Range(0, 1<<40).Draw(rt, "quota"),
				ServiceClass:   rapid.SampledFrom([]string{"standard", "platinum", "diamond"}).Draw(rt, "class"),
				Banned:         rapid.Bool().Draw(rt, "banned"),
				FetchedAt:      rapid.StringMatching(`2025-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z`).Draw(rt, "fetchedAt"),
			}
		}

		original := &PersistConfig{
			BootstrapPool:   pool,
			CertFingerprint: rapid.StringMatching(`[a-f0-9]{64}`).Draw(rt, "certFP"),
			UserID:          rapid.StringMatching(`usr-[a-z0-9]{8}`).Draw(rt, "userID"),
			OSEndpoint:      rapid.StringMatching(`https://os\.\w+\.com:\d{1,5}`).Draw(rt, "osEndpoint"),
			LastEntitlement: lastEnt,
		}

		// Save → Load round-trip
		dir, err := os.MkdirTemp("", "persist-test-*")
		if err != nil {
			rt.Fatalf("MkdirTemp: %v", err)
		}
		defer os.RemoveAll(dir)
		path := filepath.Join(dir, "config.json")

		if err := Save(path, original); err != nil {
			rt.Fatalf("Save failed: %v", err)
		}

		loaded, loadErr := Load(path)
		if loadErr != nil {
			rt.Fatalf("Load failed: %v", loadErr)
		}

		// Verify equivalence
		if len(loaded.BootstrapPool) != len(original.BootstrapPool) {
			rt.Fatalf("BootstrapPool length mismatch: got %d, want %d", len(loaded.BootstrapPool), len(original.BootstrapPool))
		}
		for i, gw := range original.BootstrapPool {
			if loaded.BootstrapPool[i] != gw {
				rt.Fatalf("BootstrapPool[%d] mismatch: got %+v, want %+v", i, loaded.BootstrapPool[i], gw)
			}
		}
		if loaded.CertFingerprint != original.CertFingerprint {
			rt.Fatalf("CertFingerprint mismatch")
		}
		if loaded.UserID != original.UserID {
			rt.Fatalf("UserID mismatch")
		}
		if loaded.OSEndpoint != original.OSEndpoint {
			rt.Fatalf("OSEndpoint mismatch")
		}

		// Check LastEntitlement
		if (original.LastEntitlement == nil) != (loaded.LastEntitlement == nil) {
			rt.Fatalf("LastEntitlement nil mismatch")
		}
		if original.LastEntitlement != nil {
			if *loaded.LastEntitlement != *original.LastEntitlement {
				rt.Fatalf("LastEntitlement mismatch: got %+v, want %+v", *loaded.LastEntitlement, *original.LastEntitlement)
			}
		}
	})
}

func TestSave_AtomicWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := &PersistConfig{
		BootstrapPool:   []token.GatewayEndpoint{{IP: "1.2.3.4", Port: 443, Region: "us-east-1"}},
		CertFingerprint: "abc123",
		UserID:          "usr-test",
		OSEndpoint:      "https://os.example.com:8443",
	}

	if err := Save(path, cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Verify no temp files remain
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".tmp" {
			t.Errorf("temp file not cleaned up: %s", e.Name())
		}
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.UserID != "usr-test" {
		t.Errorf("UserID mismatch: got %s", loaded.UserID)
	}
}
