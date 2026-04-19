// Package auth - 影子认证：挑战-响应机制
package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"mirage-os/pkg/models"

	"github.com/go-redis/redis/v8"
	"gorm.io/gorm"
)

const (
	// ChallengeTTL 挑战有效期（5 分钟）
	ChallengeTTL = 5 * time.Minute
	
	// ChallengePrefix Redis 键前缀
	ChallengePrefix = "mirage:auth:challenge:"
)

// ChallengeManager 挑战管理器
type ChallengeManager struct {
	DB    *gorm.DB
	Redis *redis.Client
}

// NewChallengeManager 创建挑战管理器
func NewChallengeManager(db *gorm.DB, rdb *redis.Client) *ChallengeManager {
	return &ChallengeManager{
		DB:    db,
		Redis: rdb,
	}
}

// GenerateChallenge 生成登录挑战
func (cm *ChallengeManager) GenerateChallenge(userID, ipAddress string) (*models.AuthChallenge, error) {
	// 1. 验证用户存在
	var user models.User
	if err := cm.DB.Where("user_id = ? AND status = ?", userID, "active").First(&user).Error; err != nil {
		return nil, fmt.Errorf("用户不存在或已被禁用: %w", err)
	}

	// 2. 生成随机盐和挑战 ID
	salt := make([]byte, 32)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("生成盐失败: %w", err)
	}

	challengeID := make([]byte, 32)
	if _, err := rand.Read(challengeID); err != nil {
		return nil, fmt.Errorf("生成挑战 ID 失败: %w", err)
	}

	// 3. 构造挑战字符串
	timestamp := time.Now().Unix()
	challenge := fmt.Sprintf("mirage-auth:%s:%d:%s", userID, timestamp, hex.EncodeToString(salt))

	// 4. 创建挑战记录
	authChallenge := &models.AuthChallenge{
		ChallengeID: hex.EncodeToString(challengeID),
		UserID:      userID,
		Challenge:   challenge,
		Salt:        hex.EncodeToString(salt),
		IPAddress:   ipAddress,
		Status:      "pending",
		ExpiresAt:   time.Now().Add(ChallengeTTL),
	}

	// 5. 保存到数据库
	if err := cm.DB.Create(authChallenge).Error; err != nil {
		return nil, fmt.Errorf("保存挑战失败: %w", err)
	}

	// 6. 缓存到 Redis（快速验证）
	ctx := context.Background()
	key := ChallengePrefix + authChallenge.ChallengeID
	if err := cm.Redis.Set(ctx, key, challenge, ChallengeTTL).Err(); err != nil {
		return nil, fmt.Errorf("缓存挑战失败: %w", err)
	}

	return authChallenge, nil
}

// GetChallenge 获取挑战详情
func (cm *ChallengeManager) GetChallenge(challengeID string) (*models.AuthChallenge, error) {
	var challenge models.AuthChallenge
	if err := cm.DB.Where("challenge_id = ? AND status = ?", challengeID, "pending").First(&challenge).Error; err != nil {
		return nil, fmt.Errorf("挑战不存在或已失效: %w", err)
	}

	// 检查是否过期
	if time.Now().After(challenge.ExpiresAt) {
		cm.DB.Model(&challenge).Update("status", "expired")
		return nil, fmt.Errorf("挑战已过期")
	}

	return &challenge, nil
}

// InvalidateChallenge 使挑战失效
func (cm *ChallengeManager) InvalidateChallenge(challengeID string) error {
	// 1. 更新数据库
	if err := cm.DB.Model(&models.AuthChallenge{}).
		Where("challenge_id = ?", challengeID).
		Update("status", "expired").Error; err != nil {
		return err
	}

	// 2. 删除 Redis 缓存
	ctx := context.Background()
	key := ChallengePrefix + challengeID
	return cm.Redis.Del(ctx, key).Err()
}

// CleanupExpiredChallenges 清理过期挑战（定时任务）
func (cm *ChallengeManager) CleanupExpiredChallenges() error {
	return cm.DB.Model(&models.AuthChallenge{}).
		Where("status = ? AND expires_at < ?", "pending", time.Now()).
		Update("status", "expired").Error
}
