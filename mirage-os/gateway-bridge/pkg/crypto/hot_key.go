package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"log"
	"sync"
	"time"
)

// HotKey 热密钥管理器
type HotKey struct {
	mu        sync.RWMutex
	key       []byte
	active    bool
	activedAt time.Time
}

func NewHotKey() *HotKey { return &HotKey{} }

func (hk *HotKey) Activate(masterKey []byte) error {
	hk.mu.Lock()
	defer hk.mu.Unlock()
	hk.key = make([]byte, len(masterKey))
	copy(hk.key, masterKey)
	if err := mlock(hk.key); err != nil {
		log.Printf("[WARN] mlock failed (degraded mode): %v", err)
	}
	hk.active = true
	hk.activedAt = time.Now()
	return nil
}

func (hk *HotKey) Deactivate() {
	hk.mu.Lock()
	defer hk.mu.Unlock()
	if hk.key != nil {
		for i := range hk.key {
			hk.key[i] = 0
		}
		_ = munlock(hk.key)
	}
	hk.active = false
}

func (hk *HotKey) IsActive() bool {
	hk.mu.RLock()
	defer hk.mu.RUnlock()
	return hk.active
}

func (hk *HotKey) Encrypt(plaintext []byte) ([]byte, error) {
	hk.mu.RLock()
	defer hk.mu.RUnlock()
	if !hk.active {
		return nil, fmt.Errorf("hot key not active")
	}
	block, err := aes.NewCipher(hk.key)
	if err != nil {
		return nil, fmt.Errorf("new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("new gcm: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}
	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

func (hk *HotKey) Decrypt(ciphertext []byte) ([]byte, error) {
	hk.mu.RLock()
	defer hk.mu.RUnlock()
	if !hk.active {
		return nil, fmt.Errorf("hot key not active")
	}
	block, err := aes.NewCipher(hk.key)
	if err != nil {
		return nil, fmt.Errorf("new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("new gcm: %w", err)
	}
	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}
	nonce, ct := ciphertext[:nonceSize], ciphertext[nonceSize:]
	return gcm.Open(nil, nonce, ct, nil)
}

func (hk *HotKey) RecoverFromShares(engine *ShamirEngine, shares []Share) error {
	secret, err := engine.Combine(shares)
	if err != nil {
		return fmt.Errorf("combine shares: %w", err)
	}
	return hk.Activate(secret)
}

// GetKey 返回密钥的副本（仅用于测试）
func (hk *HotKey) GetKey() []byte {
	hk.mu.RLock()
	defer hk.mu.RUnlock()
	if hk.key == nil {
		return nil
	}
	cp := make([]byte, len(hk.key))
	copy(cp, hk.key)
	return cp
}
