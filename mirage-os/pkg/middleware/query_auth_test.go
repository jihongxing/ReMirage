package middleware

import (
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestQueryAuth_NoCredentials 无认证凭据应返回 401
func TestQueryAuth_NoCredentials(t *testing.T) {
	m := NewQueryAuthMiddleware(nil)
	handler := m.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/v2/entitlement", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

// TestQueryAuth_AllowListBypass 免认证路径应放行
func TestQueryAuth_AllowListBypass(t *testing.T) {
	m := NewQueryAuthMiddleware(nil)
	m.AllowPath("/healthz")
	handler := m.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/healthz", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

// TestQueryAuth_InvalidHMAC 无效 HMAC 签名应返回 401
func TestQueryAuth_InvalidHMAC(t *testing.T) {
	m := NewQueryAuthMiddleware(nil)
	m.hmacKey = []byte("test-secret-key-1234567890abcdef")
	handler := m.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/v2/entitlement", nil)
	req.Header.Set("X-HMAC-Signature", "invalid-sig")
	req.Header.Set("X-HMAC-Timestamp", time.Now().Format(time.RFC3339))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

// TestQueryAuth_ValidHMAC 有效 HMAC 签名应放行
func TestQueryAuth_ValidHMAC(t *testing.T) {
	key := []byte("test-secret-key-1234567890abcdef")
	m := NewQueryAuthMiddleware(nil)
	m.hmacKey = key
	handler := m.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	ts := time.Now().Format(time.RFC3339)
	path := "/api/v2/entitlement"
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(ts + path))
	sig := hex.EncodeToString(mac.Sum(nil))

	req := httptest.NewRequest("GET", path, nil)
	req.Header.Set("X-HMAC-Signature", sig)
	req.Header.Set("X-HMAC-Timestamp", ts)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

// TestQueryAuth_ExpiredHMACTimestamp 过期时间戳应返回 401
func TestQueryAuth_ExpiredHMACTimestamp(t *testing.T) {
	key := []byte("test-secret-key-1234567890abcdef")
	m := NewQueryAuthMiddleware(nil)
	m.hmacKey = key
	handler := m.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	ts := time.Now().Add(-10 * time.Minute).Format(time.RFC3339)
	path := "/api/v2/entitlement"
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(ts + path))
	sig := hex.EncodeToString(mac.Sum(nil))

	req := httptest.NewRequest("GET", path, nil)
	req.Header.Set("X-HMAC-Signature", sig)
	req.Header.Set("X-HMAC-Timestamp", ts)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

// TestQueryAuth_RawXClientIDRejected 裸 X-Client-ID 不再被信任
func TestQueryAuth_RawXClientIDRejected(t *testing.T) {
	m := NewQueryAuthMiddleware(nil)
	handler := m.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/v2/entitlement", nil)
	req.Header.Set("X-Client-ID", "user-123")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for raw X-Client-ID, got %d", w.Code)
	}
}

// TestQueryAuth_SignatureWithoutTimestamp 缺少时间戳应返回 400
func TestQueryAuth_SignatureWithoutTimestamp(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(nil)
	_ = priv

	m := NewQueryAuthMiddleware(nil)
	handler := m.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/v2/entitlement", nil)
	req.Header.Set("X-Client-ID", "user-123")
	req.Header.Set("X-Signature", "deadbeef")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}
