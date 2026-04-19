package ebpf

import "time"

// ConfigMap 对应 C 结构体 config_map
type ConfigMap struct {
	PaddingRate    uint32 // NPM 填充率 (0-100)
	JitterInterval uint32 // Jitter-Lite 扰动区间 (纳秒)
	PathCount      uint32 // G-Tunnel 路径数量
	ThreatLevel    uint32 // 威胁等级 (0-5)
	NoiseAmplitude uint32 // VPC 噪声幅度 (纳秒)
}

// ThreatEvent 对应 C 结构体 threat_event（严格对齐）
type ThreatEvent struct {
	Timestamp   uint64    // 纳秒时间戳
	ThreatType  uint32    // 威胁类型
	SourceIP    uint32    // 源 IP (网络字节序)
	SourcePort  uint16    // 源端口
	DestPort    uint16    // 目标端口
	PacketCount uint32    // 数据包计数
	Severity    uint32    // 严重程度 (0-10)
}

// EventType 威胁事件类型（保持兼容）
type EventType uint32

const (
	EventActiveProbing EventType = 1 // 主动探测
	EventReplayAttack  EventType = 2 // 重放攻击
	EventTimingAttack  EventType = 3 // 时序攻击
	EventDPIDetection  EventType = 4 // DPI 检测
)

// StrategyConfig 策略配置
type StrategyConfig struct {
	DefenseLevel   uint32        // 防御强度 (10/20/30)
	AutoAdjust     bool          // 自动调节
	UpdateInterval time.Duration // 更新间隔
}

// Statistics 统计信息
type Statistics struct {
	PacketsProcessed uint64 // 处理的数据包数
	PacketsDropped   uint64 // 丢弃的数据包数
	BytesProcessed   uint64 // 处理的字节数
	ThreatsDetected  uint64 // 检测到的威胁数
	AvgLatency       uint64 // 平均延迟 (纳秒)
}

// DefenseStrategy 防御策略
type DefenseStrategy struct {
	JitterMeanUs    uint32 // Jitter 平均值 (微秒)
	JitterStddevUs  uint32 // Jitter 标准差 (微秒)
	TemplateID      uint32 // 模板 ID
	FiberJitterUs   uint32 // 光缆抖动 (微秒)
	RouterDelayUs   uint32 // 路由器延迟 (微秒)
	NoiseIntensity  uint32 // 噪声强度 (0-100)
}

// JitterConfig Jitter-Lite 配置（对应 C 结构体）
type JitterConfig struct {
	Enabled     uint32
	MeanIATUs   uint32
	StddevIATUs uint32
	TemplateID  uint32
}

// VPCConfig VPC 配置（对应 C 结构体）
type VPCConfig struct {
	Enabled        uint32
	FiberJitterUs  uint32
	RouterDelayUs  uint32
	NoiseIntensity uint32
}

// ThreatHandler 威胁处理器接口
type ThreatHandler interface {
	HandleThreat(*ThreatEvent)
}
