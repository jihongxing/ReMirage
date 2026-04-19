// Package billing - Monero 匿名充值管理
package billing

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"mirage-os/pkg/models"

	"github.com/go-redis/redis/v8"
	"gorm.io/gorm"
)

const (
	// MinConfirmations 最小确认数
	MinConfirmations = 10
	
	// MoneroRPCEndpoint Monero 节点 RPC 地址
	MoneroRPCEndpoint = "http://localhost:18081/json_rpc"
	
	// PollInterval 轮询间隔
	PollInterval = 30 * time.Second
)

// MoneroManager Monero 充值管理器
type MoneroManager struct {
	DB          *gorm.DB
	Redis       *redis.Client
	RPCEndpoint string
}

// NewMoneroManager 创建 Monero 管理器
func NewMoneroManager(db *gorm.DB, rdb *redis.Client, rpcEndpoint string) *MoneroManager {
	if rpcEndpoint == "" {
		rpcEndpoint = MoneroRPCEndpoint
	}
	
	return &MoneroManager{
		DB:          db,
		Redis:       rdb,
		RPCEndpoint: rpcEndpoint,
	}
}

// GenerateDepositAddress 生成充值地址（集成地址）
func (mm *MoneroManager) GenerateDepositAddress(userID string) (string, error) {
	// 1. 检查用户是否已有地址
	var user models.User
	if err := mm.DB.Where("user_id = ?", userID).First(&user).Error; err != nil {
		return "", fmt.Errorf("用户不存在: %w", err)
	}

	if user.XMRAddress != "" {
		return user.XMRAddress, nil
	}

	// 2. 调用 Monero RPC 生成集成地址
	// 注意：这里需要实际的 Monero RPC 客户端
	// 示例使用固定格式，实际应调用 make_integrated_address
	integratedAddress := fmt.Sprintf("4%s", userID[:90]) // 简化示例

	// 3. 保存到数据库
	if err := mm.DB.Model(&user).Update("xmr_address", integratedAddress).Error; err != nil {
		return "", fmt.Errorf("保存地址失败: %w", err)
	}

	log.Printf("💰 [Monero] 为用户 %s 生成充值地址: %s", userID, integratedAddress)
	return integratedAddress, nil
}

// MonitorDeposits 监听充值（后台任务）
func (mm *MoneroManager) MonitorDeposits(ctx context.Context) {
	ticker := time.NewTicker(PollInterval)
	defer ticker.Stop()

	log.Println("💰 [Monero] 充值监听器已启动")

	for {
		select {
		case <-ctx.Done():
			log.Println("💰 [Monero] 充值监听器已停止")
			return
		case <-ticker.C:
			mm.checkPendingDeposits()
		}
	}
}

// checkPendingDeposits 检查待确认的充值
func (mm *MoneroManager) checkPendingDeposits() {
	var deposits []models.Deposit
	if err := mm.DB.Where("status = ?", "pending").Find(&deposits).Error; err != nil {
		log.Printf("❌ [Monero] 查询待确认充值失败: %v", err)
		return
	}

	for _, deposit := range deposits {
		mm.processDeposit(&deposit)
	}
}

// processDeposit 处理单个充值
func (mm *MoneroManager) processDeposit(deposit *models.Deposit) {
	// 1. 查询交易确认数（调用 Monero RPC）
	confirmations, err := mm.getTransactionConfirmations(deposit.TxHash)
	if err != nil {
		log.Printf("❌ [Monero] 查询交易 %s 失败: %v", deposit.TxHash, err)
		return
	}

	// 2. 更新确认数
	mm.DB.Model(deposit).Update("confirmations", confirmations)

	// 3. 推送实时进度到用户
	mm.publishDepositProgress(deposit.UserID, deposit.TxHash, confirmations)

	// 4. 达到最小确认数，确认充值
	if confirmations >= MinConfirmations && deposit.Status == "pending" {
		mm.confirmDeposit(deposit)
	}
}

