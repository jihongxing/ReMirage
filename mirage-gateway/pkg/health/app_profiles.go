// Package health - 全球百大 App 探测矩阵
// 模拟主流 App 的业务特征，主动诱导并分析干扰行为
package health

import (
	"time"
)

// AppCategory App 类别
type AppCategory int

const (
	CategoryMessaging   AppCategory = 0 // 即时通讯
	CategoryVideo       AppCategory = 1 // 视频流媒体
	CategorySocial      AppCategory = 2 // 社交网络
	CategoryVoIP        AppCategory = 3 // 语音通话
	CategoryGaming      AppCategory = 4 // 游戏
	CategoryCloud       AppCategory = 5 // 云服务
	CategoryFinance     AppCategory = 6 // 金融
	CategoryProductivity AppCategory = 7 // 生产力工具
)

// AppProfile App 流量特征画像
type AppProfile struct {
	Name           string      `json:"name"`
	Category       AppCategory `json:"category"`
	Domains        []string    `json:"domains"`
	Ports          []uint16    `json:"ports"`
	Protocol       string      `json:"protocol"` // TCP/UDP/QUIC
	
	// 流量特征
	TypicalMTU     uint16      `json:"typical_mtu"`
	PacketSizeMin  uint16      `json:"packet_size_min"`
	PacketSizeMax  uint16      `json:"packet_size_max"`
	BurstSize      int         `json:"burst_size"`       // 突发包数
	BurstInterval  time.Duration `json:"burst_interval"` // 突发间隔
	
	// 时序特征
	HeartbeatInterval time.Duration `json:"heartbeat_interval"`
	RTTTolerance      time.Duration `json:"rtt_tolerance"`     // RTT 容忍度
	JitterTolerance   float64       `json:"jitter_tolerance"`  // 抖动容忍度 (%)
	
	// 握手特征
	HandshakePattern  []byte    `json:"handshake_pattern"`  // 握手特征字节
	SNIPattern        string    `json:"sni_pattern"`        // SNI 模式
	
	// 区域权重
	RegionalWeight    map[string]float64 `json:"regional_weight"` // 区域流行度
}

// BlockingSignature 封锁特征
type BlockingSignature struct {
	Type        string `json:"type"`         // zero_window, rst_inject, 403_inject, 302_redirect
	Pattern     []byte `json:"pattern"`      // 特征字节
	Description string `json:"description"`
}

