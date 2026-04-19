package security

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// Challenge 挑战结构
type Challenge struct {
	Nonce     string `json:"nonce"`
	Timestamp int64  `json:"timestamp"`
	Raw       string `json:"raw"` // "mirage-auth:{nonce}:{timestamp}"
}

// ShadowAuth Ed25519 挑战-响应认证器
type ShadowAuth struct {
	mu                sync.Mutex
	pendingChallenges map[string]time.Time // nonce → 创建时间
	challengeTTL      time.Duration        // 300 秒
}

// NewShadowAuth 创建认证器
func NewShadowAuth() *ShadowAuth {
	return &ShadowAuth{
		pendingChallenges: make(map[string]time.Time),
		challengeTTL:      300 * time.Second,
	}
}

// GenerateChallenge 生成挑战字符串
func (sa *ShadowAuth) GenerateChallenge() (*Challenge, error) {
	nonceBytes := make([]byte, 32)
	if _, err := rand.Read(nonceBytes); err != nil {
		return nil, fmt.Errorf("生成随机数失败: %w", err)
	}

	nonce := hex.EncodeToString(nonceBytes)
	ts := time.Now().Unix()
	raw := fmt.Sprintf("mirage-auth:%s:%d", nonce, ts)

	sa.mu.Lock()
	sa.pendingChallenges[nonce] = time.Now()
	sa.mu.Unlock()

	return &Challenge{
		Nonce:     nonce,
		Timestamp: ts,
		Raw:       raw,
	}, nil
}

// VerifySignature 验证 Ed25519 签名
func (sa *ShadowAuth) VerifySignature(publicKeyHex, challenge, signatureHex string) error {
	pubKeyBytes, err := hex.DecodeString(publicKeyHex)
	if err != nil || len(pubKeyBytes) != ed25519.PublicKeySize {
		return fmt.Errorf("认证失败")
	}

	sigBytes, err := hex.DecodeString(signatureHex)
	if err != nil || len(sigBytes) != ed25519.SignatureSize {
		return fmt.Errorf("认证失败")
	}

	// 解析挑战中的时间戳检查过期
	var nonce string
	var ts int64
	if _, err := fmt.Sscanf(challenge, "mirage-auth:%64s:%d", &nonce, &ts); err == nil {
		if time.Since(time.Unix(ts, 0)) > sa.challengeTTL {
			return fmt.Errorf("挑战已过期")
		}
	}

	pubKey := ed25519.PublicKey(pubKeyBytes)
	if !ed25519.Verify(pubKey, []byte(challenge), sigBytes) {
		return fmt.Errorf("认证失败")
	}

	// 消费 nonce，防止重放
	sa.mu.Lock()
	delete(sa.pendingChallenges, nonce)
	sa.mu.Unlock()

	return nil
}

// CleanExpired 清理过期挑战
func (sa *ShadowAuth) CleanExpired() {
	sa.mu.Lock()
	defer sa.mu.Unlock()

	now := time.Now()
	for nonce, created := range sa.pendingChallenges {
		if now.Sub(created) > sa.challengeTTL {
			delete(sa.pendingChallenges, nonce)
		}
	}
}
