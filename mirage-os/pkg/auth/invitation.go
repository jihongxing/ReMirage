// Package auth - 邀请制管理
package auth

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"mirage-os/pkg/models"

	"gorm.io/gorm"
)

// InvitationManager 邀请码管理器
type InvitationManager struct {
	DB *gorm.DB
}

// NewInvitationManager 创建邀请管理器
func NewInvitationManager(db *gorm.DB) *InvitationManager {
	return &InvitationManager{DB: db}
}

// GenerateInvitation 生成邀请码
func (im *InvitationManager) GenerateInvitation(creatorUserID string, validDays int) (*models.Invitation, error) {
	// 1. 验证创建者权限
	var creator models.User
	if err := im.DB.Where("user_id = ?", creatorUserID).First(&creator).Error; err != nil {
		return nil, fmt.Errorf("用户不存在: %w", err)
	}

	if creator.InviteQuota <= 0 {
		return nil, fmt.Errorf("邀请配额不足")
	}

	if creator.TrustScore < 60 {
		return nil, fmt.Errorf("信用分不足 60，无法生成邀请码")
	}

	// 2. 生成随机邀请码
	codeBytes := make([]byte, 16)
	if _, err := rand.Read(codeBytes); err != nil {
		return nil, fmt.Errorf("生成邀请码失败: %w", err)
	}

	code := hex.EncodeToString(codeBytes)

	// 3. 创建邀请记录
	invitation := &models.Invitation{
		Code:      code,
		CreatedBy: creatorUserID,
		Status:    "unused",
		ExpiresAt: time.Now().AddDate(0, 0, validDays),
	}

	// 4. 开启事务
	err := im.DB.Transaction(func(tx *gorm.DB) error {
		// 保存邀请码
		if err := tx.Create(invitation).Error; err != nil {
			return fmt.Errorf("保存邀请码失败: %w", err)
		}

		// 扣减邀请配额
		if err := tx.Model(&creator).Update("invite_quota", gorm.Expr("invite_quota - ?", 1)).Error; err != nil {
			return fmt.Errorf("扣减邀请配额失败: %w", err)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return invitation, nil
}

// ValidateInvitation 验证邀请码
func (im *InvitationManager) ValidateInvitation(code string) (*models.Invitation, error) {
	var invitation models.Invitation
	if err := im.DB.Where("code = ?", code).First(&invitation).Error; err != nil {
		return nil, fmt.Errorf("邀请码不存在: %w", err)
	}

	// 检查状态
	if invitation.Status != "unused" {
		return nil, fmt.Errorf("邀请码已被使用或已过期")
	}

	// 检查是否过期
	if time.Now().After(invitation.ExpiresAt) {
		im.DB.Model(&invitation).Update("status", "expired")
		return nil, fmt.Errorf("邀请码已过期")
	}

	return &invitation, nil
}

// UseInvitation 使用邀请码
func (im *InvitationManager) UseInvitation(code, newUserID string) error {
	// 1. 验证邀请码
	invitation, err := im.ValidateInvitation(code)
	if err != nil {
		return err
	}

	// 2. 开启事务
	return im.DB.Transaction(func(tx *gorm.DB) error {
		// 标记邀请码已使用
		now := time.Now()
		if err := tx.Model(invitation).Updates(map[string]interface{}{
			"status":  "used",
			"used_by": newUserID,
			"used_at": &now,
		}).Error; err != nil {
			return fmt.Errorf("更新邀请码状态失败: %w", err)
		}

		// 增加邀请人的信用分
		if err := tx.Model(&models.User{}).
			Where("user_id = ?", invitation.CreatedBy).
			Update("trust_score", gorm.Expr("LEAST(trust_score + 5, 100)")).Error; err != nil {
			return fmt.Errorf("更新邀请人信用分失败: %w", err)
		}

		return nil
	})
}

// GetUserInvitations 获取用户的邀请记录
func (im *InvitationManager) GetUserInvitations(userID string) ([]models.Invitation, error) {
	var invitations []models.Invitation
	if err := im.DB.Where("created_by = ?", userID).
		Order("created_at DESC").
		Find(&invitations).Error; err != nil {
		return nil, err
	}

	return invitations, nil
}

// GrantInviteQuota 授予邀请配额（管理员操作）
func (im *InvitationManager) GrantInviteQuota(userID string, quota int) error {
	return im.DB.Model(&models.User{}).
		Where("user_id = ?", userID).
		Update("invite_quota", gorm.Expr("invite_quota + ?", quota)).Error
}