// GlobalAppProfiles 全球百大 App 特征库
var GlobalAppProfiles = map[string]*AppProfile{
	// ========== 即时通讯 ==========
	"whatsapp": {
		Name:     "WhatsApp",
		Category: CategoryMessaging,
		Domains:  []string{"web.whatsapp.com", "wa.me", "*.whatsapp.net"},
		Ports:    []uint16{443, 5222},
		Protocol: "TCP",
		TypicalMTU:        1400,
		PacketSizeMin:     64,
		PacketSizeMax:     1400,
		BurstSize:         3,
		BurstInterval:     50 * time.Millisecond,
		HeartbeatInterval: 25 * time.Second,
		RTTTolerance:      300 * time.Millisecond,
		JitterTolerance:   30,
		HandshakePattern:  []byte{0x00, 0x04, 0x00, 0x01}, // Noise Protocol 片段
		SNIPattern:        "*.whatsapp.net",
		RegionalWeight: map[string]float64{
			"sg": 0.9, "de": 0.8, "us": 0.7, "br": 0.95,
		},
	},
	"telegram": {
		Name:     "Telegram",
		Category: CategoryMessaging,
		Domains:  []string{"telegram.org", "t.me", "*.telegram.org"},
		Ports:    []uint16{443, 80, 5222},
		Protocol: "TCP",
		TypicalMTU:        1460,
		PacketSizeMin:     56,
		PacketSizeMax:     1460,
		BurstSize:         4,
		BurstInterval:     30 * time.Millisecond,
		HeartbeatInterval: 15 * time.Second,
		RTTTolerance:      250 * time.Millisecond,
		JitterTolerance:   25,
		HandshakePattern:  []byte{0xef, 0xef, 0xef, 0xef}, // MTProto 特征
		SNIPattern:        "*.telegram.org",
		RegionalWeight: map[string]float64{
			"sg": 0.85, "de": 0.9, "ru": 0.3, "ir": 0.1,
		},
	},
	"signal": {
		Name:     "Signal",
		Category: CategoryMessaging,
		Domains:  []string{"signal.org", "*.signal.org"},
		Ports:    []uint16{443},
		Protocol: "TCP",
		TypicalMTU:        1400,
		PacketSizeMin:     64,
		PacketSizeMax:     1400,
		BurstSize:         2,
		BurstInterval:     100 * time.Millisecond,
		HeartbeatInterval: 30 * time.Second,
		RTTTolerance:      400 * time.Millisecond,
		JitterTolerance:   35,
		HandshakePattern:  []byte{0x00, 0x00, 0x00, 0x00}, // Signal Protocol
		SNIPattern:        "*.signal.org",
		RegionalWeight: map[string]float64{
			"sg": 0.6, "de": 0.7, "us": 0.8,
		},
	},

	// ========== 视频流媒体 ==========
	"youtube": {
		Name:     "YouTube",
		Category: CategoryVideo,
		Domains:  []string{"youtube.com", "*.googlevideo.com", "*.ytimg.com"},
		Ports:    []uint16{443, 80},
		Protocol: "QUIC",
		TypicalMTU:        1350,
		PacketSizeMin:     1200,
		PacketSizeMax:     1350,
		BurstSize:         10,
		BurstInterval:     5 * time.Millisecond,
		HeartbeatInterval: 0, // 持续流
		RTTTolerance:      500 * time.Millisecond,
		JitterTolerance:   50,
		HandshakePattern:  []byte{0xc0, 0x00, 0x00, 0x01}, // QUIC Initial
		SNIPattern:        "*.googlevideo.com",
		RegionalWeight: map[string]float64{
			"sg": 0.95, "de": 0.95, "us": 0.98, "cn": 0.05,
		},
	},
	"tiktok": {
		Name:     "TikTok",
		Category: CategoryVideo,
		Domains:  []string{"tiktok.com", "*.tiktokcdn.com", "*.musical.ly"},
		Ports:    []uint16{443},
		Protocol: "QUIC",
		TypicalMTU:        1400,
		PacketSizeMin:     800,
		PacketSizeMax:     1400,
		BurstSize:         8,
		BurstInterval:     10 * time.Millisecond,
		HeartbeatInterval: 0,
		RTTTolerance:      400 * time.Millisecond,
		JitterTolerance:   45,
		HandshakePattern:  []byte{0xc0, 0x00, 0x00, 0x01},
		SNIPattern:        "*.tiktokcdn.com",
		RegionalWeight: map[string]float64{
			"sg": 0.9, "de": 0.85, "us": 0.8, "in": 0.3,
		},
	},
	"netflix": {
		Name:     "Netflix",
		Category: CategoryVideo,
		Domains:  []string{"netflix.com", "*.nflxvideo.net"},
		Ports:    []uint16{443},
		Protocol: "TCP",
		TypicalMTU:        1460,
		PacketSizeMin:     1400,
		PacketSizeMax:     1460,
		BurstSize:         15,
		BurstInterval:     2 * time.Millisecond,
		HeartbeatInterval: 0,
		RTTTolerance:      600 * time.Millisecond,
		JitterTolerance:   60,
		HandshakePattern:  []byte{0x16, 0x03, 0x01}, // TLS 1.0 ClientHello
		SNIPattern:        "*.nflxvideo.net",
		RegionalWeight: map[string]float64{
			"sg": 0.8, "de": 0.85, "us": 0.95,
		},
	},

	// ========== 社交网络 ==========
	"instagram": {
		Name:     "Instagram",
		Category: CategorySocial,
		Domains:  []string{"instagram.com", "*.cdninstagram.com"},
		Ports:    []uint16{443},
		Protocol: "QUIC",
		TypicalMTU:        1350,
		PacketSizeMin:     500,
		PacketSizeMax:     1350,
		BurstSize:         6,
		BurstInterval:     20 * time.Millisecond,
		HeartbeatInterval: 30 * time.Second,
		RTTTolerance:      400 * time.Millisecond,
		JitterTolerance:   40,
		HandshakePattern:  []byte{0xc0, 0x00, 0x00, 0x01},
		SNIPattern:        "*.cdninstagram.com",
		RegionalWeight: map[string]float64{
			"sg": 0.9, "de": 0.9, "us": 0.95, "ir": 0.2,
		},
	},
	"twitter": {
		Name:     "Twitter/X",
		Category: CategorySocial,
		Domains:  []string{"twitter.com", "x.com", "*.twimg.com"},
		Ports:    []uint16{443},
		Protocol: "TCP",
		TypicalMTU:        1460,
		PacketSizeMin:     200,
		PacketSizeMax:     1460,
		BurstSize:         4,
		BurstInterval:     50 * time.Millisecond,
		HeartbeatInterval: 60 * time.Second,
		RTTTolerance:      350 * time.Millisecond,
		JitterTolerance:   35,
		HandshakePattern:  []byte{0x16, 0x03, 0x03},
		SNIPattern:        "*.twimg.com",
		RegionalWeight: map[string]float64{
			"sg": 0.85, "de": 0.8, "us": 0.9,
		},
	},

	// ========== VoIP ==========
	"discord": {
		Name:     "Discord",
		Category: CategoryVoIP,
		Domains:  []string{"discord.com", "*.discord.gg", "*.discordapp.net"},
		Ports:    []uint16{443, 50000, 50001},
		Protocol: "UDP",
		TypicalMTU:        1200,
		PacketSizeMin:     100,
		PacketSizeMax:     1200,
		BurstSize:         50,
		BurstInterval:     20 * time.Millisecond,
		HeartbeatInterval: 5 * time.Second,
		RTTTolerance:      150 * time.Millisecond, // VoIP 对延迟敏感
		JitterTolerance:   15,
		HandshakePattern:  []byte{0x80, 0x00}, // RTP
		SNIPattern:        "*.discordapp.net",
		RegionalWeight: map[string]float64{
			"sg": 0.8, "de": 0.85, "us": 0.9,
		},
	},
	"zoom": {
		Name:     "Zoom",
		Category: CategoryVoIP,
		Domains:  []string{"zoom.us", "*.zoom.us"},
		Ports:    []uint16{443, 8801, 8802},
		Protocol: "UDP",
		TypicalMTU:        1200,
		PacketSizeMin:     80,
		PacketSizeMax:     1200,
		BurstSize:         30,
		BurstInterval:     20 * time.Millisecond,
		HeartbeatInterval: 3 * time.Second,
		RTTTolerance:      200 * time.Millisecond,
		JitterTolerance:   20,
		HandshakePattern:  []byte{0x05, 0x00},
		SNIPattern:        "*.zoom.us",
		RegionalWeight: map[string]float64{
			"sg": 0.9, "de": 0.9, "us": 0.95,
		},
	},

	// ========== 云服务 ==========
	"google_drive": {
		Name:     "Google Drive",
		Category: CategoryCloud,
		Domains:  []string{"drive.google.com", "*.googleusercontent.com"},
		Ports:    []uint16{443},
		Protocol: "QUIC",
		TypicalMTU:        1350,
		PacketSizeMin:     500,
		PacketSizeMax:     1350,
		BurstSize:         5,
		BurstInterval:     30 * time.Millisecond,
		HeartbeatInterval: 60 * time.Second,
		RTTTolerance:      500 * time.Millisecond,
		JitterTolerance:   50,
		HandshakePattern:  []byte{0xc0, 0x00, 0x00, 0x01},
		SNIPattern:        "*.googleusercontent.com",
		RegionalWeight: map[string]float64{
			"sg": 0.9, "de": 0.9, "us": 0.95,
		},
	},
	"dropbox": {
		Name:     "Dropbox",
		Category: CategoryCloud,
		Domains:  []string{"dropbox.com", "*.dropboxstatic.com"},
		Ports:    []uint16{443},
		Protocol: "TCP",
		TypicalMTU:        1460,
		PacketSizeMin:     500,
		PacketSizeMax:     1460,
		BurstSize:         4,
		BurstInterval:     50 * time.Millisecond,
		HeartbeatInterval: 120 * time.Second,
		RTTTolerance:      600 * time.Millisecond,
		JitterTolerance:   55,
		HandshakePattern:  []byte{0x16, 0x03, 0x03},
		SNIPattern:        "*.dropbox.com",
		RegionalWeight: map[string]float64{
			"sg": 0.7, "de": 0.75, "us": 0.85,
		},
	},
}

