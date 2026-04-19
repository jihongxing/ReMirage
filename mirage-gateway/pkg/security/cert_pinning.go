package security

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"log"
	"sync"
)

// CertPin 证书钉扎管理器
type CertPin struct {
	mu         sync.RWMutex
	pinnedHash [32]byte
	pinned     bool
	configHash string
}

// NewCertPin 创建证书钉扎管理器
func NewCertPin(configHash string) *CertPin {
	cp := &CertPin{
		configHash: configHash,
	}
	if configHash != "" {
		decoded, err := hex.DecodeString(configHash)
		if err == nil && len(decoded) == 32 {
			copy(cp.pinnedHash[:], decoded)
			cp.pinned = true
			log.Printf("[CertPin] ✅ 已加载预设证书指纹: %s", configHash[:16]+"...")
		} else {
			log.Printf("[CertPin] ⚠️ 预设指纹格式无效，忽略")
		}
	}
	return cp
}

// PinCertificate 钉扎证书
func (cp *CertPin) PinCertificate(cert *x509.Certificate) error {
	if cert == nil {
		return fmt.Errorf("证书为 nil")
	}
	hash := sha256.Sum256(cert.Raw)

	cp.mu.Lock()
	cp.pinnedHash = hash
	cp.pinned = true
	cp.mu.Unlock()

	log.Printf("[CertPin] ✅ 证书已钉扎: %s", hex.EncodeToString(hash[:])[:16]+"...")
	return nil
}

// VerifyPin 验证证书指纹
func (cp *CertPin) VerifyPin(cert *x509.Certificate) error {
	if cert == nil {
		return fmt.Errorf("证书为 nil")
	}

	cp.mu.RLock()
	defer cp.mu.RUnlock()

	if !cp.pinned {
		return fmt.Errorf("证书未钉扎")
	}

	hash := sha256.Sum256(cert.Raw)
	if hash != cp.pinnedHash {
		log.Printf("[CertPin] 🚨 证书指纹不匹配！预期: %s, 实际: %s",
			hex.EncodeToString(cp.pinnedHash[:])[:16], hex.EncodeToString(hash[:])[:16])
		return fmt.Errorf("证书指纹不匹配")
	}

	return nil
}

// UpdatePin 更新钉扎指纹
func (cp *CertPin) UpdatePin(newCert *x509.Certificate) error {
	if newCert == nil {
		return fmt.Errorf("证书为 nil")
	}
	hash := sha256.Sum256(newCert.Raw)

	cp.mu.Lock()
	cp.pinnedHash = hash
	cp.pinned = true
	cp.mu.Unlock()

	log.Printf("[CertPin] ✅ 证书钉扎已更新: %s", hex.EncodeToString(hash[:])[:16]+"...")
	return nil
}

// GetPinnedHash 获取当前钉扎的指纹
func (cp *CertPin) GetPinnedHash() string {
	cp.mu.RLock()
	defer cp.mu.RUnlock()
	if !cp.pinned {
		return ""
	}
	return hex.EncodeToString(cp.pinnedHash[:])
}

// IsPinned 返回是否已钉扎
func (cp *CertPin) IsPinned() bool {
	cp.mu.RLock()
	defer cp.mu.RUnlock()
	return cp.pinned
}
