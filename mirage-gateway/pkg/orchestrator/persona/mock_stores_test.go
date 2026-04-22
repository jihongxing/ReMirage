package persona

import (
	"fmt"
	"sort"
	"sync"
)

// memManifestStore 内存 ManifestStore 实现
type memManifestStore struct {
	mu         sync.RWMutex
	manifests  map[string][]*PersonaManifest // persona_id -> versions
	sessionMap map[string]string             // session_id -> persona_id (active)
}

func newMemManifestStore() *memManifestStore {
	return &memManifestStore{
		manifests:  make(map[string][]*PersonaManifest),
		sessionMap: make(map[string]string),
	}
}

func (s *memManifestStore) GetMaxVersion(personaID string) (uint64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	versions := s.manifests[personaID]
	var max uint64
	for _, m := range versions {
		if m.Version > max {
			max = m.Version
		}
	}
	return max, nil
}

func (s *memManifestStore) Save(m *PersonaManifest) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := *m
	s.manifests[m.PersonaID] = append(s.manifests[m.PersonaID], &cp)
	return nil
}

func (s *memManifestStore) GetByPersonaIDAndVersion(personaID string, version uint64) (*PersonaManifest, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, m := range s.manifests[personaID] {
		if m.Version == version {
			return m, nil
		}
	}
	return nil, fmt.Errorf("not found")
}

func (s *memManifestStore) GetLatest(personaID string) (*PersonaManifest, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	versions := s.manifests[personaID]
	if len(versions) == 0 {
		return nil, fmt.Errorf("not found")
	}
	var latest *PersonaManifest
	for _, m := range versions {
		if latest == nil || m.Version > latest.Version {
			latest = m
		}
	}
	return latest, nil
}

func (s *memManifestStore) ListVersions(personaID string) ([]*PersonaManifest, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	versions := s.manifests[personaID]
	if len(versions) == 0 {
		return nil, fmt.Errorf("not found")
	}
	result := make([]*PersonaManifest, len(versions))
	copy(result, versions)
	sort.Slice(result, func(i, j int) bool {
		return result[i].Version > result[j].Version
	})
	return result, nil
}

func (s *memManifestStore) GetActiveBySession(sessionID string) (*PersonaManifest, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	personaID, ok := s.sessionMap[sessionID]
	if !ok {
		return nil, fmt.Errorf("no active persona for session %s", sessionID)
	}
	for _, versions := range s.manifests {
		for _, m := range versions {
			if m.PersonaID == personaID && m.Lifecycle == LifecycleActive {
				return m, nil
			}
		}
	}
	return nil, fmt.Errorf("not found")
}

func (s *memManifestStore) FindCoolingBySession(sessionID string) (*PersonaManifest, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, versions := range s.manifests {
		for _, m := range versions {
			if m.Lifecycle == LifecycleCooling {
				return m, nil
			}
		}
	}
	return nil, fmt.Errorf("no cooling persona")
}

func (s *memManifestStore) UpdateLifecycle(personaID string, version uint64, lifecycle PersonaLifecycle) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, m := range s.manifests[personaID] {
		if m.Version == version {
			m.Lifecycle = lifecycle
			return nil
		}
	}
	return fmt.Errorf("not found")
}

// SetSessionPersona 设置 session 到 persona 的映射（测试辅助）
func (s *memManifestStore) SetSessionPersona(sessionID, personaID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessionMap[sessionID] = personaID
}

// memSessionStore 内存 SessionStore
type memSessionStore struct {
	mu       sync.RWMutex
	sessions map[string]string
}

func newMemSessionStore() *memSessionStore {
	return &memSessionStore{sessions: make(map[string]string)}
}

func (s *memSessionStore) GetCurrentPersonaID(sessionID string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sessions[sessionID], nil
}

func (s *memSessionStore) SetCurrentPersonaID(sessionID, personaID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[sessionID] = personaID
	return nil
}

// memControlStore 内存 ControlStore
type memControlStore struct {
	mu             sync.RWMutex
	epoch          uint64
	personaVersion uint64
}

func newMemControlStore(epoch uint64) *memControlStore {
	return &memControlStore{epoch: epoch}
}

func (s *memControlStore) GetEpoch() uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.epoch
}

func (s *memControlStore) GetPersonaVersion() uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.personaVersion
}

func (s *memControlStore) SetPersonaVersion(v uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.personaVersion = v
	return nil
}
