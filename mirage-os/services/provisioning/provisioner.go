// Package provisioning - 零触达自动化配置引擎
// XMR 到账 → Ed25519 凭证生成 → Shadow Cell 分配 → 阅后即焚交付
// 全链路零人工干预
package provisioning

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"mirage-os/pkg/redact"
	"sync"
	"time"

	"gorm.io/gorm"
)

// Provisioner 自动化配置引擎
type Provisioner struct {
	mu sync.RWMutex
	db *gorm.DB

	// 一次性链接存储（内存态，重启即销毁）
	burnLinks map[string]*BurnLink

	// 回调
	onProvisioned func(uid string, config *ClientConfig)

	// 配置
	linkTTL        time.Duration // 链接有效期
	maxAccessCount int           // 最大访问次数（1 = 阅后即焚）
}

// BurnLink 阅后即焚链接
type BurnLink struct {
	Token       string    `json:"token"`
	UID         string    `json:"-"`
	Payload     []byte    `json:"-"` // AES-GCM 加密的配置
	DecryptKey  string    `json:"-"` // 解密密钥（仅在生成时返回一次）
	CreatedAt   time.Time `json:"created_at"`
	ExpiresAt   time.Time `json:"expires_at"`
	AccessCount int       `json:"access_count"`
	MaxAccess   int       `json:"max_access"`
	Consumed    bool      `json:"consumed"`
}

// ClientConfig 客户端连接配置
type ClientConfig struct {
	Version     int       `json:"version"`
	GeneratedAt time.Time `json:"generated_at"`
	ExpiresAt   time.Time `json:"expires_at"`
	UID         string    `json:"uid"`
	CellID      string    `json:"cell_id"`
	CellLevel   string    `json:"cell_level"`
	Region      string    `json:"region"`

	// Ed25519 认证凭证
	PrivateKey string `json:"private_key"` // hex
	PublicKey  string `json:"public_key"`  // hex

	// 连接参数
	Endpoints []EndpointConfig `json:"endpoints"`
	SNI       string           `json:"sni"`
	CACert    string           `json:"ca_cert"`

	// 配额
	QuotaBytes  uint64 `json:"quota_bytes"`
	QuotaExpiry string `json:"quota_expiry"`
}

// EndpointConfig 端点配置
type EndpointConfig struct {
	Address  string `json:"address"`
	Port     int    `json:"port"`
	Protocol string `json:"protocol"` // quic, tcp
	Priority int    `json:"priority"`
}

// NewProvisioner 创建自动化配置引擎
func NewProvisioner(db *gorm.DB) *Provisioner {
	return &Provisioner{
		db:             db,
		burnLinks:      make(map[string]*BurnLink),
		linkTTL:        30 * time.Minute,
		maxAccessCount: 1,
	}
}

// SetOnProvisioned 设置配置完成回调
func (p *Provisioner) SetOnProvisioned(fn func(string, *ClientConfig)) {
	p.onProvisioned = fn
}

