package api

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"testing"
	"time"

	"google.golang.org/grpc/metadata"
)

// ============================================================
// 安全回归测试 — Gateway 侧 mTLS/HMAC 校验
// ============================================================

func TestSecurityRegression_mTLS_RejectWithoutSignature(t *testing.T) {
	auth := NewCommandAuthenticator("test-secret-key")

	// 无 metadata 的 context → 应拒绝
	ctx := context.Background()
	err := auth.Verify(ctx, "PushStrategy")
	if err == nil {
		t.Fatal("无 metadata 时应拒绝请求")
	}
}

func TestSecurityRegression_mTLS_RejectInvalidHMAC(t *testing.T) {
	auth := NewCommandAuthenticator("test-secret-key")

	ts := strconv.FormatInt(time.Now().Unix(), 10)
	md := metadata.New(map[string]string{
		"x-mirage-sig":   "invalid-signature-value",
		"x-mirage-ts":    ts,
		"x-mirage-nonce": "test-nonce-001",
	})
	ctx := metadata.NewIncomingContext(context.Background(), md)

	err := auth.Verify(ctx, "PushStrategy")
	if err == nil {
		t.Fatal("无效 HMAC 签名时应拒绝请求")
	}
}

func TestSecurityRegression_mTLS_AcceptValidHMAC(t *testing.T) {
	secret := "test-secret-key"
	auth := NewCommandAuthenticator(secret)

	cmdType := "PushStrategy"
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	nonce := "unique-nonce-001"
	payloadHash := "abc123def456"

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(cmdType + ts + nonce + payloadHash))
	sig := hex.EncodeToString(mac.Sum(nil))

	md := metadata.New(map[string]string{
		"x-mirage-sig":          sig,
		"x-mirage-ts":           ts,
		"x-mirage-nonce":        nonce,
		"x-mirage-payload-hash": payloadHash,
	})
	ctx := metadata.NewIncomingContext(context.Background(), md)

	err := auth.Verify(ctx, cmdType)
	if err != nil {
		t.Fatalf("有效 HMAC 签名应通过校验，错误: %v", err)
	}
}

func TestSecurityRegression_mTLS_RejectExpiredTimestamp(t *testing.T) {
	secret := "test-secret-key"
	auth := NewCommandAuthenticator(secret)

	cmdType := "PushStrategy"
	// 120 秒前的时间戳（超过 60 秒窗口）
	ts := strconv.FormatInt(time.Now().Unix()-120, 10)
	nonce := "unique-nonce-002"
	payloadHash := "abc123"

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(cmdType + ts + nonce + payloadHash))
	sig := hex.EncodeToString(mac.Sum(nil))

	md := metadata.New(map[string]string{
		"x-mirage-sig":          sig,
		"x-mirage-ts":           ts,
		"x-mirage-nonce":        nonce,
		"x-mirage-payload-hash": payloadHash,
	})
	ctx := metadata.NewIncomingContext(context.Background(), md)

	err := auth.Verify(ctx, cmdType)
	if err == nil {
		t.Fatal("过期时间戳应被拒绝")
	}
}

func TestSecurityRegression_RejectMissingNonce(t *testing.T) {
	secret := "test-secret-key"
	auth := NewCommandAuthenticator(secret)

	cmdType := "PushStrategy"
	ts := strconv.FormatInt(time.Now().Unix(), 10)

	// 不携带 nonce → 应拒绝
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(cmdType + ts))
	sig := hex.EncodeToString(mac.Sum(nil))

	md := metadata.New(map[string]string{
		"x-mirage-sig": sig,
		"x-mirage-ts":  ts,
	})
	ctx := metadata.NewIncomingContext(context.Background(), md)

	err := auth.Verify(ctx, cmdType)
	if err == nil {
		t.Fatal("缺少 nonce 时应拒绝请求")
	}
}

func TestSecurityRegression_RejectHighRiskWithoutPayloadHash(t *testing.T) {
	secret := "test-secret-key"
	auth := NewCommandAuthenticator(secret)

	cmdType := "PushStrategy" // 高危命令
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	nonce := "unique-nonce-003"

	// 不携带 payload-hash → 高危命令应拒绝
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(cmdType + ts + nonce))
	sig := hex.EncodeToString(mac.Sum(nil))

	md := metadata.New(map[string]string{
		"x-mirage-sig":   sig,
		"x-mirage-ts":    ts,
		"x-mirage-nonce": nonce,
	})
	ctx := metadata.NewIncomingContext(context.Background(), md)

	err := auth.Verify(ctx, cmdType)
	if err == nil {
		t.Fatal("高危命令缺少 payload-hash 时应拒绝请求")
	}
}

func TestSecurityRegression_NonceReplayDetected(t *testing.T) {
	secret := "test-secret-key"
	auth := NewCommandAuthenticator(secret)

	cmdType := "PushStrategy"
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	nonce := "replay-nonce-001"
	payloadHash := "hash123"

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(cmdType + ts + nonce + payloadHash))
	sig := hex.EncodeToString(mac.Sum(nil))

	md := metadata.New(map[string]string{
		"x-mirage-sig":          sig,
		"x-mirage-ts":           ts,
		"x-mirage-nonce":        nonce,
		"x-mirage-payload-hash": payloadHash,
	})

	// 第一次 → 通过
	ctx1 := metadata.NewIncomingContext(context.Background(), md)
	if err := auth.Verify(ctx1, cmdType); err != nil {
		t.Fatalf("首次请求应通过: %v", err)
	}

	// 第二次相同 nonce → 应拒绝（重放）
	ctx2 := metadata.NewIncomingContext(context.Background(), md)
	if err := auth.Verify(ctx2, cmdType); err == nil {
		t.Fatal("重放 nonce 应被拒绝")
	}
}
