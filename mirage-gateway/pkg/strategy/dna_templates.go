// Package strategy - B-DNA 拟态模板库（V2 生产版）
package strategy

// DNATemplate B-DNA 拟态模板
type DNATemplate struct {
	Name            string  // 模板名称
	TargetIATMu     uint32  // 目标间隔均值（微秒）
	TargetIATSigma  uint32  // 目标间隔标准差（微秒）
	PaddingStrategy uint32  // 填充策略：0=固定, 1=正态分布, 2=跟随载荷
	TargetMTU       uint16  // 模拟特定 MTU
	BurstSize       uint32  // 突发包数量
	BurstInterval   uint32  // 突发间隔（微秒）
	Description     string  // 描述
}

// 预设拟态模板库
var (
	// TemplateZoom Zoom 视频会议指纹
	// 特征：低延迟、高频率、小包为主、周期性突发
	TemplateZoom = DNATemplate{
		Name:            "Zoom-Conference",
		TargetIATMu:     20000,  // 20ms 均值（50 pps）
		TargetIATSigma:  5000,   // 5ms 标准差
		PaddingStrategy: 1,      // 正态分布填充
		TargetMTU:       1432,   // Zoom 特征 MTU
		BurstSize:       3,      // 每次突发 3 个包
		BurstInterval:   100000, // 100ms 突发间隔
		Description:     "模拟 Zoom 视频会议流量：低延迟、高频率、周期性突发",
	}

	// TemplateNetflix Netflix 视频流指纹
	// 特征：高吞吐、长尾分布、满载包为主、持续传输
	TemplateNetflix = DNATemplate{
		Name:            "Netflix-Streaming",
		TargetIATMu:     8000,   // 8ms 均值（125 pps）
		TargetIATSigma:  3000,   // 3ms 标准差
		PaddingStrategy: 2,      // 跟随载荷填充
		TargetMTU:       1500,   // 标准 MTU（满载包）
		BurstSize:       10,     // 每次突发 10 个包
		BurstInterval:   50000,  // 50ms 突发间隔
		Description:     "模拟 Netflix 视频流：高吞吐、满载包、持续传输",
	}

	// TemplateWhatsApp WhatsApp 即时通讯指纹
	// 特征：小包为主、间歇性传输、低频率
	TemplateWhatsApp = DNATemplate{
		Name:            "WhatsApp-Messaging",
		TargetIATMu:     100000, // 100ms 均值（10 pps）
		TargetIATSigma:  30000,  // 30ms 标准差
		PaddingStrategy: 0,      // 固定填充
		TargetMTU:       512,    // 小包
		BurstSize:       1,      // 单包传输
		BurstInterval:   200000, // 200ms 间隔
		Description:     "模拟 WhatsApp 即时通讯：小包、间歇性、低频率",
	}

	// TemplateSteam Steam 游戏下载指纹
	// 特征：极高吞吐、满载包、持续传输、无突发
	TemplateSteam = DNATemplate{
		Name:            "Steam-Download",
		TargetIATMu:     2000,   // 2ms 均值（500 pps）
		TargetIATSigma:  500,    // 0.5ms 标准差
		PaddingStrategy: 2,      // 跟随载荷填充
		TargetMTU:       1500,   // 标准 MTU
		BurstSize:       50,     // 大突发
		BurstInterval:   10000,  // 10ms 突发间隔
		Description:     "模拟 Steam 游戏下载：极高吞吐、满载包、持续传输",
	}

	// TemplateDiscord Discord 语音通话指纹
	// 特征：中等延迟、中等频率、固定包大小
	TemplateDiscord = DNATemplate{
		Name:            "Discord-Voice",
		TargetIATMu:     40000,  // 40ms 均值（25 pps）
		TargetIATSigma:  10000,  // 10ms 标准差
		PaddingStrategy: 0,      // 固定填充
		TargetMTU:       960,    // Discord 语音包大小
		BurstSize:       2,      // 双包传输
		BurstInterval:   80000,  // 80ms 间隔
		Description:     "模拟 Discord 语音通话：中等延迟、固定包大小",
	}

	// TemplateHTTPS 标准 HTTPS 浏览指纹
	// 特征：间歇性突发、长尾分布、混合包大小
	TemplateHTTPS = DNATemplate{
		Name:            "HTTPS-Browsing",
		TargetIATMu:     50000,  // 50ms 均值（20 pps）
		TargetIATSigma:  20000,  // 20ms 标准差
		PaddingStrategy: 1,      // 正态分布填充
		TargetMTU:       1400,   // 标准 HTTPS MTU
		BurstSize:       5,      // 中等突发
		BurstInterval:   150000, // 150ms 间隔
		Description:     "模拟标准 HTTPS 浏览：间歇性突发、混合包大小",
	}
)

// AllTemplates 所有预设模板
var AllTemplates = []DNATemplate{
	TemplateZoom,
	TemplateNetflix,
	TemplateWhatsApp,
	TemplateSteam,
	TemplateDiscord,
	TemplateHTTPS,
}

// GetTemplate 根据名称获取模板
func GetTemplate(name string) *DNATemplate {
	for _, tpl := range AllTemplates {
		if tpl.Name == name {
			return &tpl
		}
	}
	return nil
}

// GetTemplateByIndex 根据索引获取模板
func GetTemplateByIndex(index int) *DNATemplate {
	if index < 0 || index >= len(AllTemplates) {
		return &TemplateHTTPS // 默认返回 HTTPS
	}
	return &AllTemplates[index]
}

// TemplateToKernelConfig 将模板转换为内核配置
func (t *DNATemplate) ToKernelConfig() map[string]uint32 {
	return map[string]uint32{
		"target_iat_mu":     t.TargetIATMu,
		"target_iat_sigma":  t.TargetIATSigma,
		"padding_strategy":  t.PaddingStrategy,
		"target_mtu":        uint32(t.TargetMTU),
		"burst_size":        t.BurstSize,
		"burst_interval":    t.BurstInterval,
	}
}

// CalculateEntropy 计算模板的流量熵（用于评估隐蔽性）
func (t *DNATemplate) CalculateEntropy() float64 {
	// 简化的熵计算：基于 IAT 标准差与均值的比值
	// 熵越高，流量越随机，越难被识别
	if t.TargetIATMu == 0 {
		return 0
	}
	return float64(t.TargetIATSigma) / float64(t.TargetIATMu)
}

// IsHighThroughput 判断是否为高吞吐模板
func (t *DNATemplate) IsHighThroughput() bool {
	// IAT < 10ms 认为是高吞吐
	return t.TargetIATMu < 10000
}

// IsLowLatency 判断是否为低延迟模板
func (t *DNATemplate) IsLowLatency() bool {
	// IAT < 30ms 认为是低延迟
	return t.TargetIATMu < 30000
}

// GetPacketRate 获取包速率（pps）
func (t *DNATemplate) GetPacketRate() float64 {
	if t.TargetIATMu == 0 {
		return 0
	}
	// pps = 1,000,000 / IAT(us)
	return 1000000.0 / float64(t.TargetIATMu)
}

// GetBandwidth 估算带宽（Mbps）
func (t *DNATemplate) GetBandwidth() float64 {
	// 带宽 = pps × MTU × 8 / 1,000,000
	pps := t.GetPacketRate()
	return pps * float64(t.TargetMTU) * 8 / 1000000
}