// OnXMRConfirmed XMR 到账确认回调 — 全自动流水线入口
// 由 XMRProcessor.onConfirmed 触发
func (p *Provisioner) OnXMRConfirmed(uid string, amountPiconero uint64) error {
	log.Printf("[Provisioner] 💰 XMR 到账: uid=%s, amount=%d piconero", redact.Token(uid), amountPiconero)

	// 1. 生成 Ed25519 密钥对
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return fmt.Errorf("生成密钥对失败: %w", err)
	}

	pubKeyHex := hex.EncodeToString(pubKey)
	privKeyHex := hex.EncodeToString(privKey)

	// 2. 从公钥派生 UID（双重哈希）
	first := sha256.Sum256(pubKey)
	second := sha256.Sum256(first[:])
	derivedUID := "u-" + hex.EncodeToString(second[:6])

	// 3. 分配 Shadow Cell
	cellID, cellLevel, region, err := p.allocateShadowCell(uid)
	if err != nil {
		return fmt.Errorf("分配蜂窝失败: %w", err)
	}

	// 4. 计算配额（基于充值金额）
	quotaBytes := p.calculateQuota(amountPiconero, cellLevel)

	// 5. 获取可用端点
	endpoints := p.getAvailableEndpoints(region)

	// 6. 构建客户端配置
	config := &ClientConfig{
		Version:     1,
		GeneratedAt: time.Now(),
		ExpiresAt:   time.Now().Add(30 * 24 * time.Hour),
		UID:         derivedUID,
		CellID:      cellID,
		CellLevel:   cellLevel,
		Region:      region,
		PrivateKey:  privKeyHex,
		PublicKey:   pubKeyHex,
		Endpoints:   endpoints,
		SNI:         "cdn.cloudflare.com", // 伪装 SNI
		QuotaBytes:  quotaBytes,
		QuotaExpiry: time.Now().Add(30 * 24 * time.Hour).Format(time.RFC3339),
	}

	// 7. 持久化公钥到数据库（私钥仅存在于配置中，不落盘）
	if err := p.persistCredentials(uid, pubKeyHex); err != nil {
		log.Printf("[Provisioner] ⚠️ 持久化凭证失败: %v", err)
	}

	// 8. 生成阅后即焚链接
	link, err := p.createBurnLink(uid, config)
	if err != nil {
		return fmt.Errorf("生成交付链接失败: %w", err)
	}

	log.Printf("[Provisioner] ✅ 配置完成: uid=%s, cell=%s, link=%s", redact.Token(uid), cellID, redact.Token(link.Token))

	// 9. 触发回调
	if p.onProvisioned != nil {
		go p.onProvisioned(uid, config)
	}

	return nil
}

// allocateShadowCell 分配 Shadow Cell
func (p *Provisioner) allocateShadowCell(uid string) (cellID, cellLevel, region string, err error) {
	// 查找有空位的蜂窝，优先选择用户最少的
	type CellCandidate struct {
		ID        string
		Level     string
		Region    string
		MaxUsers  int
		UserCount int
	}

	var candidates []CellCandidate
	err = p.db.Raw(`
		SELECT c.id, c.level, c.region, c.max_users,
			   COUNT(u.id) as user_count
		FROM cells c
		LEFT JOIN users u ON u.cell_id = c.id
		GROUP BY c.id, c.level, c.region, c.max_users
		HAVING COUNT(u.id) < c.max_users
		ORDER BY COUNT(u.id) ASC
		LIMIT 5
	`).Scan(&candidates).Error

	if err != nil || len(candidates) == 0 {
		return "", "", "", fmt.Errorf("无可用蜂窝")
	}

	// 选择负载最低的蜂窝
	best := candidates[0]

	// 将用户绑定到蜂窝
	if err := p.db.Exec("UPDATE users SET cell_id = ? WHERE id = ?", best.ID, uid).Error; err != nil {
		return "", "", "", fmt.Errorf("绑定蜂窝失败: %w", err)
	}

	return best.ID, best.Level, best.Region, nil
}

// calculateQuota 根据充值金额计算配额
func (p *Provisioner) calculateQuota(piconero uint64, cellLevel string) uint64 {
	// XMR → USD（假设 1 XMR ≈ $150）
	xmrAmount := float64(piconero) / 1e12
	usdAmount := xmrAmount * 150.0

	// 根据蜂窝等级计算每美元对应的流量
	var gbPerDollar float64
	switch cellLevel {
	case "DIAMOND":
		gbPerDollar = 5.0 // $0.20/GB
	case "PLATINUM":
		gbPerDollar = 6.67 // $0.15/GB
	default:
		gbPerDollar = 10.0 // $0.10/GB
	}

	totalGB := usdAmount * gbPerDollar
	return uint64(totalGB * 1024 * 1024 * 1024)
}

// getAvailableEndpoints 获取可用端点
func (p *Provisioner) getAvailableEndpoints(region string) []EndpointConfig {
	// 从数据库查询该区域在线的 Gateway
	type GWInfo struct {
		IPAddress string
	}

	var gateways []GWInfo
	p.db.Raw(`
		SELECT ip_address FROM gateways
		WHERE status = 'ONLINE' AND cell_id IN (
			SELECT id FROM cells WHERE region = ?
		)
		ORDER BY active_connections ASC
		LIMIT 3
	`, region).Scan(&gateways)

	endpoints := make([]EndpointConfig, 0, len(gateways))
	for i, gw := range gateways {
		if gw.IPAddress == "" {
			continue
		}
		endpoints = append(endpoints, EndpointConfig{
			Address:  gw.IPAddress,
			Port:     443,
			Protocol: "quic",
			Priority: i + 1,
		})
	}

	// 兜底：至少返回一个默认端点
	if len(endpoints) == 0 {
		endpoints = append(endpoints, EndpointConfig{
			Address:  "0.0.0.0",
			Port:     443,
			Protocol: "quic",
			Priority: 1,
		})
	}

	return endpoints
}

