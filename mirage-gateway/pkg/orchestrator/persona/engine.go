// Package persona - PersonaEngine 核心实现（切换/回滚/查询）
package persona

import (
	"context"
	"fmt"
	"mirage-gateway/pkg/ebpf"
	"sync"
)

// SessionStore Session 状态存储接口
type SessionStore interface {
	GetCurrentPersonaID(sessionID string) (string, error)
	SetCurrentPersonaID(sessionID string, personaID string) error
}

// ControlStore Control 状态存储接口
type ControlStore interface {
	GetEpoch() uint64
	GetPersonaVersion() uint64
	SetPersonaVersion(version uint64) error
}

// ManifestStore Manifest 持久化存储接口
type ManifestStore interface {
	VersionStore
	GetByPersonaIDAndVersion(personaID string, version uint64) (*PersonaManifest, error)
	GetLatest(personaID string) (*PersonaManifest, error)
	ListVersions(personaID string) ([]*PersonaManifest, error)
	GetActiveBySession(sessionID string) (*PersonaManifest, error)
	FindCoolingBySession(sessionID string) (*PersonaManifest, error)
	UpdateLifecycle(personaID string, version uint64, lifecycle PersonaLifecycle) error
}

// Engine PersonaEngine 核心实现
type Engine struct {
	mu            sync.Mutex
	mapUpdater    ebpf.PersonaMapUpdater
	manifestStore ManifestStore
	sessionStore  SessionStore
	controlStore  ControlStore
	// slotMapping 记录 persona_id -> slot 映射
	slotMapping map[string]uint32
}

// NewEngine 创建 PersonaEngine
func NewEngine(
	mapUpdater ebpf.PersonaMapUpdater,
	manifestStore ManifestStore,
	sessionStore SessionStore,
	controlStore ControlStore,
) *Engine {
	return &Engine{
		mapUpdater:    mapUpdater,
		manifestStore: manifestStore,
		sessionStore:  sessionStore,
		controlStore:  controlStore,
		slotMapping:   make(map[string]uint32),
	}
}

// CreateManifest 创建 Manifest：校验 → 计算 checksum → 版本检查 → epoch 对齐 → 持久化
func (e *Engine) CreateManifest(_ context.Context, m *PersonaManifest) error {
	ep := &controlEpochProvider{store: e.controlStore}
	return CreateManifestWithVersion(m, e.manifestStore, ep)
}

