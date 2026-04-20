// Package ebpf - 战术策略同步器
// 将 Raft 广播的战术模式同步到 eBPF Map
package ebpf

import (
	"encoding/binary"
	"fmt"
	"log"
	"time"
)

// TacticalMode 战术模式
type TacticalMode uint32

const (
	TacticalNormal     TacticalMode = 0
	TacticalSleep      TacticalMode = 1
	TacticalAggressive TacticalMode = 2
	TacticalStealth    TacticalMode = 3
)

// GlobalPolicy 全局策略（与 C 结构体对齐）
type GlobalPolicy struct {
	TacticalMode    uint32
	SocialJitter    uint32
	CIDRotationRate uint32
	FECRedundancy   uint32
	StealthFilter   uint32
	Timestamp       uint64
}

// TacticalSyncer 战术同步器
type TacticalSyncer struct {
	loader      *Loader
	currentMode TacticalMode
}

// NewTacticalSyncer 创建同步器
func NewTacticalSyncer(loader *Loader) *TacticalSyncer {
	return &TacticalSyncer{
		loader:      loader,
		currentMode: TacticalNormal,
	}
}

// UpdateGlobalPolicy 更新全局策略到 eBPF Map
// 写入失败时不更新内存状态，并尝试写入 emergency_ctrl_map 触发降级
func (ts *TacticalSyncer) UpdateGlobalPolicy(mode TacticalMode) error {
	policy := ts.getConfigForMode(mode)
	policy.Timestamp = uint64(time.Now().UnixNano())

	// 序列化为字节
	data := make([]byte, 32)
	binary.LittleEndian.PutUint32(data[0:4], policy.TacticalMode)
	binary.LittleEndian.PutUint32(data[4:8], policy.SocialJitter)
	binary.LittleEndian.PutUint32(data[8:12], policy.CIDRotationRate)
	binary.LittleEndian.PutUint32(data[12:16], policy.FECRedundancy)
	binary.LittleEndian.PutUint32(data[16:20], policy.StealthFilter)
	binary.LittleEndian.PutUint64(data[24:32], policy.Timestamp)

	// 写入 eBPF global_policy_map
	if ts.loader != nil {
		policyMap := ts.loader.GetMap("global_policy_map")
		if policyMap != nil {
			key := uint32(0)
			if err := policyMap.Put(&key, data); err != nil {
				log.Printf("🚨 [TacticalSyncer] 写入 global_policy_map 失败: %v（Raft 已 commit 但内核未同步）", err)
				// 尝试写入紧急降级标志
				ts.triggerEmergencyOnWriteFailure()
				return fmt.Errorf("写入 global_policy_map 失败: %w", err)
			}
			log.Printf("[TacticalSyncer] 已同步战术模式: %d → global_policy_map", mode)
		}
	}

	ts.currentMode = mode
	return nil
}

// triggerEmergencyOnWriteFailure Map 写入失败时尝试触发紧急降级
func (ts *TacticalSyncer) triggerEmergencyOnWriteFailure() {
	if ts.loader == nil {
		return
	}
	emergencyMap := ts.loader.GetMap("emergency_ctrl_map")
	if emergencyMap == nil {
		return
	}
	key := uint32(0)
	value := uint32(1) // 降级模式
	if err := emergencyMap.Put(&key, &value); err != nil {
		log.Printf("🚨 [TacticalSyncer] emergency_ctrl_map 写入也失败: %v", err)
	}
}

// SetGhostMode 设置 Ghost Mode
func (ts *TacticalSyncer) SetGhostMode(enabled bool) error {
	value := uint32(0)
	if enabled {
		value = 1
	}

	if ts.loader != nil {
		ghostMap := ts.loader.GetMap("ghost_mode_map")
		if ghostMap != nil {
			key := uint32(0)
			if err := ghostMap.Put(&key, &value); err != nil {
				return fmt.Errorf("写入 ghost_mode_map 失败: %w", err)
			}
		}
	}

	log.Printf("[TacticalSyncer] Ghost Mode: %v", enabled)
	return nil
}

// getConfigForMode 获取模式配置
func (ts *TacticalSyncer) getConfigForMode(mode TacticalMode) GlobalPolicy {
	switch mode {
	case TacticalSleep:
		return GlobalPolicy{
			TacticalMode:    uint32(TacticalSleep),
			SocialJitter:    10,
			CIDRotationRate: 1,
			FECRedundancy:   10,
			StealthFilter:   0,
		}
	case TacticalAggressive:
		return GlobalPolicy{
			TacticalMode:    uint32(TacticalAggressive),
			SocialJitter:    90,
			CIDRotationRate: 25,
			FECRedundancy:   45,
			StealthFilter:   0,
		}
	case TacticalStealth:
		return GlobalPolicy{
			TacticalMode:    uint32(TacticalStealth),
			SocialJitter:    70,
			CIDRotationRate: 20,
			FECRedundancy:   35,
			StealthFilter:   9,
		}
	default:
		return GlobalPolicy{
			TacticalMode:    uint32(TacticalNormal),
			SocialJitter:    50,
			CIDRotationRate: 5,
			FECRedundancy:   20,
			StealthFilter:   0,
		}
	}
}

// GetCurrentMode 获取当前模式
func (ts *TacticalSyncer) GetCurrentMode() TacticalMode {
	return ts.currentMode
}
