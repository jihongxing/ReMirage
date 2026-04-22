package ebpf

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"sync"
)

const (
	BDNAProfileChromeWin11 uint32 = iota
	BDNAProfileChromeMacOS
	BDNAProfileFirefoxWin11
	BDNAProfileFirefoxLinux
	BDNAProfileSafariMacOS
	BDNAProfileEdgeWin11
)

const BDNAProfileRegistrySchemaV1 = "remirage.bdna.registry/v1"

// BDNAFingerprintProfile 对应 C 结构体 stack_fingerprint。
// 它只负责握手 / 栈画像，不承载跨协议时域模板。
type BDNAFingerprintProfile struct {
	TCPWindow         uint16
	TCPWScale         uint8
	TCPMSS            uint16
	TCPSACKOK         uint8
	TCPTimestamps     uint8
	QUICMaxIdle       uint32
	QUICMaxData       uint32
	QUICMaxStreamsBi  uint32
	QUICMaxStreamsUni uint32
	QUICAckDelayExp   uint16
	TLSVersion        uint16
	TLSExtOrder       [32]uint8
	TLSExtCount       uint8
	ProfileID         uint32
	ProfileName       [32]byte
}

// BDNAProfileRegistry 是 B-DNA 握手画像库的版本化 registry。
// 它将默认画像库从 Go 内嵌常量收敛为仓库内的独立数据快照。
type BDNAProfileRegistry struct {
	SchemaVersion        string                     `json:"schema_version"`
	RegistryVersion      string                     `json:"registry_version"`
	DefaultActiveProfile uint32                     `json:"default_active_profile_id"`
	Profiles             []BDNAProfileRegistryEntry `json:"profiles"`
}

// BDNAProfileRegistryEntry 是 registry 中的单个画像条目。
// 字段设计保持与 stack_fingerprint 一一对应，减少语义漂移。
type BDNAProfileRegistryEntry struct {
	ID                uint32  `json:"id"`
	Name              string  `json:"name"`
	TCPWindow         uint16  `json:"tcp_window"`
	TCPWScale         uint8   `json:"tcp_wscale"`
	TCPMSS            uint16  `json:"tcp_mss"`
	TCPSACKOK         uint8   `json:"tcp_sack_ok"`
	TCPTimestamps     uint8   `json:"tcp_timestamps"`
	QUICMaxIdle       uint32  `json:"quic_max_idle"`
	QUICMaxData       uint32  `json:"quic_max_data"`
	QUICMaxStreamsBi  uint32  `json:"quic_max_streams_bi"`
	QUICMaxStreamsUni uint32  `json:"quic_max_streams_uni"`
	QUICAckDelayExp   uint16  `json:"quic_ack_delay_exp"`
	TLSVersion        uint16  `json:"tls_version"`
	TLSExtOrder       []uint8 `json:"tls_ext_order"`
}

// BDNAProfileUpdater 收敛 B-DNA 的握手画像控制面：
// - fingerprint_map: 画像模板库
// - active_profile_map: 当前激活画像
type BDNAProfileUpdater struct {
	loader   *Loader
	mu       sync.RWMutex
	registry *BDNAProfileRegistry
}

func NewBDNAProfileUpdater(loader *Loader) *BDNAProfileUpdater {
	return &BDNAProfileUpdater{loader: loader}
}

func LoadBDNAProfileRegistry(path string) (*BDNAProfileRegistry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read B-DNA profile registry %q: %w", path, err)
	}

	var registry BDNAProfileRegistry
	if err := json.Unmarshal(data, &registry); err != nil {
		return nil, fmt.Errorf("parse B-DNA profile registry %q: %w", path, err)
	}

	if err := registry.Validate(); err != nil {
		return nil, fmt.Errorf("validate B-DNA profile registry %q: %w", path, err)
	}

	return &registry, nil
}

func (r *BDNAProfileRegistry) Validate() error {
	if r == nil {
		return fmt.Errorf("registry is nil")
	}
	if r.SchemaVersion != BDNAProfileRegistrySchemaV1 {
		return fmt.Errorf("unsupported schema_version %q", r.SchemaVersion)
	}
	if r.RegistryVersion == "" {
		return fmt.Errorf("registry_version is required")
	}
	if len(r.Profiles) == 0 {
		return fmt.Errorf("profiles is empty")
	}

	seenIDs := make(map[uint32]struct{}, len(r.Profiles))
	seenNames := make(map[string]struct{}, len(r.Profiles))
	defaultFound := false

	for _, profile := range r.Profiles {
		if profile.Name == "" {
			return fmt.Errorf("profile %d name is required", profile.ID)
		}
		if len(profile.Name) > 32 {
			return fmt.Errorf("profile %d name exceeds 32 bytes", profile.ID)
		}
		if len(profile.TLSExtOrder) == 0 {
			return fmt.Errorf("profile %d tls_ext_order is required", profile.ID)
		}
		if len(profile.TLSExtOrder) > 32 {
			return fmt.Errorf("profile %d tls_ext_order exceeds 32 entries", profile.ID)
		}
		if _, exists := seenIDs[profile.ID]; exists {
			return fmt.Errorf("duplicate profile id %d", profile.ID)
		}
		if _, exists := seenNames[profile.Name]; exists {
			return fmt.Errorf("duplicate profile name %q", profile.Name)
		}

		seenIDs[profile.ID] = struct{}{}
		seenNames[profile.Name] = struct{}{}

		if profile.ID == r.DefaultActiveProfile {
			defaultFound = true
		}
	}

	if !defaultFound {
		return fmt.Errorf("default_active_profile_id %d not found", r.DefaultActiveProfile)
	}

	return nil
}

