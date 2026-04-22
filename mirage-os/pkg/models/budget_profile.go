// Package models - BudgetProfile GORM 模型（mirage-os 侧）
package models

import "time"

// BudgetProfile 预算配置对象
type BudgetProfile struct {
	ProfileID             string    `gorm:"column:profile_id;primaryKey;size:64" json:"profile_id"`
	SessionID             string    `gorm:"column:session_id;uniqueIndex;size:64" json:"session_id"`
	LatencyBudgetMs       int64     `gorm:"column:latency_budget_ms;not null" json:"latency_budget_ms"`
	BandwidthBudgetRatio  float64   `gorm:"column:bandwidth_budget_ratio;type:numeric(5,4);not null" json:"bandwidth_budget_ratio"`
	SwitchBudgetPerHour   int       `gorm:"column:switch_budget_per_hour;not null" json:"switch_budget_per_hour"`
	EntryBurnBudgetPerDay int       `gorm:"column:entry_burn_budget_per_day;not null" json:"entry_burn_budget_per_day"`
	GatewayLoadBudget     float64   `gorm:"column:gateway_load_budget;type:numeric(5,4);not null" json:"gateway_load_budget"`
	HardenedAllowed       bool      `gorm:"column:hardened_allowed;not null;default:false" json:"hardened_allowed"`
	EscapeAllowed         bool      `gorm:"column:escape_allowed;not null;default:false" json:"escape_allowed"`
	LastResortAllowed     bool      `gorm:"column:last_resort_allowed;not null;default:false" json:"last_resort_allowed"`
	CreatedAt             time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt             time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

// TableName 指定表名
func (BudgetProfile) TableName() string { return "budget_profiles" }