// persistCredentials 持久化公钥
func (p *Provisioner) persistCredentials(uid, pubKeyHex string) error {
	return p.db.Exec(
		"UPDATE users SET ed25519_pubkey = ? WHERE id = ?",
		pubKeyHex, uid,
	).Error
}

// createBurnLink 创建阅后即焚链接
func (p *Provisioner) createBurnLink(uid string, config *ClientConfig) (*BurnLink, error) {
	// 1. 序列化配置
	plaintext, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}

	// 2. 生成随机 AES-256 密钥
	aesKey := make([]byte, 32)
	if _, err := rand.Read(aesKey); err != nil {
		return nil, err
	}

	// 3. AES-GCM 加密
	block, err := aes.NewCipher(aesKey)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)

	// 4. 生成链接 Token
	tokenBytes := make([]byte, 32)
	rand.Read(tokenBytes)
	token := hex.EncodeToString(tokenBytes)

	link := &BurnLink{
		Token:       token,
		UID:         uid,
		Payload:     ciphertext,
		DecryptKey:  base64.URLEncoding.EncodeToString(aesKey),
		CreatedAt:   time.Now(),
		ExpiresAt:   time.Now().Add(p.linkTTL),
		MaxAccess:   p.maxAccessCount,
		AccessCount: 0,
		Consumed:    false,
	}

	p.mu.Lock()
	p.burnLinks[token] = link
	p.mu.Unlock()

	return link, nil
}

// RedeemBurnLink 兑换阅后即焚链接（一次性）
func (p *Provisioner) RedeemBurnLink(token, decryptKey string) ([]byte, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	link, exists := p.burnLinks[token]
	if !exists {
		return nil, fmt.Errorf("链接不存在或已销毁")
	}

	if link.Consumed {
		// 已消费，立即删除并返回错误
		delete(p.burnLinks, token)
		return nil, fmt.Errorf("链接已被使用")
	}

	if time.Now().After(link.ExpiresAt) {
		delete(p.burnLinks, token)
		return nil, fmt.Errorf("链接已过期")
	}

	// 解密
	aesKey, err := base64.URLEncoding.DecodeString(decryptKey)
	if err != nil {
		return nil, fmt.Errorf("密钥格式错误")
	}

	block, err := aes.NewCipher(aesKey)
	if err != nil {
		return nil, fmt.Errorf("解密失败")
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("解密失败")
	}

	nonceSize := gcm.NonceSize()
	if len(link.Payload) < nonceSize {
		return nil, fmt.Errorf("数据损坏")
	}

	nonce, ciphertext := link.Payload[:nonceSize], link.Payload[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("解密失败")
	}

	// 标记已消费并立即销毁
	link.Consumed = true
	link.AccessCount++
	delete(p.burnLinks, token)

	log.Printf("[Provisioner] 🔥 链接已兑换并销毁: uid=%s", redact.Token(link.UID))

	return plaintext, nil
}

// CleanExpiredLinks 清理过期链接
func (p *Provisioner) CleanExpiredLinks() {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now()
	for token, link := range p.burnLinks {
		if now.After(link.ExpiresAt) {
			delete(p.burnLinks, token)
		}
	}
}

// StartCleanupLoop 启动清理循环
func (p *Provisioner) StartCleanupLoop(stopCh <-chan struct{}) {
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-stopCh:
				return
			case <-ticker.C:
				p.CleanExpiredLinks()
			}
		}
	}()
}

// GetLinkStatus 获取链接状态（不暴露内容）
func (p *Provisioner) GetLinkStatus(token string) (exists bool, consumed bool, expired bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	link, ok := p.burnLinks[token]
	if !ok {
		return false, false, false
	}
	return true, link.Consumed, time.Now().After(link.ExpiresAt)
}
