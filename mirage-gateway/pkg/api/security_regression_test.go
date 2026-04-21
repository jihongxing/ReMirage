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
		"x-mirage-sig": "invalid-signature-value",
		"x-mirage-ts":  ts,
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

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(cmdType + ts))
	sig := hex.EncodeToString(mac.Sum(nil))

	md := metadata.New(map[string]string{
		"x-mirage-sig": sig,
		"x-mirage-ts":  ts,
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
		t.Fatal("过期时间戳应被拒绝")
	}
}