func (r *BDNAProfileRegistry) ProfileIDs() []uint32 {
	if r == nil {
		return nil
	}

	ids := make([]uint32, 0, len(r.Profiles))
	for _, profile := range r.Profiles {
		ids = append(ids, profile.ID)
	}

	sort.Slice(ids, func(i, j int) bool {
		return ids[i] < ids[j]
	})

	return ids
}

func (r *BDNAProfileRegistry) ProfileByID(profileID uint32) (BDNAProfileRegistryEntry, bool) {
	if r == nil {
		return BDNAProfileRegistryEntry{}, false
	}

	for _, profile := range r.Profiles {
		if profile.ID == profileID {
			return profile, true
		}
	}

	return BDNAProfileRegistryEntry{}, false
}

func (r *BDNAProfileRegistry) DefaultProfileName() string {
	profile, ok := r.ProfileByID(r.DefaultActiveProfile)
	if !ok {
		return ""
	}
	return profile.Name
}

func (p BDNAProfileRegistryEntry) RuntimeProfile() BDNAFingerprintProfile {
	profile := BDNAFingerprintProfile{
		TCPWindow:         p.TCPWindow,
		TCPWScale:         p.TCPWScale,
		TCPMSS:            p.TCPMSS,
		TCPSACKOK:         p.TCPSACKOK,
		TCPTimestamps:     p.TCPTimestamps,
		QUICMaxIdle:       p.QUICMaxIdle,
		QUICMaxData:       p.QUICMaxData,
		QUICMaxStreamsBi:  p.QUICMaxStreamsBi,
		QUICMaxStreamsUni: p.QUICMaxStreamsUni,
		QUICAckDelayExp:   p.QUICAckDelayExp,
		TLSVersion:        p.TLSVersion,
		TLSExtCount:       uint8(len(p.TLSExtOrder)),
		ProfileID:         p.ID,
	}

	copy(profile.TLSExtOrder[:], p.TLSExtOrder)
	copy(profile.ProfileName[:], []byte(p.Name))

	return profile
}

func (u *BDNAProfileUpdater) SeedRegistry(registry *BDNAProfileRegistry) error {
	if registry == nil {
		return fmt.Errorf("registry is nil")
	}
	if err := registry.Validate(); err != nil {
		return err
	}

	fingerprintMap := u.loader.GetMap("fingerprint_map")
	if fingerprintMap == nil {
		return fmt.Errorf("fingerprint_map not found")
	}

	for _, profileID := range registry.ProfileIDs() {
		entry, ok := registry.ProfileByID(profileID)
		if !ok {
			return fmt.Errorf("profile %d missing from registry", profileID)
		}

		runtimeProfile := entry.RuntimeProfile()
		id := profileID
		if err := fingerprintMap.Put(&id, &runtimeProfile); err != nil {
			return fmt.Errorf("write fingerprint_map[%d]: %w", id, err)
		}
	}

	u.mu.Lock()
	u.registry = registry
	u.mu.Unlock()

	return nil
}

func (u *BDNAProfileUpdater) SeedRegistryFromFile(path string) (*BDNAProfileRegistry, error) {
	registry, err := LoadBDNAProfileRegistry(path)
	if err != nil {
		return nil, err
	}
	if err := u.SeedRegistry(registry); err != nil {
		return nil, err
	}
	return registry, nil
}

func (u *BDNAProfileUpdater) SetActiveProfile(profileID uint32) error {
	u.mu.RLock()
	registry := u.registry
	u.mu.RUnlock()

	if registry != nil {
		if _, ok := registry.ProfileByID(profileID); !ok {
			return fmt.Errorf("profile %d not found in loaded registry", profileID)
		}
	}

	activeProfileMap := u.loader.GetMap("active_profile_map")
	if activeProfileMap == nil {
		return fmt.Errorf("active_profile_map not found")
	}

	key := uint32(0)
	if err := activeProfileMap.Put(&key, &profileID); err != nil {
		return fmt.Errorf("write active_profile_map[%d]: %w", key, err)
	}

	return nil
}

func (u *BDNAProfileUpdater) GetActiveProfile() (uint32, error) {
	activeProfileMap := u.loader.GetMap("active_profile_map")
	if activeProfileMap == nil {
		return 0, fmt.Errorf("active_profile_map not found")
	}

	key := uint32(0)
	var profileID uint32
	if err := activeProfileMap.Lookup(&key, &profileID); err != nil {
		return 0, fmt.Errorf("read active_profile_map[%d]: %w", key, err)
	}

	return profileID, nil
}

func (u *BDNAProfileUpdater) ProfileIDs() []uint32 {
	u.mu.RLock()
	defer u.mu.RUnlock()

	if u.registry == nil {
		return nil
	}

	return u.registry.ProfileIDs()
}
