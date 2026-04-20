//go:build !linux

// Package ebpf - 非 Linux 平台的 Loader 桩实现
// 提供类型定义和空方法，使依赖 Loader 的代码在 Windows/macOS 上可编译
// 实际 eBPF 功能仅在 Linux 上可用
package ebpf

import (
	"fmt"
	"log"

	ciliumebpf "github.com/cilium/ebpf"
)

// Loader eBPF 加载器（非 Linux 平台桩实现）
type Loader struct {
	iface string
	maps  map[string]*ciliumebpf.Map
}

// NewLoader 创建加载器（非 Linux 桩）
func NewLoader(iface string) *Loader {
	return &Loader{
		iface: iface,
		maps:  make(map[string]*ciliumebpf.Map),
	}
}

// LoadAndAttach 加载并挂载（非 Linux 桩：直接返回错误）
func (l *Loader) LoadAndAttach() error {
	return fmt.Errorf("eBPF 仅支持 Linux 平台")
}

// GetMap 获取 Map（非 Linux 桩：返回 nil）
func (l *Loader) GetMap(name string) *ciliumebpf.Map {
	return nil
}

// UpdateStrategy 更新策略（非 Linux 桩）
func (l *Loader) UpdateStrategy(strategy *DefenseStrategy) error {
	log.Println("[eBPF-Stub] UpdateStrategy: 非 Linux 平台，跳过")
	return nil
}

// Close 关闭（非 Linux 桩）
func (l *Loader) Close() error {
	return nil
}

// GetSockMap 桩
func (l *Loader) GetSockMap() *ciliumebpf.Map { return nil }

// GetProxyMap 桩
func (l *Loader) GetProxyMap() *ciliumebpf.Map { return nil }

// GetConnStateMap 桩
func (l *Loader) GetConnStateMap() *ciliumebpf.Map { return nil }

// GetSockmapStats 桩
func (l *Loader) GetSockmapStats() (map[string]uint64, error) {
	return nil, fmt.Errorf("eBPF 仅支持 Linux 平台")
}

// StartMonitoring 桩
func (l *Loader) StartMonitoring(handler ThreatEventHandler) error {
	return fmt.Errorf("eBPF 监控仅支持 Linux 平台")
}
