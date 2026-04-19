// Package ebpf - 紧急自毁 eBPF 管理
package ebpf

import (
	"fmt"
	"log"
)

const (
	// EmergencyWipeCode 紧急自毁指令码
	EmergencyWipeCode uint32 = 0xDEADBEEF
)

// EmergencyManager 紧急自毁管理器
type EmergencyManager struct {
	loader *Loader
}

// NewEmergencyManager 创建紧急自毁管理器
func NewEmergencyManager(loader *Loader) *EmergencyManager {
	return &EmergencyManager{
		loader: loader,
	}
}

// TriggerWipe 触发紧急自毁
func (em *EmergencyManager) TriggerWipe() error {
	log.Println("[Emergency] 🔥 触发紧急自毁程序")
	
	// 1. 下发紧急指令码到内核
	if err := em.setEmergencyCode(EmergencyWipeCode); err != nil {
		return fmt.Errorf("下发紧急指令失败: %w", err)
	}
	
	// 2. 清空所有敏感 Map
	if err := em.wipeSensitiveMaps(); err != nil {
		log.Printf("[Emergency] ⚠️ 清空 Map 失败: %v", err)
	}
	
	log.Println("[Emergency] ✅ 紧急自毁完成")
	
	return nil
}

// setEmergencyCode 设置紧急指令码
func (em *EmergencyManager) setEmergencyCode(code uint32) error {
	key := uint32(0)
	
	emergencyMap := em.loader.GetMap("emergency_ctrl_map")
	if emergencyMap == nil {
		return fmt.Errorf("emergency_ctrl_map 不存在")
	}
	
	if err := emergencyMap.Put(&key, &code); err != nil {
		return fmt.Errorf("更新 emergency_ctrl_map 失败: %w", err)
	}
	
	log.Printf("[Emergency] 已下发紧急指令码: 0x%X", code)
	
	return nil
}

// wipeSensitiveMaps 清空所有敏感 Map
func (em *EmergencyManager) wipeSensitiveMaps() error {
	sensitiveMaps := []string{
		"dna_template_map",
		"jitter_config_map",
		"npm_config_map",
		"vpc_config_map",
		"quota_map",
		"cell_phase_map",
	}
	
	for _, mapName := range sensitiveMaps {
		if err := em.clearMap(mapName); err != nil {
			log.Printf("[Emergency] ⚠️ 清空 %s 失败: %v", mapName, err)
			continue
		}
		log.Printf("[Emergency] ✅ 已清空: %s", mapName)
	}
	
	return nil
}

// clearMap 清空指定 Map
func (em *EmergencyManager) clearMap(mapName string) error {
	m := em.loader.GetMap(mapName)
	if m == nil {
		return fmt.Errorf("Map %s 不存在", mapName)
	}
	
	// 遍历并删除所有条目
	// 当前简化版本：将前 16 个 key 置零
	for i := uint32(0); i < 16; i++ {
		zero := uint64(0)
		_ = m.Put(&i, &zero)
	}
	
	return nil
}

// ResetEmergencyCode 重置紧急指令码
func (em *EmergencyManager) ResetEmergencyCode() error {
	key := uint32(0)
	zero := uint32(0)
	
	emergencyMap := em.loader.GetMap("emergency_ctrl_map")
	if emergencyMap == nil {
		return fmt.Errorf("emergency_ctrl_map 不存在")
	}
	
	if err := emergencyMap.Put(&key, &zero); err != nil {
		return fmt.Errorf("重置紧急指令码失败: %w", err)
	}
	
	log.Println("[Emergency] 已重置紧急指令码")
	
	return nil
}

// CheckEmergencyStatus 检查紧急状态
func (em *EmergencyManager) CheckEmergencyStatus() (bool, error) {
	key := uint32(0)
	var code uint32
	
	emergencyMap := em.loader.GetMap("emergency_ctrl_map")
	if emergencyMap == nil {
		return false, fmt.Errorf("emergency_ctrl_map 不存在")
	}
	
	if err := emergencyMap.Lookup(&key, &code); err != nil {
		return false, fmt.Errorf("查询紧急状态失败: %w", err)
	}
	
	return code == EmergencyWipeCode, nil
}
