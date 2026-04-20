// Package ebpf - CellLifecycleManager eBPF 适配器
// 实现 strategy 包定义的接口，通过 eBPF Map 桥接 Go 控制面到 C 数据面
package ebpf

import (
	"fmt"
	"log"
)

// PhaseMapUpdater 实现 strategy.EBPFPhaseUpdater 接口
type PhaseMapUpdater struct {
	loader *Loader
}

func NewPhaseMapUpdater(loader *Loader) *PhaseMapUpdater {
	return &PhaseMapUpdater{loader: loader}
}

func (p *PhaseMapUpdater) UpdatePhaseMap(phase uint32) error {
	m := p.loader.GetMap("cell_phase_map")
	if m == nil {
		return fmt.Errorf("cell_phase_map 不存在")
	}
	key := uint32(0)
	if err := m.Put(&key, &phase); err != nil {
		return fmt.Errorf("写入 cell_phase_map 失败: %w", err)
	}
	log.Printf("[PhaseUpdater] cell_phase_map 已更新: %d", phase)
	return nil
}
