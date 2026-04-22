package api

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"mirage-gateway/pkg/threat"
	"strconv"
	"sync"
	"time"

	"google.golang.org/grpc/metadata"
)

// CommandAuthenticator HMAC 签名校验器（扩展版）
// HMAC 覆盖范围：commandType + timestamp + nonce + SHA256(payload)
type CommandAuthenticator struct {
	secret     []byte
	nonceCache *nonceCache
	nonceStore *threat.NonceStore // L2 NonceStore（可选，用于跨组件 nonce 去重）
}

// nonceCache LRU nonce 重放缓存，TTL 120s
type nonceCache struct {
	mu      sync.Mutex
	entries map[string]time.Time
	ttl     time.Duration
}

func newNonceCache(ttl time.Duration) *nonceCache {
	nc := &nonceCache{
		entries: make(map[string]time.Time),
		ttl:     ttl,
	}
	go nc.cleanupLoop()
	return nc
}

func (nc *nonceCache) Check(nonce string) bool {
	nc.mu.Lock()
	defer nc.mu.Unlock()
	if _, exists := nc.entries[nonce]; exists {
		return true // 已使用
	}
	nc.entries[nonce] = time.Now()
	return false
}

func (nc *nonceCache) cleanupLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		nc.mu.Lock()
		now := time.Now()
		for k, v := range nc.entries {
			if now.Sub(v) > nc.ttl {
				delete(nc.entries, k)
			}
		}
		nc.mu.Unlock()
	}
}

// NewCommandAuthenticator 创建签名校验器
func NewCommandAuthenticator(secret string) *CommandAuthenticator {
	return &CommandAuthenticator{
		secret:     []byte(secret),
		nonceCache: newNonceCache(120 * time.Second),
	}
}

// SetNonceStore 注入 L2 NonceStore（用于跨组件 nonce 去重）
func (ca *CommandAuthenticator) SetNonceStore(ns *threat.NonceStore) {
	ca.nonceStore = ns
}

// Verify 从 gRPC metadata 中提取签名并校验
// metadata key: "x-mirage-sig" = HMAC-SHA256(secret, commandType + timestamp + nonce + SHA256(payload)) hex
// metadata key: "x-mirage-ts" = Unix timestamp string
// metadata key: "x-mirage-nonce" = 唯一 nonce
// metadata key: "x-mirage-payload-hash" = SHA256(payload) hex
func (ca *CommandAuthenticator) Verify(ctx context.Context, commandType string) error {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		threat.AuthFailureTotal.WithLabelValues(threat.GetGatewayID()).Inc()
		return fmt.Errorf("missing gRPC metadata")
	}

	sig := md.Get("x-mirage-sig")
	ts := md.Get("x-mirage-ts")
	nonce := md.Get("x-mirage-nonce")
	payloadHash := md.Get("x-mirage-payload-hash")
	if len(sig) == 0 || len(ts) == 0 {
		threat.AuthFailureTotal.WithLabelValues(threat.GetGatewayID()).Inc()
		return fmt.Errorf("missing signature or timestamp in metadata")
	}

	// 校验时间窗口（±60秒）
	tsVal, err := strconv.ParseInt(ts[0], 10, 64)
	if err != nil {
		threat.AuthFailureTotal.WithLabelValues(threat.GetGatewayID()).Inc()
		return fmt.Errorf("invalid timestamp format")
	}
	diff := math.Abs(float64(time.Now().Unix() - tsVal))
	if diff > 60 {
		threat.AuthFailureTotal.WithLabelValues(threat.GetGatewayID()).Inc()
		return fmt.Errorf("timestamp expired: drift=%.0fs", diff)
	}

	// Nonce 防重放校验（nonce 为必填字段）
	if len(nonce) == 0 {
		threat.AuthFailureTotal.WithLabelValues(threat.GetGatewayID()).Inc()
		return fmt.Errorf("missing nonce in metadata (x-mirage-nonce required)")
	}
	nonceStr := nonce[0]
	if ca.nonceCache.Check(nonceStr) {
		threat.AuthFailureTotal.WithLabelValues(threat.GetGatewayID()).Inc()
		return fmt.Errorf("nonce replay detected: %s", nonceStr)
	}
	// L2 NonceStore 跨组件去重
	if ca.nonceStore != nil {
		isDup, origIP := ca.nonceStore.CheckAndStore([]byte(nonceStr), "", time.Now())
		if isDup {
			threat.AuthFailureTotal.WithLabelValues(threat.GetGatewayID()).Inc()
			return fmt.Errorf("nonce replay detected (L2 store, original_ip=%s): %s", origIP, nonceStr)
		}
	}

	// 提取 payload hash（高危命令强制要求）
	phStr := ""
	if len(payloadHash) > 0 {
		phStr = payloadHash[0]
	}

	// 高危命令强制 payload-hash
	if isHighRiskCommand(commandType) && phStr == "" {
		threat.AuthFailureTotal.WithLabelValues(threat.GetGatewayID()).Inc()
		return fmt.Errorf("high-risk command %s requires x-mirage-payload-hash", commandType)
	}

	// 计算 HMAC-SHA256(secret, commandType + timestamp + nonce + SHA256(payload))
	mac := hmac.New(sha256.New, ca.secret)
	mac.Write([]byte(commandType + ts[0] + nonceStr + phStr))
	expected := hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(sig[0]), []byte(expected)) {
		threat.AuthFailureTotal.WithLabelValues(threat.GetGatewayID()).Inc()
		return fmt.Errorf("signature mismatch")
	}

	return nil
}

// highRiskCommands 高危命令集合（必须携带完整 nonce + payload-hash）
var highRiskCommands = map[string]bool{
	"PushStrategy":      true,
	"PushReincarnation": true,
	"PushQuota":         true,
}

// isHighRiskCommand 检查命令是否为高危命令
func isHighRiskCommand(commandType string) bool {
	return highRiskCommands[commandType]
}
