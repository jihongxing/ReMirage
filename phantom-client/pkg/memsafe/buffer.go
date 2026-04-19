package memsafe

import (
	"sync"
)

// SecureBuffer holds sensitive data with mlock protection and secure wipe.
type SecureBuffer struct {
	data   []byte
	locked bool
	wiped  bool
	mu     sync.Mutex
}

var (
	registry   []*SecureBuffer
	registryMu sync.Mutex
)

// NewSecureBuffer allocates a secure buffer and attempts mlock.
func NewSecureBuffer(size int) (*SecureBuffer, error) {
	buf := &SecureBuffer{
		data: make([]byte, size),
	}
	if err := mlock(buf.data); err != nil {
		buf.locked = false
	} else {
		buf.locked = true
	}
	registryMu.Lock()
	registry = append(registry, buf)
	registryMu.Unlock()
	return buf, nil
}

// Write copies data into the secure buffer.
func (sb *SecureBuffer) Write(data []byte) error {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	if len(data) > len(sb.data) {
		if sb.locked {
			_ = munlock(sb.data)
		}
		sb.data = make([]byte, len(data))
		if err := mlock(sb.data); err != nil {
			sb.locked = false
		} else {
			sb.locked = true
		}
	}
	sb.data = sb.data[:len(data)]
	copy(sb.data, data)
	sb.wiped = false
	return nil
}

// Read returns a copy of the buffer contents.
func (sb *SecureBuffer) Read() []byte {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	out := make([]byte, len(sb.data))
	copy(out, sb.data)
	return out
}

// Wipe zeroes every byte and unlocks memory.
func (sb *SecureBuffer) Wipe() {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	for i := range sb.data {
		sb.data[i] = 0
	}
	if sb.locked {
		_ = munlock(sb.data)
		sb.locked = false
	}
	sb.wiped = true
}

// IsWiped returns whether the buffer has been wiped.
func (sb *SecureBuffer) IsWiped() bool {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.wiped
}

// WipeAll wipes every registered SecureBuffer.
func WipeAll() {
	registryMu.Lock()
	defer registryMu.Unlock()
	for _, sb := range registry {
		sb.Wipe()
	}
	registry = nil
}
