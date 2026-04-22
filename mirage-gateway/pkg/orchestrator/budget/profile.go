package budget

import (
	"time"

	"github.com/google/uuid"
)

// BudgetProfile 预算配置对象
type BudgetProfile struct {
	ProfileID             string    `json:"profile_id" gorm:"primaryKey;size:64"`
	SessionID             string    `json:"session_id" gorm:"uniqueIndex;size:64"` // 空字符串表示全局
	LatencyBudgetMs       int64     `json:"latency_budget_ms" gorm:"not null"`
	BandwidthBudgetRatio  float64   `json:"bandwidth_budget_ratio" gorm:"type:numeric(5,4);not null"`
	SwitchBudgetPerHour   int       `json:"switch_budget_per_hour" gorm:"not null"`
	EntryBurnBudgetPerDay int       `json:"entry_burn_budget_per_day" gorm:"not null"`
	GatewayLoadBudget     float64   `json:"gateway_load_budget" gorm:"type:numeric(5,4);not null"`
	HardenedAllowed       bool      `json:"hardened_allowed" gorm:"not null;default:false"`
	EscapeAllowed         bool      `json:"escape_allowed" gorm:"not null;default:false"`
	LastResortAllowed     bool      `json:"last_resort_allowed" gorm:"not null;default:false"`
	CreatedAt             time.Time `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt             time.Time `json:"updated_at" gorm:"autoUpdateTime"`
}

// TableName GORM 表名
func (BudgetProfile) TableName() string { return "budget_profiles" }

// Validate 校验所有数值字段在合法范围内
func (bp *BudgetProfile) Validate() error {
	if bp.LatencyBudgetMs <= 0 {
		return &ErrInvalidBudgetProfile{Field: "latency_budget_ms", Message: "must be > 0"}
	}
	if bp.BandwidthBudgetRatio < 0.0 || bp.BandwidthBudgetRatio > 1.0 {
		return &ErrInvalidBudgetProfile{Field: "bandwidth_budget_ratio", Message: "must be in [0.0, 1.0]"}
	}
	if bp.SwitchBudgetPerHour < 0 {
		return &ErrInvalidBudgetProfile{Field: "switch_budget_per_hour", Message: "must be >= 0"}
	}
	if bp.EntryBurnBudgetPerDay < 0 {
		return &ErrInvalidBudgetProfile{Field: "entry_burn_budget_per_day", Message: "must be >= 0"}
	}
	if bp.GatewayLoadBudget < 0.0 || bp.GatewayLoadBudget > 1.0 {
		return &ErrInvalidBudgetProfile{Field: "gateway_load_budget", Message: "must be in [0.0, 1.0]"}
	}
	return nil
}

// DefaultBudgetProfile 返回 Standard 等级默认预算配置
func DefaultBudgetProfile() *BudgetProfile {
	return &BudgetProfile{
		ProfileID:             uuid.New().String(),
		SessionID:             "",
		LatencyBudgetMs:       200,
		BandwidthBudgetRatio:  0.5,
		SwitchBudgetPerHour:   5,
		EntryBurnBudgetPerDay: 2,
		GatewayLoadBudget:     0.7,
		HardenedAllowed:       false,
		EscapeAllowed:         false,
		LastResortAllowed:     false,
	}
}
