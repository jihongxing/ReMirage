// Package persona - V2 Persona Manifest 结构体与生命周期定义
package persona

import (
	"crypto/sha256"
	"fmt"
	"time"
)

// PersonaLifecycle 画像生命周期枚举
type PersonaLifecycle string

const (
	LifecyclePrepared     PersonaLifecycle = "Prepared"
	LifecycleShadowLoaded PersonaLifecycle = "ShadowLoaded"
	LifecycleActive       PersonaLifecycle = "Active"
	LifecycleCooling      PersonaLifecycle = "Cooling"
	LifecycleRetired      PersonaLifecycle = "Retired"
)

// AllLifecycles 所有合法 PersonaLifecycle 值
var AllLifecycles = []PersonaLifecycle{
	LifecyclePrepared, LifecycleShadowLoaded, LifecycleActive,
	LifecycleCooling, LifecycleRetired,
}

// PersonaManifest 不可拆分的统一画像快照
type PersonaManifest struct {
	PersonaID            string           `gorm:"size:64;not null;uniqueIndex:idx_persona_version" json:"persona_id"`
	Version              uint64           `gorm:"not null;uniqueIndex:idx_persona_version" json:"version"`
	Epoch                uint64           `gorm:"index;not null" json:"epoch"`
	Checksum             string           `gorm:"size:64;not null" json:"checksum"`
	HandshakeProfileID   string           `gorm:"size:64;not null" json:"handshake_profile_id"`
	PacketShapeProfileID string           `gorm:"size:64;not null" json:"packet_shape_profile_id"`
	TimingProfileID      string           `gorm:"size:64;not null" json:"timing_profile_id"`
	BackgroundProfileID  string           `gorm:"size:64;not null" json:"background_profile_id"`
	MTUProfileID         string           `gorm:"size:64;default:''" json:"mtu_profile_id"`
	FECProfileID         string           `gorm:"size:64;default:''" json:"fec_profile_id"`
	LifecyclePolicyID    string           `gorm:"size:64;default:''" json:"lifecycle_policy_id"`
	Lifecycle            PersonaLifecycle `gorm:"size:16;not null;check:lifecycle IN ('Prepared','ShadowLoaded','Active','Cooling','Retired')" json:"lifecycle"`
	CreatedAt            time.Time        `gorm:"autoCreateTime" json:"created_at"`
}

// validTransitions 合法生命周期转换表
var validTransitions = map[[2]PersonaLifecycle]bool{
	{LifecyclePrepared, LifecycleShadowLoaded}: true,
	{LifecycleShadowLoaded, LifecycleActive}:   true,
	{LifecycleShadowLoaded, LifecycleRetired}:  true,
	{LifecycleActive, LifecycleCooling}:        true,
	{LifecycleCooling, LifecycleRetired}:       true,
}

// ValidateManifest 校验四个必填 profile_id 非空
func ValidateManifest(m *PersonaManifest) error {
	if m.HandshakeProfileID == "" {
		return &ErrMissingProfile{FieldName: "handshake_profile_id"}
	}
	if m.PacketShapeProfileID == "" {
		return &ErrMissingProfile{FieldName: "packet_shape_profile_id"}
	}
	if m.TimingProfileID == "" {
		return &ErrMissingProfile{FieldName: "timing_profile_id"}
	}
	if m.BackgroundProfileID == "" {
		return &ErrMissingProfile{FieldName: "background_profile_id"}
	}
	return nil
}

// ComputeChecksum 基于六个 profile_id 拼接计算 SHA-256
func ComputeChecksum(m *PersonaManifest) string {
	data := m.HandshakeProfileID + "|" +
		m.PacketShapeProfileID + "|" +
		m.TimingProfileID + "|" +
		m.BackgroundProfileID + "|" +
		m.MTUProfileID + "|" +
		m.FECProfileID
	hash := sha256.Sum256([]byte(data))
	return fmt.Sprintf("%x", hash)
}

// TransitionLifecycle 校验并执行生命周期转换
func TransitionLifecycle(from, to PersonaLifecycle) error {
	if !validTransitions[[2]PersonaLifecycle{from, to}] {
		return &ErrInvalidLifecycleTransition{From: from, To: to}
	}
	return nil
}
