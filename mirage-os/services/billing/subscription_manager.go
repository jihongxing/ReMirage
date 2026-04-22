package billing

import (
	"context"
	"fmt"
	"log"
	"mirage-os/pkg/models"
	"sync"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// SubscriptionManager 订阅到期管理器
type SubscriptionManager struct {
	db       *gorm.DB
	billing  *BillingServiceImpl
	interval time.Duration
	stopCh   chan struct{}
	wg       sync.WaitGroup
}

// NewSubscriptionManager 创建订阅到期管理器
func NewSubscriptionManager(db *gorm.DB, billing *BillingServiceImpl) *SubscriptionManager {
	return &SubscriptionManager{
		db:       db,
		billing:  billing,
		interval: 1 * time.Hour,
		stopCh:   make(chan struct{}),
	}
}

// Start 启动定期检查
func (m *SubscriptionManager) Start(ctx context.Context) {
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		ticker := time.NewTicker(m.interval)
		defer ticker.Stop()
		log.Println("[SubscriptionManager] 订阅到期管理器已启动")
		for {
			select {
			case <-ctx.Done():
				return
			case <-m.stopCh:
				return
			case <-ticker.C:
				m.processExpiredSubscriptions()
			}
		}
	}()
}

// Stop 停止管理器
func (m *SubscriptionManager) Stop() {
	close(m.stopCh)
	m.wg.Wait()
	log.Println("[SubscriptionManager] 订阅到期管理器已停止")
}

// processExpiredSubscriptions 处理到期订阅
func (m *SubscriptionManager) processExpiredSubscriptions() {
	var users []models.User
	if err := m.db.Where(
		"subscription_expires_at IS NOT NULL AND subscription_expires_at < ? AND cell_level > 1",
		time.Now(),
	).Find(&users).Error; err != nil {
		log.Printf("[SubscriptionManager] 查询到期用户失败: %v", err)
		return
	}

	for _, user := range users {
		if user.AutoRenew {
			if err := m.tryAutoRenew(user); err == nil {
				log.Printf("[SubscriptionManager] 用户 %s 自动续费成功", user.UserID)
				continue
			}
		}
		m.downgradeToStandard(user)
	}
}

// tryAutoRenew 尝试自动续费
func (m *SubscriptionManager) tryAutoRenew(user models.User) error {
	planType := user.SubscriptionPackageType
	if planType == "" {
		return fmt.Errorf("no subscription package type")
	}
	priceUSD, ok := tierPrices[planType]
	if !ok {
		return fmt.Errorf("unknown plan type: %s", planType)
	}
	totalPrice := float64(priceUSD) / 100.0
	if user.BalanceUSD < totalPrice {
		return fmt.Errorf("insufficient balance for auto-renew")
	}

	return m.db.Transaction(func(tx *gorm.DB) error {
		// 重新加锁读取，防止并发
		var freshUser models.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_id = ?", user.UserID).
			First(&freshUser).Error; err != nil {
			return err
		}
		if freshUser.BalanceUSD < totalPrice {
			return fmt.Errorf("insufficient balance after re-check")
		}

		expiresAt := time.Now().Add(30 * 24 * time.Hour)
		if err := tx.Model(&freshUser).Updates(map[string]any{
			"balance_usd":             gorm.Expr("balance_usd - ?", totalPrice),
			"subscription_expires_at": expiresAt,
		}).Error; err != nil {
			return err
		}

		// 创建购买记录
		if err := tx.Create(&models.QuotaPurchase{
			UserID:      user.UserID,
			PackageType: planType,
			QuotaBytes:  0,
			CostUSD:     totalPrice,
			CellLevel:   freshUser.CellLevel,
			ExpiresAt:   &expiresAt,
		}).Error; err != nil {
			return err
		}

		// 计费流水
		if err := tx.Create(&models.BillingLog{
			UserID:  user.UserID,
			CostUSD: totalPrice,
			LogType: "subscription",
		}).Error; err != nil {
			return err
		}

		return nil
	})
}

// downgradeToStandard 降级到 Standard
func (m *SubscriptionManager) downgradeToStandard(user models.User) {
	if err := m.db.Model(&user).Updates(map[string]any{
		"cell_level":                1,
		"subscription_package_type": "",
		"subscription_expires_at":   nil,
	}).Error; err != nil {
		log.Printf("[SubscriptionManager] 用户 %s 降级失败: %v", user.UserID, err)
		return
	}

	if err := m.db.Create(&models.BillingLog{
		UserID:  user.UserID,
		LogType: "downgrade",
	}).Error; err != nil {
		log.Printf("[SubscriptionManager] 用户 %s 降级日志写入失败: %v", user.UserID, err)
	}

	log.Printf("[SubscriptionManager] 用户 %s 等级订阅到期，降级为 Standard", user.UserID)
}

// ProcessExpiredPure 纯函数版本，用于属性测试
// 输入：用户状态（balance, cellLevel, autoRenew, planType）
// 输出：处理后的 cellLevel
func ProcessExpiredPure(balanceUSD float64, cellLevel int, autoRenew bool, planType string) (newCellLevel int) {
	if cellLevel <= 1 {
		return 1 // Already Standard
	}
	if autoRenew && planType != "" {
		priceUSD, ok := tierPrices[planType]
		if ok {
			totalPrice := float64(priceUSD) / 100.0
			if balanceUSD >= totalPrice {
				return cellLevel // Auto-renewed, level stays
			}
		}
	}
	return 1 // Downgrade to Standard
}
