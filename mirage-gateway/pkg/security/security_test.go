package security

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	"pgregory.net/rapid"
)

// Feature: gateway-closure, Property 1: Ed25519 签名往返一致性
func TestProperty_Ed25519RoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}

		sa := NewShadowAuth()
		challenge, err := sa.GenerateChallenge()
		if err != nil {
			t.Fatal(err)
		}

		sig := ed25519.Sign(priv, []byte(challenge.Raw))
		pubHex := hex.EncodeToString(pub)
		sigHex := hex.EncodeToString(sig)

		if err := sa.VerifySignature(pubHex, challenge.Raw, sigHex); err != nil {
			t.Fatalf("合法签名验证失败: %v", err)
		}
	})
}

// Feature: gateway-closure, Property 2: 挑战唯一性
func TestProperty_ChallengeUniqueness(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		n := rapid.IntRange(2, 50).Draw(t, "n")
		sa := NewShadowAuth()
		nonces := make(map[string]bool)

		for i := 0; i < n; i++ {
			ch, err := sa.GenerateChallenge()
			if err != nil {
				t.Fatal(err)
			}
			if nonces[ch.Nonce] {
				t.Fatalf("重复 nonce: %s", ch.Nonce)
			}
			nonces[ch.Nonce] = true
		}
	})
}

// Feature: gateway-closure, Property 3: 无效签名统一拒绝
func TestProperty_InvalidSignatureRejected(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		pub, _, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}

		sa := NewShadowAuth()
		challenge, err := sa.GenerateChallenge()
		if err != nil {
			t.Fatal(err)
		}

		// 随机篡改签名
		fakeSig := make([]byte, ed25519.SignatureSize)
		rand.Read(fakeSig)

		pubHex := hex.EncodeToString(pub)
		sigHex := hex.EncodeToString(fakeSig)

		err = sa.VerifySignature(pubHex, challenge.Raw, sigHex)
		if err == nil {
			t.Fatal("应拒绝无效签名")
		}
		// 错误信息不应包含具体失败原因
		if err.Error() != "认证失败" {
			t.Fatalf("错误信息不应泄露原因: %v", err)
		}
	})
}

// Feature: gateway-closure, Property 4: 过期挑战拒绝
func TestProperty_ExpiredChallengeRejected(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}

		sa := NewShadowAuth()

		// 构造过期挑战（时间戳 > 300 秒前）
		offset := rapid.IntRange(301, 600).Draw(t, "offset")
		nonce := make([]byte, 32)
		rand.Read(nonce)
		nonceHex := hex.EncodeToString(nonce)
		ts := time.Now().Add(-time.Duration(offset) * time.Second).Unix()
		raw := fmt.Sprintf("mirage-auth:%s:%d", nonceHex, ts)

		sig := ed25519.Sign(priv, []byte(raw))
		pubHex := hex.EncodeToString(pub)
		sigHex := hex.EncodeToString(sig)

		err = sa.VerifySignature(pubHex, raw, sigHex)
		if err == nil {
			t.Fatal("应拒绝过期挑战")
		}
	})
}

// 单元测试: TLS 配置加载 - enabled=false
func TestTLSManager_Disabled(t *testing.T) {
	cfg := TLSConfig{Enabled: false}
	tm, err := NewTLSManager(cfg)
	if err != nil {
		t.Fatalf("创建 TLSManager 失败: %v", err)
	}

	clientCfg, err := tm.GetClientTLSConfig()
	if err != nil {
		t.Fatalf("GetClientTLSConfig 失败: %v", err)
	}
	if clientCfg != nil {
		t.Fatal("禁用时应返回 nil")
	}

	serverCfg, err := tm.GetServerTLSConfig()
	if err != nil {
		t.Fatalf("GetServerTLSConfig 失败: %v", err)
	}
	if serverCfg != nil {
		t.Fatal("禁用时应返回 nil")
	}
}

// 单元测试: TLS 配置加载 - 无效路径
func TestTLSManager_InvalidPath(t *testing.T) {
	cfg := TLSConfig{
		Enabled:  true,
		CertFile: "/nonexistent/cert.pem",
		KeyFile:  "/nonexistent/key.pem",
		CAFile:   "/nonexistent/ca.pem",
	}
	_, err := NewTLSManager(cfg)
	if err == nil {
		t.Fatal("应返回错误")
	}
}
