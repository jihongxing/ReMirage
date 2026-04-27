// Package billing - 配额熔断事件处理器
//
// 熔断流程：
//
//	Gateway QuotaBucketManager.Consume 返回 false
//	→ FuseCallback.onUserExhausted 断开用户连接 + 上报 FUSE_TRIGGERED
//	→ OS gateway-bridge ReportSessionEvent 写入 BillingLog(log_type=fuse)
//
// 解除熔断流程：
//
//	用户购买新流量包 → PurchaseQuota 事务 commit
//	→ QuotaBridge.SyncAfterPurchase 同步 Redis + QuotaManager
//	→ QuotaDispatcher.PushQuotaForUser 下发 QuotaPush 到 Gateway
//	→ Gateway PushQuota handler → QuotaBucketManager.UpdateQuota 重置 Exhausted=0
//	→ 用户重新连接后正常消费
package billing

import (
	"log"
	"mirage-os/pkg/models"
	"mirage-os/pkg/redact"

	"gorm.io/gorm"
)

// FuseEvent 熔断事件
type FuseEvent struct {
	UserID    string `json:"user_id"`
	GatewayID string `json:"gateway_id"`
	Reason    string `json:"reason"`
}

// FuseHandler 熔断事件处理器
type FuseHandler struct {
	db *gorm.DB
}

// NewFuseHandler 创建熔断事件处理器
func NewFuseHandler(db *gorm.DB) *FuseHandler {
	return &FuseHandler{db: db}
}

// HandleFuseEvent 处理熔断事件：写入 BillingLog
func (h *FuseHandler) HandleFuseEvent(event *FuseEvent) error {
	billingLog := models.BillingLog{
		UserID:    event.UserID,
		GatewayID: event.GatewayID,
		LogType:   "fuse",
	}
	if err := h.db.Create(&billingLog).Error; err != nil {
		log.Printf("[FuseHandler] 写入熔断日志失败: %v", err)
		return err
	}
	log.Printf("[FuseHandler] 用户 %s 配额熔断已记录 (gateway: %s)", redact.Token(event.UserID), event.GatewayID)
	return nil
}
