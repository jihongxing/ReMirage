// Package models - PersonaManifest GORM 模型（mirage-os 侧）
package models

import "time"

// Persona 生命周期枚举常量
const (
	PersonaLifecyclePrepared     = "Prepared"
	PersonaLifecycleShadowLoaded = "ShadowLoaded"
	PersonaLifecycleActive       = "Active"
	PersonaLifecycleCooling      = "Cooling"
	PersonaLifecycleRetired      = "Retired"
)

// PersonaManifest 不可拆分的统一画像快照
type PersonaManifest struct {
	PersonaID            string    `gorm:"column:persona_id;size:64;not null;primaryKey" json:"persona_id"`
	Version              uint64    `gorm:"column:version;not null;primaryKey" json:"version"`
	Epoch                uint64    `gorm:"column:epoch;index;not null" json:"epoch"`
	Checksum             string    `gorm:"column:checksum;size:64;not null" json:"checksum"`
	HandshakeProfileID   string    `gorm:"column:handshake_profile_id;size:64;not null" json:"handshake_profile_id"`
	PacketShapeProfileID string    `gorm:"column:packet_shape_profile_id;size:64;not null" json:"packet_shape_profile_id"`
	TimingProfileID      string    `gorm:"column:timing_profile_id;size:64;not null" json:"timing_profile_id"`
	BackgroundProfileID  string    `gorm:"column:background_profile_id;size:64;not null" json:"background_profile_id"`
	MTUProfileID         string    `gorm:"column:mtu_profile_id;size:64;default:''" json:"mtu_profile_id"`
	FECProfileID         string    `gorm:"column:fec_profile_id;size:64;default:''" json:"fec_profile_id"`
	LifecyclePolicyID    string    `gorm:"column:lifecycle_policy_id;size:64;default:''" json:"lifecycle_policy_id"`
	Lifecycle            string    `gorm:"column:lifecycle;size:16;not null;check:lifecycle IN ('Prepared','ShadowLoaded','Active','Cooling','Retired')" json:"lifecycle"`
	CreatedAt            time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
}

// TableName 指定表名
func (PersonaManifest) TableName() string { return "persona_manifests" }
