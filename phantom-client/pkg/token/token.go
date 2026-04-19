package token

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"golang.org/x/crypto/chacha20poly1305"

	"phantom-client/pkg/memsafe"
)

var errInvalidToken = errors.New("Invalid token")

// GatewayEndpoint represents a single gateway entry point.
type GatewayEndpoint struct {
	IP     string `json:"ip"`
	Port   int    `json:"port"`
	Region string `json:"region"`
}

// BootstrapConfig holds the decrypted token payload.
type BootstrapConfig struct {
	BootstrapPool   []GatewayEndpoint `json:"bootstrap_pool"`
	AuthKey         []byte            `json:"auth_key"`
	PreSharedKey    []byte            `json:"psk"`
	CertFingerprint string            `json:"cert_fp"`
	UserID          string            `json:"user_id"`
	ExpiresAt       time.Time         `json:"expires_at"`
	secureBuf       *memsafe.SecureBuffer
}

// ParseToken decodes base64 → ChaCha20-Poly1305 decrypt → JSON → SecureBuffer lock.
// key must be 32 bytes.
func ParseToken(tokenStr string, key []byte) (*BootstrapConfig, error) {
	raw, err := base64.StdEncoding.DecodeString(tokenStr)
	if err != nil {
		return nil, errInvalidToken
	}

	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, errInvalidToken
	}

	nonceSize := aead.NonceSize()
	if len(raw) < nonceSize {
		return nil, errInvalidToken
	}

	nonce, ciphertext := raw[:nonceSize], raw[nonceSize:]
	plaintext, err := aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, errInvalidToken
	}

	var config BootstrapConfig
	if err := json.Unmarshal(plaintext, &config); err != nil {
		return nil, errInvalidToken
	}

	// Lock sensitive data in memory
	buf, _ := memsafe.NewSecureBuffer(len(plaintext))
	_ = buf.Write(plaintext)
	config.secureBuf = buf

	// Zero plaintext slice
	for i := range plaintext {
		plaintext[i] = 0
	}

	return &config, nil
}

// TokenToBase64 serializes config → encrypts with ChaCha20-Poly1305 → base64.
func TokenToBase64(config *BootstrapConfig, key []byte) (string, error) {
	plaintext, err := json.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("marshal: %w", err)
	}

	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return "", fmt.Errorf("cipher: %w", err)
	}

	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("nonce: %w", err)
	}

	ciphertext := aead.Seal(nonce, nonce, plaintext, nil)

	// Zero plaintext
	for i := range plaintext {
		plaintext[i] = 0
	}

	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// WipeConfig securely erases the config's secure buffer.
func (c *BootstrapConfig) WipeConfig() {
	if c.secureBuf != nil {
		c.secureBuf.Wipe()
	}
}
