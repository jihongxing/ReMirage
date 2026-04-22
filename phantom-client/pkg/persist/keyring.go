package persist

import (
	"fmt"
	"log"
	"sync"
)

// Keyring provides OS-level secret storage abstraction.
// Store/Load/Delete sensitive materials (PSK, AuthKey) without writing to disk.
type Keyring interface {
	Store(service, key string, value []byte) error
	Load(service, key string) ([]byte, error)
	Delete(service, key string) error
}

// memoryKeyring is an in-memory fallback when OS keyring is unavailable.
// WARNING: secrets are lost on process exit. For production, integrate
// go-keyring (github.com/zalando/go-keyring) for OS-native secret storage
// (Linux: Secret Service/keyctl, Windows: Credential Manager, macOS: Keychain).
type memoryKeyring struct {
	mu    sync.RWMutex
	store map[string][]byte
}

// NewKeyring returns a Keyring implementation.
// Currently returns memoryKeyring as fallback. To use OS-native keyring,
// add go-keyring dependency and swap this factory.
func NewKeyring() Keyring {
	log.Println("[Keyring] WARNING: using in-memory keyring fallback. Secrets will be lost on process exit. Add go-keyring for production OS keyring support.")
	return &memoryKeyring{
		store: make(map[string][]byte),
	}
}

func keyringKey(service, key string) string {
	return service + "/" + key
}

func (m *memoryKeyring) Store(service, key string, value []byte) error {
	if service == "" || key == "" {
		return fmt.Errorf("keyring: service and key must not be empty")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	// Copy value to avoid external mutation
	cp := make([]byte, len(value))
	copy(cp, value)
	m.store[keyringKey(service, key)] = cp
	return nil
}

func (m *memoryKeyring) Load(service, key string) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	v, ok := m.store[keyringKey(service, key)]
	if !ok {
		return nil, fmt.Errorf("keyring: key %s/%s not found", service, key)
	}
	// Return copy
	cp := make([]byte, len(v))
	copy(cp, v)
	return cp, nil
}

func (m *memoryKeyring) Delete(service, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := keyringKey(service, key)
	if v, ok := m.store[k]; ok {
		// Zero out before delete
		for i := range v {
			v[i] = 0
		}
		delete(m.store, k)
	}
	return nil
}
