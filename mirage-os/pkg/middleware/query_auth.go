// Package middleware - Query Surface 认证中间件
package middleware

import (
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"os"
	"strings"
	"time"

	"mirage-os/pkg/models"

	"gorm.io/gorm"
)

// QueryAuthMiddleware 为 V2 Query Surface 提供真实认证
// 支持两种认证方式：
//  1. HMAC 签名认证（Gateway → OS 内部调用）
//  2. Ed25519 签名认证（Client → OS 用户级调用）
//
// 不再信任裸 X-Client-ID header
type QueryAuthMiddleware struct {
	db        *gorm.DB
	hmacKey   []byte
	allowList map[string]bool // 无需认证的路径（如 /healthz）
}

// NewQueryAuthMiddleware 创建认证中间件
func NewQueryAuthMiddleware(db *gorm.DB) *QueryAuthMiddleware {
	m := &QueryAuthMiddleware{
		db:        db,
		allowList: make(map[string]bool),
	}
	if key := os.Getenv("QUERY_HMAC_SECRET"); key != "" {
		m.hmacKey, _ = hex.DecodeString(key)
	}
	return m
}

// AllowPath 添加免认证路径
func (m *QueryAuthMiddleware) AllowPath(path string) {
	m.allowList[path] = true
}

// Wrap 包装 http.Handler，注入认证检查
func (m *QueryAuthMiddleware) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 免认证路径
		if m.allowList[r.URL.Path] {
			next.ServeHTTP(w, r)
			return
		}

		// 尝试 HMAC 认证（内部 Gateway 调用）
		if sig := r.Header.Get("X-HMAC-Signature"); sig != "" {
			if m.verifyHMAC(r) {
				next.ServeHTTP(w, r)
				return
			}
			http.Error(w, `{"error":"HMAC signature invalid"}`, http.StatusUnauthorized)
			return
		}

		// 尝试 Ed25519 签名认证（用户级调用）
		if sig := r.Header.Get("X-Signature"); sig != "" {
			clientID := r.Header.Get("X-Client-ID")
			timestamp := r.Header.Get("X-Timestamp")
			if clientID == "" || timestamp == "" {
				http.Error(w, `{"error":"X-Client-ID and X-Timestamp required with X-Signature"}`, http.StatusBadRequest)
				return
			}
			if m.verifyClientSignature(clientID, timestamp, sig) {
				next.ServeHTTP(w, r)
				return
			}
			http.Error(w, `{"error":"signature verification failed"}`, http.StatusUnauthorized)
			return
		}

		// 无认证凭据
		http.Error(w, `{"error":"authentication required"}`, http.StatusUnauthorized)
	})
}

// verifyHMAC 验证 HMAC 签名（Gateway 内部调用）
func (m *QueryAuthMiddleware) verifyHMAC(r *http.Request) bool {
	if len(m.hmacKey) == 0 {
		return false
	}
	sig := r.Header.Get("X-HMAC-Signature")
	ts := r.Header.Get("X-HMAC-Timestamp")
	if sig == "" || ts == "" {
		return false
	}

	// 检查时间戳（5 分钟窗口）
	tsTime, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return false
	}
	if time.Since(tsTime).Abs() > 5*time.Minute {
		return false
	}

	// 计算 HMAC: SHA256(timestamp + path)
	mac := hmac.New(sha256.New, m.hmacKey)
	mac.Write([]byte(ts + r.URL.Path))
	expected := hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(sig), []byte(expected))
}

// verifyClientSignature 验证 Ed25519 客户端签名
func (m *QueryAuthMiddleware) verifyClientSignature(clientID, timestamp, signature string) bool {
	if m.db == nil {
		return false
	}

	// 检查时间戳
	ts, err := time.Parse(time.RFC3339, timestamp)
	if err != nil {
		return false
	}
	if time.Since(ts).Abs() > 5*time.Minute {
		return false
	}

	// 查找用户公钥
	var user models.User
	if err := m.db.Where("user_id = ? AND status = ?", clientID, "active").
		Select("hardware_public_key").First(&user).Error; err != nil {
		return false
	}
	if user.HardwarePublicKey == "" {
		return false
	}

	pubKeyBytes, err := hex.DecodeString(user.HardwarePublicKey)
	if err != nil || len(pubKeyBytes) != ed25519.PublicKeySize {
		return false
	}

	sigBytes, err := hex.DecodeString(strings.TrimPrefix(signature, "0x"))
	if err != nil {
		return false
	}

	// 验证签名: sign(clientID + timestamp)
	message := []byte(clientID + timestamp)
	return ed25519.Verify(ed25519.PublicKey(pubKeyBytes), message, sigBytes)
}