// SwitchPersona 原子切换
func (e *Engine) SwitchPersona(_ context.Context, sessionID string, newManifest *PersonaManifest, params *ebpf.PersonaParams) error {
	if !e.mu.TryLock() {
		return ErrSwitchInProgress
	}
	defer e.mu.Unlock()

	// 1. 校验
	if err := ValidateManifest(newManifest); err != nil {
		return err
	}

	// 2. 记录切换前状态
	prevPersonaID, _ := e.sessionStore.GetCurrentPersonaID(sessionID)
	prevVersion := e.controlStore.GetPersonaVersion()
	prevSlot, _ := e.mapUpdater.GetActiveSlot()

	// 3. WriteShadow
	shadowSlot, err := e.mapUpdater.WriteShadow(params)
	if err != nil {
		return err
	}

	// 4. VerifyShadow
	if err := e.mapUpdater.VerifyShadow(shadowSlot, params); err != nil {
		return err
	}

	// 5. Atomic Flip
	if err := e.mapUpdater.Flip(shadowSlot); err != nil {
		// 回滚 Flip 失败：恢复 active_slot
		_ = e.mapUpdater.Flip(prevSlot)
		return ErrFlipFailed
	}

	// 6. 更新 Session 和 Control
	if err := e.sessionStore.SetCurrentPersonaID(sessionID, newManifest.PersonaID); err != nil {
		// 回滚
		_ = e.mapUpdater.Flip(prevSlot)
		_ = e.sessionStore.SetCurrentPersonaID(sessionID, prevPersonaID)
		return fmt.Errorf("update session failed: %w", err)
	}

	if err := e.controlStore.SetPersonaVersion(newManifest.Version); err != nil {
		// 回滚
		_ = e.mapUpdater.Flip(prevSlot)
		_ = e.sessionStore.SetCurrentPersonaID(sessionID, prevPersonaID)
		_ = e.controlStore.SetPersonaVersion(prevVersion)
		return fmt.Errorf("update control failed: %w", err)
	}

	// 7. 先将已有 Cooling → Retired（保证最多一个 Cooling）
	existingCooling, _ := e.manifestStore.FindCoolingBySession(sessionID)
	if existingCooling != nil {
		_ = e.manifestStore.UpdateLifecycle(existingCooling.PersonaID, existingCooling.Version, LifecycleRetired)
	}

	// 8. 旧 Active → Cooling
	if prevPersonaID != "" {
		oldManifest, err := e.manifestStore.GetActiveBySession(sessionID)
		if err == nil && oldManifest != nil && oldManifest.PersonaID != newManifest.PersonaID {
			_ = e.manifestStore.UpdateLifecycle(oldManifest.PersonaID, oldManifest.Version, LifecycleCooling)
		}
	}

	// 8. 新 Manifest → Active
	_ = e.manifestStore.UpdateLifecycle(newManifest.PersonaID, newManifest.Version, LifecycleActive)

	// 记录 slot 映射
	e.slotMapping[newManifest.PersonaID] = shadowSlot
	if prevPersonaID != "" {
		e.slotMapping[prevPersonaID] = prevSlot
	}

	return nil
}

type controlEpochProvider struct {
	store ControlStore
}

func (p *controlEpochProvider) CurrentEpoch() uint64 {
	return p.store.GetEpoch()
}

// Rollback 回滚到 Cooling 版本
func (e *Engine) Rollback(_ context.Context, sessionID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	// 查找 Cooling Manifest
	coolingManifest, err := e.manifestStore.FindCoolingBySession(sessionID)
	if err != nil || coolingManifest == nil {
		return ErrNoCoolingTarget
	}

	// 获取 Cooling Persona 所在 Slot
	coolingSlot, ok := e.slotMapping[coolingManifest.PersonaID]
	if !ok {
		// 如果没有映射，Cooling 在非活跃 Slot
		active, _ := e.mapUpdater.GetActiveSlot()
		coolingSlot = 1 - active
	}

	// Flip 到 Cooling Slot
	if err := e.mapUpdater.Flip(coolingSlot); err != nil {
		return ErrFlipFailed
	}

	// 更新 Session
	if err := e.sessionStore.SetCurrentPersonaID(sessionID, coolingManifest.PersonaID); err != nil {
		return fmt.Errorf("rollback update session failed: %w", err)
	}

	// 当前 Active → Retired
	currentActive, _ := e.manifestStore.GetActiveBySession(sessionID)
	if currentActive != nil && currentActive.PersonaID != coolingManifest.PersonaID {
		_ = e.manifestStore.UpdateLifecycle(currentActive.PersonaID, currentActive.Version, LifecycleRetired)
	}

	// Cooling → Active
	_ = e.manifestStore.UpdateLifecycle(coolingManifest.PersonaID, coolingManifest.Version, LifecycleActive)

	return nil
}

// GetLatest 按 persona_id 查询最新版本
func (e *Engine) GetLatest(_ context.Context, personaID string) (*PersonaManifest, error) {
	return e.manifestStore.GetLatest(personaID)
}

// ListVersions 按 persona_id 查询全部版本，version 降序
func (e *Engine) ListVersions(_ context.Context, personaID string) ([]*PersonaManifest, error) {
	return e.manifestStore.ListVersions(personaID)
}

// GetActiveBySession 按 session_id 查询当前 Active Persona
func (e *Engine) GetActiveBySession(_ context.Context, sessionID string) (*PersonaManifest, error) {
	return e.manifestStore.GetActiveBySession(sessionID)
}
