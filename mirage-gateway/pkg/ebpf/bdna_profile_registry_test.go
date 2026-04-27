package ebpf

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadBDNAProfileRegistry(t *testing.T) {
	t.Parallel()

	registryJSON := `{
  "schema_version": "remirage.bdna.registry/v1",
  "registry_version": "2026-04-23",
  "default_active_profile_id": 5,
  "profiles": [
    {
      "id": 5,
      "name": "edge-win11",
      "tcp_window": 65535,
      "tcp_wscale": 8,
      "tcp_mss": 1460,
      "tcp_sack_ok": 1,
      "tcp_timestamps": 1,
      "quic_max_idle": 30000,
      "quic_max_data": 10485760,
      "quic_max_streams_bi": 100,
      "quic_max_streams_uni": 100,
      "quic_ack_delay_exp": 3,
      "tls_version": 772,
      "tls_ext_order": [0, 23, 35]
    },
    {
      "id": 2,
      "name": "firefox-win11",
      "tcp_window": 65535,
      "tcp_wscale": 7,
      "tcp_mss": 1460,
      "tcp_sack_ok": 1,
      "tcp_timestamps": 1,
      "quic_max_idle": 30000,
      "quic_max_data": 12582912,
      "quic_max_streams_bi": 96,
      "quic_max_streams_uni": 96,
      "quic_ack_delay_exp": 3,
      "tls_version": 772,
      "tls_ext_order": [0, 23, 35, 13]
    }
  ]
}`

	path := filepath.Join(t.TempDir(), "registry.json")
	if err := os.WriteFile(path, []byte(registryJSON), 0o600); err != nil {
		t.Fatalf("write registry: %v", err)
	}

	registry, err := LoadBDNAProfileRegistry(path)
	if err != nil {
		t.Fatalf("LoadBDNAProfileRegistry() error = %v", err)
	}

	if registry.RegistryVersion != "2026-04-23" {
		t.Fatalf("RegistryVersion = %q, want %q", registry.RegistryVersion, "2026-04-23")
	}

	if got := registry.DefaultProfileName(); got != "edge-win11" {
		t.Fatalf("DefaultProfileName() = %q, want %q", got, "edge-win11")
	}

	if got, want := registry.ProfileIDs(), []uint32{2, 5}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ProfileIDs() = %v, want %v", got, want)
	}

	entry, ok := registry.ProfileByID(2)
	if !ok {
		t.Fatalf("ProfileByID(2) not found")
	}

	runtimeProfile := entry.RuntimeProfile()
	if runtimeProfile.TLSExtCount != 4 {
		t.Fatalf("RuntimeProfile().TLSExtCount = %d, want 4", runtimeProfile.TLSExtCount)
	}
	if runtimeProfile.ProfileID != 2 {
		t.Fatalf("RuntimeProfile().ProfileID = %d, want 2", runtimeProfile.ProfileID)
	}
}

func TestBDNAProfileRegistryValidateRejectsDuplicateIDs(t *testing.T) {
	t.Parallel()

	registry := &BDNAProfileRegistry{
		SchemaVersion:        BDNAProfileRegistrySchemaV1,
		RegistryVersion:      "2026-04-23",
		DefaultActiveProfile: 0,
		Profiles: []BDNAProfileRegistryEntry{
			{ID: 0, Name: "chrome-win11", TLSExtOrder: []uint8{0x00}},
			{ID: 0, Name: "edge-win11", TLSExtOrder: []uint8{0x00}},
		},
	}

	if err := registry.Validate(); err == nil {
		t.Fatalf("Validate() error = nil, want duplicate id error")
	}
}

func TestBuildProfileSelectEntriesAllowsSparseProfileIDs(t *testing.T) {
	t.Parallel()

	registry := &BDNAProfileRegistry{
		SchemaVersion:        BDNAProfileRegistrySchemaV1,
		RegistryVersion:      "2026-04-23",
		DefaultActiveProfile: 10,
		Profiles: []BDNAProfileRegistryEntry{
			{ID: 10, Name: "chrome-win", TLSExtOrder: []uint8{0x00}},
			{ID: 42, Name: "firefox-linux", TLSExtOrder: []uint8{0x00}},
		},
	}

	entries, err := BuildProfileSelectEntries(registry, map[uint32]uint32{10: 7, 42: 3})
	if err != nil {
		t.Fatalf("BuildProfileSelectEntries() error = %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("len(entries) = %d, want 2", len(entries))
	}
	if entries[0].ProfileID != 10 || entries[0].CumulativeWeight != 7 {
		t.Fatalf("entries[0] = %+v", entries[0])
	}
	if entries[1].ProfileID != 42 || entries[1].CumulativeWeight != 10 {
		t.Fatalf("entries[1] = %+v", entries[1])
	}
	if err := ValidateProfileSelectEntries(registry, entries); err != nil {
		t.Fatalf("ValidateProfileSelectEntries() error = %v", err)
	}
}

func TestValidateProfileSelectEntriesRejectsInvalidCDF(t *testing.T) {
	t.Parallel()

	registry := &BDNAProfileRegistry{
		SchemaVersion:        BDNAProfileRegistrySchemaV1,
		RegistryVersion:      "2026-04-23",
		DefaultActiveProfile: 1,
		Profiles: []BDNAProfileRegistryEntry{
			{ID: 1, Name: "chrome-win", TLSExtOrder: []uint8{0x00}},
		},
	}

	err := ValidateProfileSelectEntries(registry, []BDNAProfileSelectEntry{
		{CumulativeWeight: 10, ProfileID: 1},
		{CumulativeWeight: 10, ProfileID: 1},
	})
	if err == nil {
		t.Fatalf("ValidateProfileSelectEntries() error = nil, want non-increasing CDF error")
	}
}