// KnownBlockingSignatures 已知封锁特征库
var KnownBlockingSignatures = []BlockingSignature{
	{
		Type:        "tcp_zero_window",
		Pattern:     []byte{0x00, 0x00}, // Window Size = 0
		Description: "TCP 零窗口攻击，常用于限制视频流",
	},
	{
		Type:        "rst_injection",
		Pattern:     []byte{0x00, 0x14}, // RST flag
		Description: "RST 注入，强制断开连接",
	},
	{
		Type:        "http_403_inject",
		Pattern:     []byte("HTTP/1.1 403"),
		Description: "伪造 403 响应",
	},
	{
		Type:        "http_302_redirect",
		Pattern:     []byte("HTTP/1.1 302"),
		Description: "重定向到警告页面",
	},
	{
		Type:        "quic_version_neg",
		Pattern:     []byte{0x00, 0x00, 0x00, 0x00}, // Version 0
		Description: "QUIC 版本协商攻击",
	},
	{
		Type:        "tls_alert",
		Pattern:     []byte{0x15, 0x03}, // TLS Alert
		Description: "TLS Alert 注入",
	},
}

// GetAppProfile 获取 App 特征
func GetAppProfile(appName string) *AppProfile {
	return GlobalAppProfiles[appName]
}

// GetAppsByCategory 按类别获取 App 列表
func GetAppsByCategory(category AppCategory) []*AppProfile {
	var apps []*AppProfile
	for _, profile := range GlobalAppProfiles {
		if profile.Category == category {
			apps = append(apps, profile)
		}
	}
	return apps
}

// GetRegionalApps 获取区域热门 App
func GetRegionalApps(region string, minWeight float64) []*AppProfile {
	var apps []*AppProfile
	for _, profile := range GlobalAppProfiles {
		if weight, ok := profile.RegionalWeight[region]; ok && weight >= minWeight {
			apps = append(apps, profile)
		}
	}
	return apps
}
