// Package ebpf - B-DNA 模板更新器（Go → C 数据面桥接）
// 通过 eBPF Map 向 dna_template_map 写入微调后的模板参数
package ebpf

import (
	"fmt"
	"log"
)

// DNATemplateEntry 对应 C 结构体 dna_template（严格字节对齐）
type DNATemplateEntry struct {
	TargetIATMu     uint32 // 目标间隔均值（微秒）
	TargetIATSigma  uint32 // 目标间隔标准差（微秒）
	PaddingStrategy uint32 // 0:固定, 1:正态分布, 2:跟随载荷
	TargetMTU       uint16 // 模拟特定 MTU
	Reserved        uint16 // 对齐
	BurstSize       uint32 // 突发包数量
	BurstInterval   uint32 // 突发间隔（微秒）
}

// DNAMapUpdater 通过 eBPF Map 更新 B-DNA 模板
type DNAMapUpdater struct {
	loader *Loader
}

// NewDNAMapUpdater 创建 DNA 模板更新器
func NewDNAMapUpdater(loader *Loader) *DNAMapUpdater {
	return &DNAMapUpdater{loader: loader}
}

// UpdateDNATemplate 写入模板到 dna_template_map
func (d *DNAMapUpdater) UpdateDNATemplate(templateID uint32, iatMeanUs, iatSigmaUs, paddingStrategy uint32, targetMTU uint16) error {
	m := d.loader.GetMap("dna_template_map")
	if m == nil {
		return fmt.Errorf("dna_template_map 不存在")
	}

	entry := &DNATemplateEntry{
		TargetIATMu:     iatMeanUs,
		TargetIATSigma:  iatSigmaUs,
		PaddingStrategy: paddingStrategy,
		TargetMTU:       targetMTU,
		BurstSize:       4,     // 默认突发 4 包
		BurstInterval:   10000, // 默认 10ms 突发间隔
	}

	if err := m.Put(&templateID, entry); err != nil {
		return fmt.Errorf("写入 dna_template_map[%d] 失败: %w", templateID, err)
	}

	log.Printf("[DNAUpdater] 模板 %d 已更新: IAT=%dμs±%dμs, padding=%d, MTU=%d",
		templateID, iatMeanUs, iatSigmaUs, paddingStrategy, targetMTU)
	return nil
}
