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
	Timestamp   uint64 // 纳秒时间戳
	ThreatType  uint32 // 威胁类型
	SourceIP    uint32 // 源 IP (网络字节序)
	SourcePort  uint16 // 源端口
	DestPort    uint16 // 目标端口
	PacketCount uint32 // 数据包计数
	Severity    uint32 // 严重程度 (0-10)
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
	JitterMeanUs   uint32 // Jitter 平均值 (微秒)
	JitterStddevUs uint32 // Jitter 标准差 (微秒)
	TemplateID     uint32 // 模板 ID
	PaddingRate    uint32 // NPM 填充率 (0-100)
	FiberJitterUs  uint32 // 光缆抖动 (微秒)
	RouterDelayUs  uint32 // 路由器延迟 (微秒)
	NoiseIntensity uint32 // 噪声强度 (0-100)
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

const (
	NPMModeFixedMTU uint32 = iota
	NPMModeRandomRange
	NPMModeGaussian
	NPMModeMimic
)

const (
	DefaultNPMGlobalMTU     uint32 = 1460
	DefaultNPMMinPacketSize uint32 = 128
)

// NPMConfig NPM 配置（对应 C 结构体 struct npm_config）
type NPMConfig struct {
	Enabled       uint32
	FillingRate   uint32
	GlobalMTU     uint32
	MinPacketSize uint32
	PaddingMode   uint32
	DecoyRate     uint32
}

// NewDefaultNPMConfig 返回当前统一的 NPM 默认配置。
func NewDefaultNPMConfig(paddingRate uint32) NPMConfig {
	if paddingRate > 100 {
		paddingRate = 100
	}

	return NPMConfig{
		Enabled:       1,
		FillingRate:   paddingRate,
		GlobalMTU:     DefaultNPMGlobalMTU,
		MinPacketSize: DefaultNPMMinPacketSize,
		PaddingMode:   NPMModeMimic,
		DecoyRate:     0,
	}
}

// ConnKey mirrors B-DNA's C struct conn_key. L4Proto is part of the key so
// TCP and UDP flows with the same four-tuple cannot share a profile entry.
type ConnKey struct {
	SrcIP   uint32
	DstIP   uint32
	SrcPort uint16
	DstPort uint16
	L4Proto uint8
	Pad     [3]uint8
}

const (
	IPProtoTCP uint8 = 6
	IPProtoUDP uint8 = 17
)

// ThreatHandler 威胁处理器接口
type ThreatHandler interface {
	HandleThreat(*ThreatEvent)
}

// --- ICMP Tunnel 相关结构体（与 C 数据面 bpf/icmp_tunnel.c 严格字节对齐） ---

// ICMPConfig ICMP Tunnel 配置（Go → C，通过 eBPF Map 下发）
type ICMPConfig struct {
	Enabled    uint32 // 是否启用
	TargetIP   uint32 // 目标 IP（网络字节序）
	GatewayIP  uint32 // 网关 IP（网络字节序）
	Identifier uint16 // ICMP Identifier（会话标识）
	Reserved   uint16 // 保留字段，对齐用
}

// ICMPTxEntry ICMP 发送队列条目（Go → C，通过 eBPF Queue Map）
type ICMPTxEntry struct {
	Seq      uint32     // 序列号
	DataLen  uint16     // 数据长度
	Reserved uint16     // 保留字段
	Data     [1024]byte // 加密后的 Payload
}

// ICMPRxEvent ICMP 接收事件（C → Go，通过 Ring Buffer 上报）
type ICMPRxEvent struct {
	Timestamp  uint64     // 纳秒时间戳
	SrcIP      uint32     // 源 IP（网络字节序）
	Identifier uint16     // ICMP Identifier
	Seq        uint16     // 序列号
	DataLen    uint16     // 数据长度
	Reserved   uint16     // 保留字段
	Data       [1024]byte // 提取的 Payload
}

// --- L1 纵深防御相关结构体（与 C 数据面 bpf/common.h 严格字节对齐） ---

// RateLimitConfig 速率限制配置（Go → C，通过 rate_config_map 下发）
type RateLimitConfig struct {
	SynPPSLimit  uint32 // SYN 包每秒上限
	ConnPPSLimit uint32 // 总连接每秒上限
	Enabled      uint32 // 是否启用
}

// RateEvent 速率限制触发事件（C → Go，通过 l1_defense_events Ring Buffer 上报）
type RateEvent struct {
	Timestamp   uint64 // 纳秒时间戳
	SourceIP    uint32 // 源 IP（网络字节序）
	TriggerType uint32 // 0=SYN, 1=CONN
	CurrentRate uint64 // 当前速率
}

// SilentConfig 静默响应配置（Go → C，通过 silent_config_map 下发）
type SilentConfig struct {
	DropICMPUnreachable uint32 // 拦截 ICMP Unreachable
	DropTCPRst          uint32 // 拦截非法 TCP RST
	Enabled             uint32 // 是否启用
}

// L1Stats L1 层统计计数器（从 l1_stats_map 读取，与 C 侧 struct l1_stats 严格字节对齐）
type L1Stats struct {
	ASNDrops       uint64 // ASN 黑名单丢弃数
	RateDrops      uint64 // 速率限制丢弃数
	SilentDrops    uint64 // 静默响应丢弃数
	BlacklistDrops uint64 // 用户级黑名单命中（XDP 层）
	SanityDrops    uint64 // 非法画像丢弃
	ProfileDrops   uint64 // 入口准入拒绝
	TotalChecked   uint64 // 总检查数
	SynChallenge   uint64 // SYN challenge 触发次数
	AckForgery     uint64 // ACK 伪造检测次数
}
