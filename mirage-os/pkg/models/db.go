// Package models - 数据库模型定义
package models

import (
	"time"

	"gorm.io/gorm"
)

// ============================================
// 月费产品类型常量
// ============================================

const (
	PlanStandardMonthly = "plan_standard_monthly"
	PlanPlatinumMonthly = "plan_platinum_monthly"
	PlanDiamondMonthly  = "plan_diamond_monthly"
)

// ============================================
// 用户与资产核心
// ============================================

// User 用户账户与配额（影子认证模型）
type User struct {
	ID     uint32 `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID string `gorm:"uniqueIndex;size:64;not null" json:"user_id"`

	// 硬件指纹绑定（影子认证核心）
	HardwarePublicKey   string `gorm:"uniqueIndex;size:128;not null" json:"hardware_public_key"` // Ed25519/SM2 公钥
	HardwareFingerprint string `gorm:"index;size:64" json:"hardware_fingerprint"`                // 设备指纹哈希

	// 邀请制
	InvitedBy   string `gorm:"index;size:64" json:"invited_by"`                                   // 邀请人 UserID
	InviteCode  string `gorm:"uniqueIndex;size:32" json:"invite_code"`                            // 用户专属邀请码
	InviteQuota int    `gorm:"default:0" json:"invite_quota"`                                     // 可邀请人数
	TrustScore  int    `gorm:"default:50;check:trust_score BETWEEN 0 AND 100" json:"trust_score"` // 信用分

	// 资产
	XMRAddress     string     `gorm:"uniqueIndex;size:95" json:"xmr_address"`
	Balance        float64    `gorm:"type:numeric(20,8);default:0;not null" json:"balance"`                  // XMR
	BalanceUSD     float64    `gorm:"type:numeric(20,2);default:0;not null" json:"balance_usd"`              // USD
	RemainingQuota int64      `gorm:"default:0;not null" json:"remaining_quota"`                             // 剩余配额（字节）
	TotalQuota     int64      `gorm:"default:0;not null" json:"total_quota"`                                 // 总配额（字节）
	CellLevel      int        `gorm:"default:1;not null;check:cell_level BETWEEN 1 AND 3" json:"cell_level"` // 1:标准, 2:白金, 3:钻石
	AutoRenew      bool       `gorm:"default:false" json:"auto_renew"`
	QuotaExpiresAt *time.Time `json:"quota_expires_at"`

	// 等级订阅
	SubscriptionExpiresAt   *time.Time `json:"subscription_expires_at"`
	SubscriptionPackageType string     `gorm:"size:32;default:''" json:"subscription_package_type"`

	// 审计
	LastLoginAt *time.Time `json:"last_login_at"`
	LastLoginIP string     `gorm:"size:45" json:"last_login_ip"`
	Status      string     `gorm:"index;size:16;default:'active';check:status IN ('active','suspended','banned')" json:"status"`
	CreatedAt   time.Time  `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt   time.Time  `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName 指定表名
func (User) TableName() string {
	return "users"
}

// ============================================
// 蜂窝与节点拓扑
// ============================================

// Cell 蜂窝拓扑
type Cell struct {
	ID             uint32    `gorm:"primaryKey;autoIncrement" json:"id"`
	CellID         string    `gorm:"uniqueIndex;size:32;not null" json:"cell_id"`
	CellName       string    `gorm:"size:128;not null" json:"cell_name"`
	RegionCode     string    `gorm:"index;size:16;not null" json:"region_code"`
	Country        string    `gorm:"index;size:2;not null" json:"country"`
	City           string    `gorm:"size:64" json:"city"`
	Latitude       float64   `gorm:"type:numeric(10,7)" json:"latitude"`
	Longitude      float64   `gorm:"type:numeric(10,7)" json:"longitude"`
	Jurisdiction   string    `gorm:"size:64" json:"jurisdiction"`
	CellLevel      int       `gorm:"default:1;not null" json:"cell_level"`
	CostMultiplier float64   `gorm:"type:numeric(5,2);default:1.0;check:cost_multiplier > 0" json:"cost_multiplier"`
	MaxGateways    int       `gorm:"default:100" json:"max_gateways"`
	Status         string    `gorm:"index;size:16;default:'active';check:status IN ('active','maintenance','offline')" json:"status"`
	CreatedAt      time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt      time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName 指定表名
func (Cell) TableName() string {
	return "cells"
}

// Gateway Gateway 节点状态
type Gateway struct {
	ID        uint32 `gorm:"primaryKey;autoIncrement" json:"id"`
	GatewayID string `gorm:"uniqueIndex;size:32;not null" json:"gateway_id"`
	CellID    string `gorm:"index;size:32;not null" json:"cell_id"`
	UserID    string `gorm:"index;size:64" json:"user_id"`
	IPAddress string `gorm:"size:45;not null" json:"ip_address"`
	Version   string `gorm:"size:16" json:"version"`

	// 影子蜂窝生命周期
	Phase               int        `gorm:"index;default:0;check:phase BETWEEN 0 AND 2" json:"phase"` // 0:潜伏, 1:校准, 2:服役
	IncubationStartedAt *time.Time `gorm:"index" json:"incubation_started_at"`
	NetworkQuality      float64    `gorm:"type:numeric(5,2);default:0" json:"network_quality"`      // 0-100 校准期计算
	BaselineRTT         int        `gorm:"default:0" json:"baseline_rtt"`                           // 基准 RTT (微秒)
	BaselinePacketLoss  float64    `gorm:"type:numeric(5,2);default:0" json:"baseline_packet_loss"` // 基准丢包率

	CurrentThreatLevel int        `gorm:"default:0" json:"current_threat_level"`
	ActiveConnections  int        `gorm:"default:0" json:"active_connections"`
	CPUPercent         float64    `gorm:"type:numeric(5,2);default:0" json:"cpu_percent"`
	MemoryBytes        int64      `gorm:"default:0" json:"memory_bytes"`
	BandwidthBps       int64      `gorm:"default:0" json:"bandwidth_bps"`
	IsOnline           bool       `gorm:"index;default:false" json:"is_online"`
	Status             string     `gorm:"index;size:16;default:'OFFLINE'" json:"status"` // ONLINE/DEGRADED/UNDER_ATTACK/DRAINING/DEAD/OFFLINE
	LastHeartbeatAt    *time.Time `gorm:"index" json:"last_heartbeat_at"`
	CreatedAt          time.Time  `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt          time.Time  `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName 指定表名
func (Gateway) TableName() string {
	return "gateways"
}

// ============================================
// 计费流水与情报仓库
// ============================================

// BillingLog 计费流水（高频写入）
type BillingLog struct {
	ID             uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	LogID          string    `gorm:"type:uuid;default:uuid_generate_v4()" json:"log_id"`
	GatewayID      string    `gorm:"index;size:32" json:"gateway_id"`
	UserID         string    `gorm:"index;size:64" json:"user_id"`
	CellID         string    `gorm:"size:32" json:"cell_id"`
	BusinessBytes  int64     `gorm:"default:0;check:business_bytes >= 0" json:"business_bytes"`
	DefenseBytes   int64     `gorm:"default:0;check:defense_bytes >= 0" json:"defense_bytes"`
	TotalBytes     int64     `gorm:"default:0" json:"total_bytes"`
	CostUSD        float64   `gorm:"type:numeric(20,8);default:0" json:"cost_usd"`
	CostMultiplier float64   `gorm:"type:numeric(5,2);default:1.0" json:"cost_multiplier"`
	LogType        string    `gorm:"index;size:16;default:'traffic'" json:"log_type"` // traffic/deposit/purchase/refund/subscription/fuse/downgrade
	CreatedAt      time.Time `gorm:"index;autoCreateTime" json:"created_at"`
}

// TableName 指定表名
func (BillingLog) TableName() string {
	return "billing_logs"
}

// ThreatIntel 威胁情报库
type ThreatIntel struct {
	ID             uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	GatewayID      string    `gorm:"index;size:32" json:"gateway_id"`
	SrcIP          string    `gorm:"index;size:45;not null" json:"src_ip"`
	SrcPort        int       `json:"src_port"`
	ThreatType     int       `gorm:"index;not null;check:threat_type BETWEEN 0 AND 10" json:"threat_type"`
	Severity       int       `gorm:"default:5;check:severity BETWEEN 0 AND 10" json:"severity"`
	JA4Fingerprint string    `gorm:"index;size:64" json:"ja4_fingerprint"`
	PacketCount    int       `gorm:"default:1" json:"packet_count"`
	HitCount       int64     `gorm:"default:1" json:"hit_count"`
	FirstSeen      time.Time `gorm:"autoCreateTime" json:"first_seen"`
	LastSeen       time.Time `gorm:"index;autoUpdateTime" json:"last_seen"`
}

// TableName 指定表名
func (ThreatIntel) TableName() string {
	return "threat_intel"
}

// ============================================
// 充值与购买记录
// ============================================

// Deposit Monero 充值记录
type Deposit struct {
	ID            uint64     `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID        string     `gorm:"index;size:64;not null" json:"user_id"`
	TxHash        string     `gorm:"uniqueIndex;size:64;not null" json:"tx_hash"`
	AmountXMR     float64    `gorm:"type:numeric(20,12);not null;check:amount_xmr > 0" json:"amount_xmr"`
	AmountUSD     float64    `gorm:"type:numeric(20,2);not null" json:"amount_usd"`
	ExchangeRate  float64    `gorm:"type:numeric(20,8);not null" json:"exchange_rate"`
	Status        string     `gorm:"index;size:16;default:'pending';check:status IN ('pending','confirmed','failed')" json:"status"`
	Confirmations int        `gorm:"default:0" json:"confirmations"`
	ConfirmedAt   *time.Time `json:"confirmed_at"`
	CreatedAt     time.Time  `gorm:"index;autoCreateTime" json:"created_at"`
}

