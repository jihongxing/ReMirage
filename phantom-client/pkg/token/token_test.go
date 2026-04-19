package token

import (
	"crypto/rand"
	"testing"
	"time"

	"pgregory.net/rapid"
)

func genKey() []byte {
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	return key
}

// Property 1: Token 往返一致性
func TestProperty_TokenRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		key := genKey()
		nEndpoints := rapid.IntRange(1, 5).Draw(t, "nEndpoints")
		pool := make([]GatewayEndpoint, nEndpoints)
		for i := range pool {
			pool[i] = GatewayEndpoint{
				IP:     rapid.StringMatching(`\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}`).Draw(t, "ip"),
				Port:   rapid.IntRange(1, 65535).Draw(t, "port"),
				Region: rapid.StringMatching(`[a-z]{2}-[a-z]+-\d`).Draw(t, "region"),
			}
		}
		authKey := rapid.SliceOfN(rapid.Byte(), 16, 64).Draw(t, "authKey")
		psk := rapid.SliceOfN(rapid.Byte(), 16, 64).Draw(t, "psk")

		original := &BootstrapConfig{
			BootstrapPool:   pool,
			AuthKey:         authKey,
			PreSharedKey:    psk,
			CertFingerprint: rapid.String().Draw(t, "certFP"),
			UserID:          rapid.String().Draw(t, "userID"),
			ExpiresAt:       time.Now().Add(time.Hour).Truncate(time.Second),
		}

		encoded, err := TokenToBase64(original, key)
		if err != nil {
			t.Fatal(err)
		}

		decoded, err := ParseToken(encoded, key)
		if err != nil {
			t.Fatal(err)
		}

		if len(decoded.BootstrapPool) != len(original.BootstrapPool) {
			t.Fatalf("pool length mismatch: %d vs %d", len(decoded.BootstrapPool), len(original.BootstrapPool))
		}
		if string(decoded.AuthKey) != string(original.AuthKey) {
			t.Fatal("AuthKey mismatch")
		}
		if string(decoded.PreSharedKey) != string(original.PreSharedKey) {
			t.Fatal("PreSharedKey mismatch")
		}
		if decoded.UserID != original.UserID {
			t.Fatal("UserID mismatch")
		}
	})
}

// Property 2: 无效 Token 统一拒绝
func TestProperty_InvalidTokenRejected(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		key := genKey()
		garbage := rapid.SliceOfN(rapid.Byte(), 0, 1024).Draw(t, "garbage")
		_, err := ParseToken(string(garbage), key)
		if err == nil {
			t.Fatal("expected error for random bytes")
		}
		if err.Error() != "Invalid token" {
			t.Fatalf("expected 'Invalid token', got %q", err.Error())
		}
	})
}

// 过期 Token 单元测试
func TestExpiredToken(t *testing.T) {
	key := genKey()
	config := &BootstrapConfig{
		BootstrapPool: []GatewayEndpoint{
			{IP: "1.2.3.4", Port: 443, Region: "us-east-1"},
		},
		AuthKey:         []byte("auth-key-data-here"),
		PreSharedKey:    []byte("psk-data-here-1234"),
		CertFingerprint: "abc123",
		UserID:          "user-001",
		ExpiresAt:       time.Now().Add(-time.Hour), // expired
	}

	encoded, err := TokenToBase64(config, key)
	if err != nil {
		t.Fatal(err)
	}

	decoded, err := ParseToken(encoded, key)
	if err != nil {
		t.Fatal(err)
	}

	if decoded.ExpiresAt.After(time.Now()) {
		t.Fatal("expected expired token")
	}
}
