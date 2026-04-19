// Package crypto - 盲备份管理
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"io"
	"log"
)

// BackupManager 盲备份管理器
type BackupManager struct {
	shamirConfig ShamirConfig
	masterKey    []byte // 热密钥（内存常驻）
	shares       []Share
	hotKeyActive bool   // 热密钥是否激活
}

// NewBackupManager 创建备份管理器
func NewBackupManager(threshold, totalShares int) (*BackupManager, error) {
	if threshold > totalShares {
		return nil, fmt.Errorf("阈值不能大于总份额数")
	}
	
	// 生成主密钥（32 字节 AES-256）
	masterKey := make([]byte, 32)
	if _, err := rand.Read(masterKey); err != nil {
		return nil, fmt.Errorf("生成主密钥失败: %w", err)
	}
	
	// 分割主密钥
	shares, err := SplitSecret(masterKey, ShamirConfig{
		Threshold: threshold,
		Shares:    totalShares,
	})
	if err != nil {
		return nil, fmt.Errorf("分割主密钥失败: %w", err)
	}
	
	log.Printf("[Backup] 生成主密钥并分割为 %d 份（阈值: %d）", totalShares, threshold)
	
	return &BackupManager{
		shamirConfig: ShamirConfig{
			Threshold: threshold,
			Shares:    totalShares,
		},
		masterKey:    masterKey,
		shares:       shares,
		hotKeyActive: true, // 默认激活热密钥
	}, nil
}

// GetShare 获取指定索引的份额
func (bm *BackupManager) GetShare(index int) (*Share, error) {
	if index < 0 || index >= len(bm.shares) {
		return nil, fmt.Errorf("份额索引越界: %d", index)
	}
	
	return &bm.shares[index], nil
}

// RecoverMasterKey 从份额恢复主密钥
func (bm *BackupManager) RecoverMasterKey(shares []Share) ([]byte, error) {
	if len(shares) < bm.shamirConfig.Threshold {
		return nil, fmt.Errorf("份额数量不足，需要至少 %d 份", bm.shamirConfig.Threshold)
	}
	
	masterKey, err := CombineShares(shares)
	if err != nil {
		return nil, fmt.Errorf("恢复主密钥失败: %w", err)
	}
	
	log.Printf("[Backup] 从 %d 份额恢复主密钥", len(shares))
	
	return masterKey, nil
}

// EncryptData 加密数据
func (bm *BackupManager) EncryptData(plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(bm.masterKey)
	if err != nil {
		return nil, fmt.Errorf("创建加密器失败: %w", err)
	}
	
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("创建 GCM 失败: %w", err)
	}
	
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("生成 nonce 失败: %w", err)
	}
	
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	
	return ciphertext, nil
}

// DecryptData 解密数据
func (bm *BackupManager) DecryptData(ciphertext []byte, masterKey []byte) ([]byte, error) {
	block, err := aes.NewCipher(masterKey)
	if err != nil {
		return nil, fmt.Errorf("创建解密器失败: %w", err)
	}
	
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("创建 GCM 失败: %w", err)
	}
	
	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("密文长度不足")
	}
	
	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("解密失败: %w", err)
	}
	
	return plaintext, nil
}


// GetMasterKey 获取主密钥（热密钥优先）
func (bm *BackupManager) GetMasterKey() ([]byte, error) {
	if bm.hotKeyActive && bm.masterKey != nil {
		// 热密钥路径：零延迟
		return bm.masterKey, nil
	}
	
	// 冷启动路径：需要 Shamir 恢复
	log.Println("[Backup] ⚠️ 热密钥未激活，执行 Shamir 恢复（冷启动）")
	return nil, fmt.Errorf("热密钥未激活，需要执行 Shamir 恢复")
}

// ActivateHotKey 激活热密钥（从份额恢复）
func (bm *BackupManager) ActivateHotKey(shares []Share) error {
	if bm.hotKeyActive {
		log.Println("[Backup] 热密钥已激活，跳过恢复")
		return nil
	}
	
	log.Println("[Backup] 🔥 激活热密钥（从 Shamir 份额恢复）")
	
	masterKey, err := bm.RecoverMasterKey(shares)
	if err != nil {
		return fmt.Errorf("恢复主密钥失败: %w", err)
	}
	
	bm.masterKey = masterKey
	bm.hotKeyActive = true
	
	log.Println("[Backup] ✅ 热密钥已激活，后续操作零延迟")
	
	return nil
}

// DeactivateHotKey 停用热密钥（清空内存）
func (bm *BackupManager) DeactivateHotKey() {
	if !bm.hotKeyActive {
		return
	}
	
	log.Println("[Backup] 🧹 停用热密钥（清空内存）")
	
	// 清零主密钥
	for i := range bm.masterKey {
		bm.masterKey[i] = 0
	}
	bm.masterKey = nil
	bm.hotKeyActive = false
	
	log.Println("[Backup] ✅ 热密钥已清空")
}

// IsHotKeyActive 检查热密钥是否激活
func (bm *BackupManager) IsHotKeyActive() bool {
	return bm.hotKeyActive
}

// EncryptDataFast 快速加密（使用热密钥）
func (bm *BackupManager) EncryptDataFast(plaintext []byte) ([]byte, error) {
	if !bm.hotKeyActive {
		return nil, fmt.Errorf("热密钥未激活，无法快速加密")
	}
	
	return bm.EncryptData(plaintext)
}

// DecryptDataFast 快速解密（使用热密钥）
func (bm *BackupManager) DecryptDataFast(ciphertext []byte) ([]byte, error) {
	if !bm.hotKeyActive {
		return nil, fmt.Errorf("热密钥未激活，无法快速解密")
	}
	
	return bm.DecryptData(ciphertext, bm.masterKey)
}