// confirmDeposit 确认充值
func (mm *MoneroManager) confirmDeposit(deposit *models.Deposit) {
	log.Printf("💰 [Monero] 确认充值: 用户=%s, 金额=%.8f XMR", deposit.UserID, deposit.AmountXMR)

	err := mm.DB.Transaction(func(tx *gorm.DB) error {
		// 1. 更新充值状态
		now := time.Now()
		if err := tx.Model(deposit).Updates(map[string]interface{}{
			"status":       "confirmed",
			"confirmed_at": &now,
		}).Error; err != nil {
			return fmt.Errorf("更新充值状态失败: %w", err)
		}

		// 2. 增加用户余额（使用 numeric 精度）
		if err := tx.Model(&models.User{}).
			Where("user_id = ?", deposit.UserID).
			Updates(map[string]interface{}{
				"balance":     gorm.Expr("balance + ?", deposit.AmountXMR),
				"balance_usd": gorm.Expr("balance_usd + ?", deposit.AmountUSD),
			}).Error; err != nil {
			return fmt.Errorf("更新用户余额失败: %w", err)
		}

		// 3. 记录计费流水
		billingLog := models.BillingLog{
			UserID:   deposit.UserID,
			CostUSD:  deposit.AmountUSD,
			LogType:  "deposit",
		}
		if err := tx.Create(&billingLog).Error; err != nil {
			return fmt.Errorf("记录计费流水失败: %w", err)
		}

		return nil
	})

	if err != nil {
		log.Printf("❌ [Monero] 确认充值失败: %v", err)
		return
	}

	// 4. 推送到账通知
	mm.publishDepositConfirmed(deposit.UserID, deposit.AmountXMR, deposit.AmountUSD)
}

// getTransactionConfirmations 获取交易确认数（调用 Monero RPC）
func (mm *MoneroManager) getTransactionConfirmations(txHash string) (int, error) {
	// TODO: 实际调用 Monero RPC
	// 示例返回模拟值
	return 10, nil
}

// publishDepositProgress 推送充值进度
func (mm *MoneroManager) publishDepositProgress(userID, txHash string, confirmations int) {
	ctx := context.Background()
	channel := fmt.Sprintf("mirage:user:%s:events", userID)

	event := map[string]interface{}{
		"type":      "deposit_progress",
		"timestamp": time.Now().Unix(),
		"data": map[string]interface{}{
			"txHash":        txHash,
			"confirmations": confirmations,
			"required":      MinConfirmations,
			"message":       fmt.Sprintf("检测到转账，等待 %d/%d 个确认块...", confirmations, MinConfirmations),
		},
	}

	payload, _ := json.Marshal(event)
	mm.Redis.Publish(ctx, channel, payload)
}

// publishDepositConfirmed 推送到账通知
func (mm *MoneroManager) publishDepositConfirmed(userID string, amountXMR, amountUSD float64) {
	ctx := context.Background()
	channel := fmt.Sprintf("mirage:user:%s:events", userID)

	event := map[string]interface{}{
		"type":      "deposit_confirmed",
		"timestamp": time.Now().Unix(),
		"data": map[string]interface{}{
			"amountXMR": amountXMR,
			"amountUSD": amountUSD,
			"message":   fmt.Sprintf("充值成功：%.8f XMR ($%.2f)", amountXMR, amountUSD),
		},
	}

	payload, _ := json.Marshal(event)
	mm.Redis.Publish(ctx, channel, payload)
	
	log.Printf("💰 [Monero] 已推送到账通知: 用户=%s, 金额=%.8f XMR", userID, amountXMR)
}

// CreateDeposit 创建充值记录
func (mm *MoneroManager) CreateDeposit(userID, txHash string, amountXMR, exchangeRate float64) error {
	amountUSD := amountXMR * exchangeRate

	deposit := &models.Deposit{
		UserID:        userID,
		TxHash:        txHash,
		AmountXMR:     amountXMR,
		AmountUSD:     amountUSD,
		ExchangeRate:  exchangeRate,
		Status:        "pending",
		Confirmations: 0,
	}

	if err := mm.DB.Create(deposit).Error; err != nil {
		return fmt.Errorf("创建充值记录失败: %w", err)
	}

	log.Printf("💰 [Monero] 创建充值记录: 用户=%s, TxHash=%s, 金额=%.8f XMR", userID, txHash, amountXMR)
	return nil
}

// GetExchangeRate 获取 XMR/USD 汇率
func (mm *MoneroManager) GetExchangeRate() (float64, error) {
	// TODO: 调用外部 API 获取实时汇率
	// 示例返回固定值
	return 150.0, nil
}
