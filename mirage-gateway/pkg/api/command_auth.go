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
	"time"

	"google.golang.org/grpc/metadata"
)

// CommandAuthenticator HMAC 签名校验器
type CommandAuthenticator struct {
	secret []byte
}

// NewCommandAuthenticator 创建签名校验器
func NewCommandAuthenticator(secret string) *CommandAuthenticator {
	return &CommandAuthenticator{secret: []byte(secret)}
}

// Verify 从 gRPC metadata 中提取签名并校验
// metadata key: "x-mirage-sig" = HMAC-SHA256(secret, commandType + timestamp) hex
// metadata key: "x-mirage-ts" = Unix timestamp string
func (ca *CommandAuthenticator) Verify(ctx context.Context, commandType string) error {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		threat.AuthFailureTotal.WithLabelValues(threat.GetGatewayID()).Inc()
		return fmt.Errorf("missing gRPC metadata")
	}

	sig := md.Get("x-mirage-sig")
	ts := md.Get("x-mirage-ts")
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

	// 计算 HMAC-SHA256
	mac := hmac.New(sha256.New, ca.secret)
	mac.Write([]byte(commandType + ts[0]))
	expected := hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(sig[0]), []byte(expected)) {
		threat.AuthFailureTotal.WithLabelValues(threat.GetGatewayID()).Inc()
		return fmt.Errorf("signature mismatch")
	}

	return nil
}
