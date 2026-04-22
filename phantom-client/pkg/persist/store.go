// Package persist provides non-sensitive configuration persistence with atomic writes.
package persist

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"phantom-client/pkg/token"
)

// LastEntitlement stores cached entitlement state for offline grace window.
type LastEntitlement struct {
	ExpiresAt      string `json:"expires_at"`
	QuotaRemaining int64  `json:"quota_remaining_bytes"`
	ServiceClass   string `json:"service_class"`
	Banned         bool   `json:"banned"`
	FetchedAt      string `json:"fetched_at"`
}

// PersistConfig holds non-sensitive configuration persisted to local file.
type PersistConfig struct {
	BootstrapPool   []token.GatewayEndpoint `json:"bootstrap_pool"`
	CertFingerprint string                  `json:"cert_fingerprint"`
	UserID          string                  `json:"user_id"`
	OSEndpoint      string                  `json:"os_endpoint"`
	LastEntitlement *LastEntitlement        `json:"last_entitlement,omitempty"`

	// WSS 降级配置（可选，provisioning 时写入）
	WSSSNI      string `json:"wss_sni,omitempty"`
	WSSPort     int    `json:"wss_port,omitempty"`
	WSSPath     string `json:"wss_path,omitempty"`
	WSSCertFile string `json:"wss_cert_file,omitempty"`
	WSSKeyFile  string `json:"wss_key_file,omitempty"`
	WSSCAFile   string `json:"wss_ca_file,omitempty"`
}

// Load reads a PersistConfig from the given JSON file path.
func Load(path string) (*PersistConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("persist load: %w", err)
	}
	var cfg PersistConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("persist unmarshal: %w", err)
	}
	return &cfg, nil
}

// Save writes a PersistConfig to the given path using atomic write
// (write to temp file then rename) to prevent corruption on crash.
func Save(path string, cfg *PersistConfig) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("persist marshal: %w", err)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("persist mkdir: %w", err)
	}

	// Write to temp file in the same directory for atomic rename
	tmp, err := os.CreateTemp(dir, ".persist-*.tmp")
	if err != nil {
		return fmt.Errorf("persist temp: %w", err)
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("persist write: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("persist close: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("persist rename: %w", err)
	}

	return nil
}