// TableName 指定表名
func (Deposit) TableName() string {
	return "deposits"
}

// QuotaPurchase 流量包购买记录
type QuotaPurchase struct {
	ID          uint64     `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID      string     `gorm:"index;size:64;not null" json:"user_id"`
	PackageType string     `gorm:"size:32;not null" json:"package_type"` // 10GB/50GB/100GB/500GB/1TB/plan_*_monthly
	QuotaBytes  int64      `gorm:"not null;check:quota_bytes >= 0" json:"quota_bytes"`
	CostUSD     float64    `gorm:"type:numeric(20,2);not null;check:cost_usd > 0" json:"cost_usd"`
	CellLevel   int        `gorm:"default:1" json:"cell_level"`
	ExpiresAt   *time.Time `json:"expires_at"`
	CreatedAt   time.Time  `gorm:"index;autoCreateTime" json:"created_at"`
}

// TableName 指定表名
func (QuotaPurchase) TableName() string {
	return "quota_purchases"
}

// ============================================
// 邀请码管理
// ============================================

// Invitation 邀请码记录
type Invitation struct {
	ID        uint64     `gorm:"primaryKey;autoIncrement" json:"id"`
	Code      string     `gorm:"uniqueIndex;size:32;not null" json:"code"`
	CreatedBy string     `gorm:"index;size:64;not null" json:"created_by"` // 创建者 UserID
	UsedBy    string     `gorm:"index;size:64" json:"used_by"`             // 使用者 UserID
	Status    string     `gorm:"index;size:16;default:'unused';check:status IN ('unused','used','expired')" json:"status"`
	ExpiresAt time.Time  `gorm:"index" json:"expires_at"`
	UsedAt    *time.Time `json:"used_at"`
	CreatedAt time.Time  `gorm:"autoCreateTime" json:"created_at"`
}

// TableName 指定表名
func (Invitation) TableName() string {
	return "invitations"
}

// ============================================
// 认证挑战记录
// ============================================

// AuthChallenge 登录挑战（Redis 备份）
type AuthChallenge struct {
	ID          uint64     `gorm:"primaryKey;autoIncrement" json:"id"`
	ChallengeID string     `gorm:"uniqueIndex;size:64;not null" json:"challenge_id"`
	UserID      string     `gorm:"index;size:64;not null" json:"user_id"`
	Challenge   string     `gorm:"size:256;not null" json:"challenge"` // 挑战字符串
	Salt        string     `gorm:"size:64;not null" json:"salt"`       // 随机盐
	IPAddress   string     `gorm:"size:45" json:"ip_address"`
	Status      string     `gorm:"index;size:16;default:'pending';check:status IN ('pending','verified','expired','failed')" json:"status"`
	ExpiresAt   time.Time  `gorm:"index" json:"expires_at"`
	VerifiedAt  *time.Time `json:"verified_at"`
	CreatedAt   time.Time  `gorm:"autoCreateTime" json:"created_at"`
}

// TableName 指定表名
func (AuthChallenge) TableName() string {
	return "auth_challenges"
}

// ============================================
// 数据库初始化
// ============================================

// AutoMigrate 自动迁移所有表
func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&User{},
		&Cell{},
		&Gateway{},
		&BillingLog{},
		&ThreatIntel{},
		&Deposit{},
		&QuotaPurchase{},
		&Invitation{},
		&AuthChallenge{},
		// V2 三层状态模型
		&V2LinkState{},
		&V2SessionState{},
		&V2ControlState{},
		// V2 Persona Engine
		&PersonaManifest{},
		// V2 Commit Engine
		&CommitTransaction{},
		// V2 Budget Engine
		&BudgetProfile{},
		// V2 Observability & Audit
		&V2AuditRecord{},
		&V2SessionTimeline{},
		&V2LinkHealthTimeline{},
		&V2PersonaVersionTimeline{},
		&V2SurvivalModeTimeline{},
		&V2TransactionTimeline{},
	)
}
