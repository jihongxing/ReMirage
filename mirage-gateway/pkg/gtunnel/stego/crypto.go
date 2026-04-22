package stego

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"

	"golang.org/x/crypto/chacha20poly1305"
)

const (
	// HMACTagSize is the size of HMAC-SHA256 tag in bytes.
	HMACTagSize = 32
	// NonceSize is the nonce size for ChaCha20-Poly1305.
	NonceSize = chacha20poly1305.NonceSize // 12
	// AuthTagSize is the authentication tag overhead of ChaCha20-Poly1305.
	AuthTagSize = chacha20poly1305.Overhead // 16
	// KeySize is the required key size (32 bytes).
	KeySize = chacha20poly1305.KeySize // 32
)

// HMACTag computes HMAC-SHA256 over data using key, returning a 32-byte tag.
func HMACTag(key, data []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(data)
	return mac.Sum(nil)
}

// HMACVerify performs constant-time HMAC verification.
func HMACVerify(key, data, tag []byte) bool {
	expected := HMACTag(key, data)
	return subtle.ConstantTimeCompare(expected, tag) == 1
}

// Encrypt encrypts plaintext using ChaCha20-Poly1305.
// Returns nonce + ciphertext (12 + len(plaintext) + 16 bytes).
func Encrypt(key, plaintext []byte) ([]byte, error) {
	if len(key) != KeySize {
		return nil, fmt.Errorf("invalid key size: %d, want %d", len(key), KeySize)
	}
	aead, err := chacha20poly1305.New(key)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, NonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	ciphertext := aead.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

// Decrypt decrypts ciphertext produced by Encrypt.
// Expects nonce (12 bytes) prepended to the ciphertext.
func Decrypt(key, ciphertext []byte) ([]byte, error) {
	if len(key) != KeySize {
		return nil, fmt.Errorf("invalid key size: %d, want %d", len(key), KeySize)
	}
	aead, err := chacha20poly1305.New(key)
	if err != nil {
		return nil, err
	}
	if len(ciphertext) < NonceSize+AuthTagSize {
		return nil, errors.New("ciphertext too short")
	}
	nonce := ciphertext[:NonceSize]
	ct := ciphertext[NonceSize:]
	plaintext, err := aead.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, err
	}
	return plaintext, nil
}

// RandomPadding generates cryptographically secure random bytes of the given length.
func RandomPadding(length int) ([]byte, error) {
	if length <= 0 {
		return nil, nil
	}
	buf := make([]byte, length)
	if _, err := rand.Read(buf); err != nil {
		return nil, err
	}
	return buf, nil
}
