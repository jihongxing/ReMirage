package gswitch

import (
	"crypto/rand"
	"fmt"

	"github.com/cilium/ebpf"
)

var defaultBDNAProfileIDs = []uint32{0, 1, 2, 3, 4, 5}

// BDNAProfileSwitcher 只表达“切换当前 B-DNA 握手画像”。
// G-Switch 通过它联动画像，不再直接关心底层 map 细节。
type BDNAProfileSwitcher interface {
	SetActiveProfile(profileID uint32) error
}

type rawMapBDNAProfileSwitcher struct {
	activeProfileMap *ebpf.Map
}

func (s *rawMapBDNAProfileSwitcher) SetActiveProfile(profileID uint32) error {
	if s.activeProfileMap == nil {
		return fmt.Errorf("active_profile_map not set")
	}

	key := uint32(0)
	return s.activeProfileMap.Put(&key, &profileID)
}

type bdnaProfileCatalog interface {
	ProfileIDs() []uint32
}

func (s *rawMapBDNAProfileSwitcher) ProfileIDs() []uint32 {
	return append([]uint32(nil), defaultBDNAProfileIDs...)
}

func randomBDNAProfileID(switcher BDNAProfileSwitcher) uint32 {
	profileIDs := defaultBDNAProfileIDs
	if catalog, ok := switcher.(bdnaProfileCatalog); ok {
		if ids := catalog.ProfileIDs(); len(ids) > 0 {
			profileIDs = ids
		}
	}

	if len(profileIDs) == 0 {
		return 0
	}

	randBytes := make([]byte, 1)
	if _, err := rand.Read(randBytes); err != nil {
		return profileIDs[0]
	}

	return profileIDs[int(randBytes[0])%len(profileIDs)]
}
