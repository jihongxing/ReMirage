package ebpf

import (
	"fmt"
	"log"
)

// VerifyGaussianMode 启动时从 eBPF Map 读取完整 NPMConfig 结构体，
// 检查 PaddingMode 字段是否为 Gaussian，非 Gaussian 时记录错误、修正后写回。
func (da *DefenseApplier) VerifyGaussianMode() error {
	npmMap := da.loader.GetMap("npm_config_map")
	if npmMap == nil {
		return fmt.Errorf("npm_config_map 不存在")
	}

	key := uint32(0)
	var cfg NPMConfig
	if err := npmMap.Lookup(&key, &cfg); err != nil {
		return fmt.Errorf("读取 npm_config_map 失败: %w", err)
	}

	if cfg.PaddingMode == NPMModeGaussian {
		log.Println("[NPM] ✅ PaddingMode 已为 Gaussian")
		return nil
	}

	log.Printf("[NPM] ⚠️ PaddingMode=%d 非 Gaussian(%d)，强制修正",
		cfg.PaddingMode, NPMModeGaussian)

	cfg.PaddingMode = NPMModeGaussian
	if err := npmMap.Put(&key, &cfg); err != nil {
		return fmt.Errorf("写回 npm_config_map 失败: %w", err)
	}

	log.Println("[NPM] ✅ PaddingMode 已修正为 Gaussian")
	return nil
}

// VerifyGaussianModeWithMap 接受 MapAccessor 接口的版本，便于测试
func VerifyGaussianModeWithMap(m MapAccessor) error {
	key := uint32(0)
	var cfg NPMConfig
	if err := m.Lookup(&key, &cfg); err != nil {
		return fmt.Errorf("读取 npm_config_map 失败: %w", err)
	}

	if cfg.PaddingMode == NPMModeGaussian {
		return nil
	}

	cfg.PaddingMode = NPMModeGaussian
	if err := m.Put(&key, &cfg); err != nil {
		return fmt.Errorf("写回 npm_config_map 失败: %w", err)
	}

	return nil
}

// MapAccessor eBPF Map 读写接口（便于测试）
type MapAccessor interface {
	Lookup(key, valueOut interface{}) error
	Put(key, value interface{}) error
}
