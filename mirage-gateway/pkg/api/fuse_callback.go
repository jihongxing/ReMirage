// Package api - 配额熔断回调
// 当 QuotaBucketManager 检测到用户配额耗尽时：
// 1. 仅断开该用户的连接（不影响其他用户）
// 2. 通过 ReportSessionEvent 上报 FUSE_TRIGGERED 事件到 OS
// 3. OS 收到后写入 BillingLog（log_type = fuse）
//
// 解除熔断流程：
// 用户购买新流量包 → OS PurchaseQuota → SyncAfterPurchase → QuotaPush 下发新配额
// → Gateway PushQuota handler → QuotaBucketManager.UpdateQuota 重置 Exhausted 标记
// → 用户重新连接后正常消费
package api

import (
	"context"
	"log"
	"mirage-gateway/pkg/redact"
	pb "mirage-proto/gen"
	"time"
)

// FuseCallback 配额熔断回调器
// 连接 QuotaBucketManager（熔断检测）与 GRPCClient（事件上报）+ SessionManager（连接断开）
type FuseCallback struct {
	grpcClient *GRPCClient
	sessMgr    *SessionManager
	gatewayID  string
}

// NewFuseCallback 创建熔断回调器
func NewFuseCallback(client *GRPCClient, sessMgr *SessionManager, gatewayID string) *FuseCallback {
	return &FuseCallback{
		grpcClient: client,
		sessMgr:    sessMgr,
		gatewayID:  gatewayID,
	}
}

// Register 将熔断回调注册到 QuotaBucketManager
func (fc *FuseCallback) Register(qbm *QuotaBucketManager) {
	qbm.SetOnExhausted(fc.onUserExhausted)
}

// onUserExhausted 用户配额耗尽回调
// 1. 断开该用户所有连接
// 2. 上报 FUSE_TRIGGERED 事件到 OS
func (fc *FuseCallback) onUserExhausted(userID string) {
	log.Printf("[FuseCallback] 用户 %s 配额耗尽，执行熔断", redact.RedactToken(userID))

	// 1. 断开该用户的所有会话（仅影响该用户，不影响其他用户）
	if fc.sessMgr != nil {
		disconnected := fc.sessMgr.DisconnectUser(userID)
		log.Printf("[FuseCallback] 已断开用户 %s 的 %d 个会话", redact.RedactToken(userID), disconnected)
	}

	// 2. 上报熔断事件到 OS
	if fc.grpcClient != nil {
		req := &pb.SessionEventRequest{
			GatewayId: fc.gatewayID,
			SessionId: "fuse_" + userID, // 熔断事件使用特殊 session_id 前缀
			UserId:    userID,
			EventType: pb.SessionEventType_SESSION_FUSE_TRIGGERED,
			Timestamp: time.Now().Unix(),
		}
		if err := fc.grpcClient.ReportSessionEvent(context.Background(), req); err != nil {
			log.Printf("[FuseCallback] 熔断事件上报失败 (user=%s): %v", redact.RedactToken(userID), err)
		} else {
			log.Printf("[FuseCallback] 熔断事件已上报 OS (user=%s)", redact.RedactToken(userID))
		}
	}
}
