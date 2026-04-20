// Package billing - 邀请码生成与核销
// 控制客源质量：每个用户有限的邀请配额，邀请人信用分影响被邀请人初始信用
package billing

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"mirage-os/pkg/models"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	// DefaultInviteQuota 默认邀请配额
	DefaultInviteQuota = 3
	// InviteCodeLength 邀请码长度（字节，hex 编码后 *2）
	InviteCodeLength = 8
	// InviteCodeTTL 邀请码有效期
	InviteCodeTTL = 7 * 24 * time.Hour
)

// InvitationService 邀请码服务
type InvitationService struct {
	db *gorm.DB
}

// NewInvitationService 创建邀请码服务
func NewInvitationService(db *gorm.DB) *InvitationService {
	return &InvitationService{db: db}
}

// GenerateInviteCode 生成邀请码
func (s *InvitationService) GenerateInviteCode(ctx context.Context, creatorUID string) (string, error) {
	// 检查创建者是否存在且有配额
	var user models.User
	if err := s.db.Where("user_id = ?", creatorUID).First(&user).Error; err != nil {
		return "", fmt.Errorf("用户不存在")
	}

	if user.Status != "active" {
		return "", fmt.Errorf("账户已被冻结")
	}

	// 检查剩余邀请配额
	var usedCount int64
	s.db.Model(&models.Invitation{}).Where("created_by = ? AND status = 'unused'", creatorUID).Count(&usedCount)

	if int(usedCount) >= user.InviteQuota {
		return "", fmt.Errorf("邀请配额已用尽 (%d/%d)", usedCount, user.InviteQuota)
	}

	// 生成唯一邀请码
	code, err := generateUniqueCode(s.db)
	if err != nil {
		return "", err
	}

	invitation := models.Invitation{
		Code:      code,
		CreatedBy: creatorUID,
		Status:    "unused",
		ExpiresAt: time.Now().Add(InviteCodeTTL),
	}

	if err := s.db.Create(&invitation).Error; err != nil {
		return "", fmt.Errorf("创建邀请码失败: %w", err)
	}

	return code, nil
}

// RedeemInviteCode 核销邀请码（新用户注册时调用）
// 使用 SELECT ... FOR UPDATE 悲观锁防止并发双花
func (s *InvitationService) RedeemInviteCode(ctx context.Context, code string, newUserID string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var invitation models.Invitation
		// ⚠️ 关键：FOR UPDATE 行级排他锁
		// 并发请求到达时，第二个请求会阻塞在这里直到第一个事务提交
		// 提交后第二个请求读到的 status 已经是 "used"，自然被拒绝
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("code = ? AND status = 'unused'", code).
			First(&invitation).Error; err != nil {
			return fmt.Errorf("邀请码不存在或已被使用")
		}

		if time.Now().After(invitation.ExpiresAt) {
			tx.Model(&invitation).Update("status", "expired")
			return fmt.Errorf("邀请码已过期")
		}

		// 标记已使用
		now := time.Now()
		if err := tx.Model(&invitation).Updates(map[string]any{
			"status":  "used",
			"used_by": newUserID,
			"used_at": &now,
		}).Error; err != nil {
			return fmt.Errorf("核销失败: %w", err)
		}

		// 更新新用户的邀请人信息
		if err := tx.Model(&models.User{}).Where("user_id = ?", newUserID).Updates(map[string]any{
			"invited_by":   invitation.CreatedBy,
			"invite_quota": DefaultInviteQuota,
		}).Error; err != nil {
			return fmt.Errorf("更新邀请关系失败: %w", err)
		}

		// 邀请人信用分 +5（裂变奖励）
		if err := tx.Model(&models.User{}).Where("user_id = ?", invitation.CreatedBy).
			Update("trust_score", gorm.Expr("LEAST(trust_score + 5, 100)")).Error; err != nil {
			return fmt.Errorf("更新信用分失败: %w", err)
		}

		return nil
	})
}

// GetUserInvitations 查询用户的邀请码列表
func (s *InvitationService) GetUserInvitations(ctx context.Context, userID string) ([]models.Invitation, error) {
	var invitations []models.Invitation
	if err := s.db.Where("created_by = ?", userID).Order("created_at DESC").Find(&invitations).Error; err != nil {
		return nil, err
	}
	return invitations, nil
}

// CleanExpiredInvitations 清理过期邀请码
func (s *InvitationService) CleanExpiredInvitations() {
	s.db.Model(&models.Invitation{}).
		Where("status = 'unused' AND expires_at < ?", time.Now()).
		Update("status", "expired")
}

// generateUniqueCode 生成全局唯一邀请码
func generateUniqueCode(db *gorm.DB) (string, error) {
	for i := 0; i < 10; i++ {
		buf := make([]byte, InviteCodeLength)
		if _, err := rand.Read(buf); err != nil {
			return "", err
		}
		code := hex.EncodeToString(buf)

		// 检查唯一性
		var count int64
		db.Model(&models.Invitation{}).Where("code = ?", code).Count(&count)
		if count == 0 {
			return code, nil
		}
	}
	return "", fmt.Errorf("无法生成唯一邀请码")
}
