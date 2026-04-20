// Package ebpf - VPC 噪声注入器（Go → C 数据面桥接）
// 通过 eBPF Map 向 jitter.c 的 vpc_noise_profiles / active_noise_profile 写入配置
package ebpf

import (
	"encoding/binary"
	"fmt"
)

// VPCNoiseProfile 对应 C 结构体 vpc_noise_profile（严格字节对齐）
type VPCNoiseProfile struct {
	FiberBaseUs      uint32
	FiberVarianceUs  uint32
	RouterHops       uint32
	RouterQueueUs    uint32
	CongestionFactor uint32
	PacketLossRate   uint32
	ReorderRate      uint32
	DuplicateRate    uint32
}

// VPCNoiseStats 对应 C 结构体 vpc_noise_stats
type VPCNoiseStats struct {
	TotalPackets      uint64
	DelayedPackets    uint64
	TotalDelayUs      uint64
	DroppedPackets    uint64
	ReorderedPackets  uint64
	DuplicatedPackets uint64
}

// VPCInjector 通过 eBPF Map 控制 VPC 噪声数据面
type VPCInjector struct {
	loader *Loader
}

// NewVPCInjector 创建 VPC 噪声注入器
func NewVPCInjector(loader *Loader) *VPCInjector {
	return &VPCInjector{loader: loader}
}

// SetNoiseProfile 写入噪声配置到 vpc_noise_profiles Map
func (v *VPCInjector) SetNoiseProfile(regionID uint32, profile *VPCNoiseProfile) error {
	m := v.loader.GetMap("vpc_noise_profiles")
	if m == nil {
		return fmt.Errorf("vpc_noise_profiles map 不存在")
	}
	return m.Put(&regionID, profile)
}

// SetActiveProfile 激活指定区域配置
func (v *VPCInjector) SetActiveProfile(regionID uint32) error {
	m := v.loader.GetMap("active_noise_profile")
	if m == nil {
		return fmt.Errorf("active_noise_profile map 不存在")
	}
	key := uint32(0)
	return m.Put(&key, &regionID)
}

// GetNoiseStats 读取噪声统计（PERCPU_ARRAY 需要聚合）
func (v *VPCInjector) GetNoiseStats() (*VPCNoiseStats, error) {
	m := v.loader.GetMap("vpc_noise_stats")
	if m == nil {
		return nil, fmt.Errorf("vpc_noise_stats map 不存在")
	}

	key := uint32(0)
	// PERCPU_ARRAY 返回每个 CPU 的值，需要聚合
	var perCPU []byte
	if err := m.Lookup(&key, &perCPU); err != nil {
		// Fallback: 尝试直接读取单值
		var stats VPCNoiseStats
		if err2 := m.Lookup(&key, &stats); err2 != nil {
			return nil, fmt.Errorf("读取 vpc_noise_stats 失败: %w", err2)
		}
		return &stats, nil
	}

	// 聚合所有 CPU 的统计
	stats := &VPCNoiseStats{}
	entrySize := 48 // 6 * uint64
	for i := 0; i+entrySize <= len(perCPU); i += entrySize {
		stats.TotalPackets += binary.LittleEndian.Uint64(perCPU[i:])
		stats.DelayedPackets += binary.LittleEndian.Uint64(perCPU[i+8:])
		stats.TotalDelayUs += binary.LittleEndian.Uint64(perCPU[i+16:])
		stats.DroppedPackets += binary.LittleEndian.Uint64(perCPU[i+24:])
		stats.ReorderedPackets += binary.LittleEndian.Uint64(perCPU[i+32:])
		stats.DuplicatedPackets += binary.LittleEndian.Uint64(perCPU[i+40:])
	}

	return stats, nil
}
