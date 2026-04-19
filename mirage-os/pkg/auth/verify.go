// Package auth - 影子认证：签名验证
package auth

import (
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"time"

	"mirage-os/pkg/models"

	"github.com/go-redis/redis/v8"
	"gorm.io/gorm"
)

// VerifyManager 签名验证管理器
type VerifyManager struct {
	DB    *gorm.DB
	Redis *redis.Client
}

// NewVerifyManager 创建验证管理器
func NewVerifyManager(db *gorm.DB, rdb *redis.Client) *VerifyManager {
	return &VerifyManager{
		DB:    db,
		Redis: rdb,
	}
}

// VerifySignature 验证硬件签名
func (vm *VerifyManager) VerifySignature(challengeID, signature string) (*models.User, error) {
	// 1. 获取挑战记录
	var challenge models.AuthChallenge
	if err := vm.DB.Where("challenge_id = ? AND status = ?", challengeID, "pending").First(&challenge).Error; err != nil {
		return nil, fmt.Errorf("挑战不存在或已失效: %w", err)
	}

	// 2. 检查是否过期
	if time.Now().After(challenge.ExpiresAt) {
		vm.DB.Model(&challenge).Update("status", "expired")
		return nil, fmt.Errorf("挑战已过期")
	}

	// 3. 获取用户公钥
	var user models.User
	if err := vm.DB.Where("user_id = ? AND status = ?", challenge.UserID, "active").First(&user).Error; err != nil {
		return nil, fmt.Errorf("用户不存在或已被禁用: %w", err)
	}

	if user.HardwarePublicKey == "" {
		return nil, fmt.Errorf("用户未绑定硬件公钥")
	}

	// 4. 解码公钥和签名
	publicKeyBytes, err := hex.DecodeString(user.HardwarePublicKey)
	if err != nil {
		return nil, fmt.Errorf("公钥格式错误: %w", err)
	}

	signatureBytes, err := hex.DecodeString(signature)
	if err != nil {
		return nil, fmt.Errorf("签名格式错误: %w", err)
	}

	// 5. 验证签名（Ed25519）
	if len(publicKeyBytes) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("公钥长度错误，期望 %d 字节，实际 %d 字节", ed25519.PublicKeySize, len(publicKeyBytes))
	}

	publicKey := ed25519.PublicKey(publicKeyBytes)
	message := []byte(challenge.Challenge)

	if !ed25519.Verify(publicKey, message, signatureBytes) {
		// 验证失败，标记挑战
		vm.DB.Model(&challenge).Update("status", "failed")
		return nil, fmt.Errorf("签名验证失败")
	}

	// 6. 验证成功，更新挑战状态
	now := time.Now()
	if err := vm.DB.Model(&challenge).Updates(map[string]interface{}{
		"status":      "verified",
		"verified_at": &now,
	}).Error; err != nil {
		return nil, fmt.Errorf("更新挑战状态失败: %w", err)
	}

	// 7. 更新用户最后登录信息
	if err := vm.DB.Model(&user).Updates(map[string]interface{}{
		"last_login_at": &now,
		"last_login_ip": challenge.IPAddress,
	}).Error; err != nil {
		return nil, fmt.Errorf("更新用户登录信息失败: %w", err)
	}

	return &user, nil
}

// VerifyHardwareBinding 验证硬件绑定（注册时）
func (vm *VerifyManager) VerifyHardwareBinding(publicKeyHex, testSignatureHex, testMessage string) error {
	// 1. 解码公钥
	publicKeyBytes, err := hex.DecodeString(publicKeyHex)
	if err != nil {
		return fmt.Errorf("公钥格式错误: %w", err)
	}

	if len(publicKeyBytes) != ed25519.PublicKeySize {
		return fmt.Errorf("公钥长度错误，期望 %d 字节", ed25519.PublicKeySize)
	}

	// 2. 解码签名
	signatureBytes, err := hex.DecodeString(testSignatureHex)
	if err != nil {
		return fmt.Errorf("签名格式错误: %w", err)
	}

	// 3. 验证测试签名
	publicKey := ed25519.PublicKey(publicKeyBytes)
	message := []byte(testMessage)

	if !ed25519.Verify(publicKey, message, signatureBytes) {
		return fmt.Errorf("硬件绑定验证失败：签名不匹配")
	}

	return nil
}

// GenerateHardwareFingerprint 生成硬件指纹（基于公钥哈希）
func GenerateHardwareFingerprint(publicKeyHex string) (string, error) {
	publicKeyBytes, err := hex.DecodeString(publicKeyHex)
	if err != nil {
		return "", err
	}

	// 使用公钥的 SHA256 作为指纹
	fingerprint := fmt.Sprintf("%x", publicKeyBytes[:16]) // 取前 16 字节
	return fingerprint, nil
}
