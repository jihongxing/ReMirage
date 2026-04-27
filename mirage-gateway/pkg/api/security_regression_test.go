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
	"pgregory.net/rapid"
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

// =============================================================================
// Feature: phase3-operational-baseline, Property 5: HMAC 校验确定性
// 验证: 需求 5.1
// =============================================================================

func TestProperty_HMACDeterminism(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		secret := rapid.StringMatching(`[a-zA-Z0-9]{8,32}`).Draw(t, "secret")
		commandType := rapid.StringMatching(`[A-Za-z]{4,16}`).Draw(t, "commandType")
		ts := strconv.FormatInt(time.Now().Unix(), 10)
		nonce := rapid.StringMatching(`[a-zA-Z0-9]{8,32}`).Draw(t, "nonce")
		payloadHash := rapid.StringMatching(`[a-f0-9]{16,64}`).Draw(t, "payloadHash")

		// 构造合法签名
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write([]byte(commandType + ts + nonce + payloadHash))
		sig := hex.EncodeToString(mac.Sum(nil))

		md := metadata.New(map[string]string{
			"x-mirage-sig":          sig,
			"x-mirage-ts":           ts,
			"x-mirage-nonce":        nonce,
			"x-mirage-payload-hash": payloadHash,
		})

		// 每次调用使用新的 authenticator 避免 nonce 缓存干扰
		// 调用 1: 应通过
		auth1 := NewCommandAuthenticator(secret)
		ctx1 := metadata.NewIncomingContext(context.Background(), md)
		err1 := auth1.Verify(ctx1, commandType)

		// 调用 2: 相同输入，新 authenticator，应产生一致结果
		auth2 := NewCommandAuthenticator(secret)
		ctx2 := metadata.NewIncomingContext(context.Background(), md)
		err2 := auth2.Verify(ctx2, commandType)

		if (err1 == nil) != (err2 == nil) {
			t.Fatalf("相同输入应产生一致结果: err1=%v, err2=%v", err1, err2)
		}

		// 修改 commandType → 签名不匹配
		altCmd := commandType + "X"
		auth3 := NewCommandAuthenticator(secret)
		ctx3 := metadata.NewIncomingContext(context.Background(), md)
		err3 := auth3.Verify(ctx3, altCmd)
		if err3 == nil && err1 == nil {
			t.Fatal("修改 commandType 后签名应不匹配")
		}

		// 修改 nonce → 签名不匹配
		altNonce := nonce + "X"
		mdAltNonce := metadata.New(map[string]string{
			"x-mirage-sig":          sig,
			"x-mirage-ts":           ts,
			"x-mirage-nonce":        altNonce,
			"x-mirage-payload-hash": payloadHash,
		})
		auth4 := NewCommandAuthenticator(secret)
		ctx4 := metadata.NewIncomingContext(context.Background(), mdAltNonce)
		err4 := auth4.Verify(ctx4, commandType)
		if err4 == nil && err1 == nil {
			t.Fatal("修改 nonce 后签名应不匹配")
		}

		// 修改 payloadHash → 签名不匹配
		altHash := payloadHash + "ff"
		mdAltHash := metadata.New(map[string]string{
			"x-mirage-sig":          sig,
			"x-mirage-ts":           ts,
			"x-mirage-nonce":        nonce,
			"x-mirage-payload-hash": altHash,
		})
		auth5 := NewCommandAuthenticator(secret)
		ctx5 := metadata.NewIncomingContext(context.Background(), mdAltHash)
		err5 := auth5.Verify(ctx5, commandType)
		if err5 == nil && err1 == nil {
			t.Fatal("修改 payloadHash 后签名应不匹配")
		}
	})
}
