// Package jitter 区域拟态配置
// 地缘特征上下文：让流量在空间上与当地环境完美融合
package jitter

import (
	"math/rand"
	"sync"
	"time"
)

// RegionID 区域标识
type RegionID string

const (
	RegionAsiaPacific RegionID = "asia_pacific" // 新加坡、香港、日本
	RegionEurope      RegionID = "europe"       // 瑞士、冰岛、德国
	RegionNorthAmerica RegionID = "north_america" // 美国、加拿大
	RegionMiddleEast  RegionID = "middle_east"  // 阿联酋、卡塔尔
	RegionChina       RegionID = "china"        // 中国大陆（实验性）
	RegionGlobal      RegionID = "global"       // 默认全球
)

// RegionalProfile 区域配置
type RegionalProfile struct {
	RegionID        RegionID
	Name            string
	Timezone        string
	
	// 拟态权重覆盖
	MimicryOverrides map[string]float64
	
	// TLS 指纹偏好
	TLSFingerprints []string
	
	// 背景噪声域名
	NoiseDomains []string
	
	// NTP 服务器
	NTPServers []string
	
	// 时段特定覆盖
	PeriodOverrides map[TimePeriod]map[string]float64
}

// RegionalProfileManager 区域配置管理器
type RegionalProfileManager struct {
	mu sync.RWMutex

	// 区域配置表
	profiles map[RegionID]*RegionalProfile

	// 当前区域
	currentRegion RegionID

	// 统计
	stats RegionalStats
}

// RegionalStats 统计
type RegionalStats struct {
	CurrentRegion      RegionID
	CompliancePercent  float64 // 环境融合度
	ProfileSwitches    int64
	NoiseDomainsUsed   int64
}

// NewRegionalProfileManager 创建区域配置管理器
func NewRegionalProfileManager(region RegionID) *RegionalProfileManager {
	rpm := &RegionalProfileManager{
		profiles:      make(map[RegionID]*RegionalProfile),
		currentRegion: region,
	}
	rpm.initProfiles()
	return rpm
}

// initProfiles 初始化各区域配置
func (rpm *RegionalProfileManager) initProfiles() {
	// 亚太区（新加坡、香港、日本）
	rpm.profiles[RegionAsiaPacific] = &RegionalProfile{
		RegionID: RegionAsiaPacific,
		Name:     "Asia-Pacific",
		Timezone: "Asia/Singapore",
		MimicryOverrides: map[string]float64{
			"youtube":  0.25,
			"bilibili": 0.20,
			"tiktok":   0.15,
			"zoom":     0.15,
			"whatsapp": 0.10,
			"discord":  0.10,
			"steam":    0.05,
		},
		TLSFingerprints: []string{
			"chrome-windows",
			"wechat-pc",
			"safari-ios",
			"chrome-android",
		},
		NoiseDomains: []string{
			"baidu.com", "shopee.sg", "go.gov.sg",
			"lazada.sg", "grab.com", "singtel.com",
			"line.me", "yahoo.co.jp", "rakuten.co.jp",
		},
		NTPServers: []string{
			"sg.pool.ntp.org",
			"hk.pool.ntp.org",
			"jp.pool.ntp.org",
		},
		PeriodOverrides: map[TimePeriod]map[string]float64{
			PeriodWorking: {
				"zoom":     0.25,
				"teams":    0.20,
				"slack":    0.15,
				"youtube":  0.15,
				"whatsapp": 0.15,
				"discord":  0.10,
			},
			PeriodLeisure: {
				"youtube":  0.30,
				"bilibili": 0.25,
				"tiktok":   0.20,
				"netflix":  0.10,
				"discord":  0.10,
				"steam":    0.05,
			},
			PeriodSleep: {
				"https":    0.40,
				"ntp":      0.30,
				"dns":      0.20,
				"steam":    0.10,
			},
		},
	}

	// 欧洲区（瑞士、冰岛、德国）
	rpm.profiles[RegionEurope] = &RegionalProfile{
		RegionID: RegionEurope,
		Name:     "Europe",
		Timezone: "Europe/Zurich",
		MimicryOverrides: map[string]float64{
			"spotify":  0.20,
			"netflix":  0.20,
			"teams":    0.20,
			"youtube":  0.15,
			"whatsapp": 0.15,
			"discord":  0.10,
		},
		TLSFingerprints: []string{
			"firefox-linux",
			"chrome-windows",
			"teams-desktop",
			"safari-macos",
		},
		NoiseDomains: []string{
			"bbc.co.uk", "spotify.com", "eur-lex.europa.eu",
			"srf.ch", "admin.ch", "swisscom.ch",
			"dw.com", "spiegel.de", "guardian.co.uk",
		},
		NTPServers: []string{
			"ch.pool.ntp.org",
			"de.pool.ntp.org",
			"uk.pool.ntp.org",
		},
		PeriodOverrides: map[TimePeriod]map[string]float64{
			PeriodWorking: {
				"teams":    0.30,
				"slack":    0.20,
				"zoom":     0.15,
				"spotify":  0.15,
				"whatsapp": 0.10,
				"https":    0.10,
			},
			PeriodLeisure: {
				"spotify":  0.25,
				"netflix":  0.25,
				"youtube":  0.20,
				"bbc":      0.15,
				"discord":  0.10,
				"steam":    0.05,
			},
			PeriodSleep: {
				"https":    0.40,
				"ntp":      0.30,
				"dns":      0.20,
				"spotify":  0.10,
			},
		},
	}

	// 北美区（美国、加拿大）
	rpm.profiles[RegionNorthAmerica] = &RegionalProfile{
		RegionID: RegionNorthAmerica,
		Name:     "North America",
		Timezone: "America/New_York",
		MimicryOverrides: map[string]float64{
			"netflix":  0.25,
			"zoom":     0.20,
			"slack":    0.15,
			"youtube":  0.15,
			"discord":  0.15,
			"steam":    0.10,
		},
		TLSFingerprints: []string{
			"chrome-macos",
			"chrome-windows",
			"zoom-windows",
			"safari-macos",
		},
		NoiseDomains: []string{
			"amazon.com", "nytimes.com", "cnn.com",
			"google.com", "apple.com", "microsoft.com",
			"reddit.com", "twitter.com", "facebook.com",
		},
		NTPServers: []string{
			"us.pool.ntp.org",
			"time.google.com",
			"time.apple.com",
		},
		PeriodOverrides: map[TimePeriod]map[string]float64{
			PeriodWorking: {
				"zoom":     0.30,
				"slack":    0.25,
				"teams":    0.20,
				"youtube":  0.10,
				"https":    0.10,
				"discord":  0.05,
			},
			PeriodLeisure: {
				"netflix":  0.30,
				"youtube":  0.25,
				"disney":   0.15,
				"discord":  0.15,
				"steam":    0.10,
				"spotify":  0.05,
			},
			PeriodSleep: {
				"https":    0.35,
				"ntp":      0.25,
				"dns":      0.20,
				"netflix":  0.10,
				"steam":    0.10,
			},
		},
	}

	// 中东区（阿联酋、卡塔尔）
	rpm.profiles[RegionMiddleEast] = &RegionalProfile{
		RegionID: RegionMiddleEast,
		Name:     "Middle East",
		Timezone: "Asia/Dubai",
		MimicryOverrides: map[string]float64{
			"telegram": 0.25,
			"whatsapp": 0.25,
			"youtube":  0.20,
			"zoom":     0.15,
			"netflix":  0.10,
			"discord":  0.05,
		},
		TLSFingerprints: []string{
			"chrome-android",
			"chrome-windows",
			"safari-ios",
		},
		NoiseDomains: []string{
			"aljazeera.net", "dubizzle.com", "souq.com",
			"emirates.com", "etisalat.ae", "du.ae",
			"arabnews.com", "khaleejtimes.com",
		},
		NTPServers: []string{
			"ae.pool.ntp.org",
			"asia.pool.ntp.org",
		},
		PeriodOverrides: map[TimePeriod]map[string]float64{
			PeriodWorking: {
				"zoom":     0.25,
				"teams":    0.25,
				"whatsapp": 0.20,
				"telegram": 0.15,
				"https":    0.10,
				"youtube":  0.05,
			},
			PeriodLeisure: {
				"youtube":  0.30,
				"telegram": 0.20,
				"whatsapp": 0.20,
				"netflix":  0.15,
				"anghami":  0.10,
				"discord":  0.05,
			},
			PeriodSleep: {
				"https":    0.35,
				"ntp":      0.25,
				"telegram": 0.20,
				"dns":      0.20,
			},
		},
	}

	// 中国区（实验性）
	rpm.profiles[RegionChina] = &RegionalProfile{
		RegionID: RegionChina,
		Name:     "China (Experimental)",
		Timezone: "Asia/Shanghai",
		MimicryOverrides: map[string]float64{
			"wechat":   0.35,
			"dingtalk": 0.20,
			"bilibili": 0.15,
			"douyin":   0.15,
			"taobao":   0.10,
			"https":    0.05,
		},
		TLSFingerprints: []string{
			"360-browser",
			"qq-browser",
			"wechat-pc",
			"chrome-windows-cn",
		},
		NoiseDomains: []string{
			"baidu.com", "weixin.qq.com", "taobao.com",
			"jd.com", "163.com", "sina.com.cn",
			"douyin.com", "bilibili.com", "zhihu.com",
		},
		NTPServers: []string{
			"cn.pool.ntp.org",
			"ntp.aliyun.com",
			"ntp.tencent.com",
		},
		PeriodOverrides: map[TimePeriod]map[string]float64{
			PeriodWorking: {
				"dingtalk": 0.35,
				"wechat":   0.30,
				"feishu":   0.15,
				"https":    0.10,
				"bilibili": 0.05,
				"taobao":   0.05,
			},
			PeriodLeisure: {
				"bilibili": 0.25,
				"douyin":   0.25,
				"wechat":   0.20,
				"taobao":   0.15,
				"https":    0.10,
				"steam":    0.05,
			},
			PeriodSleep: {
				"https":    0.40,
				"ntp":      0.25,
				"wechat":   0.20,
				"dns":      0.15,
			},
		},
	}

	// 全球默认
	rpm.profiles[RegionGlobal] = &RegionalProfile{
		RegionID: RegionGlobal,
		Name:     "Global Default",
		Timezone: "UTC",
		MimicryOverrides: map[string]float64{
			"zoom":     0.20,
			"netflix":  0.20,
			"youtube":  0.20,
			"whatsapp": 0.15,
			"discord":  0.15,
			"steam":    0.10,
		},
		TLSFingerprints: []string{
			"chrome-windows",
			"chrome-macos",
			"firefox-linux",
		},
		NoiseDomains: []string{
			"google.com", "apple.com", "microsoft.com",
			"cloudflare.com", "amazon.com", "github.com",
		},
		NTPServers: []string{
			"pool.ntp.org",
			"time.google.com",
		},
		PeriodOverrides: map[TimePeriod]map[string]float64{
			PeriodWorking: {
				"zoom":     0.30,
				"teams":    0.25,
				"slack":    0.20,
				"https":    0.15,
				"youtube":  0.10,
			},
			PeriodLeisure: {
				"netflix":  0.30,
				"youtube":  0.30,
				"discord":  0.20,
				"steam":    0.15,
				"spotify":  0.05,
			},
			PeriodSleep: {
				"https":    0.40,
				"ntp":      0.30,
				"dns":      0.20,
				"steam":    0.10,
			},
		},
	}
}

// GetRegionalProfile 获取区域配置
func (rpm *RegionalProfileManager) GetRegionalProfile(region RegionID) *RegionalProfile {
	rpm.mu.RLock()
	defer rpm.mu.RUnlock()

	if profile, ok := rpm.profiles[region]; ok {
		return profile
	}
	return rpm.profiles[RegionGlobal]
}

// GetCurrentProfile 获取当前区域配置
func (rpm *RegionalProfileManager) GetCurrentProfile() *RegionalProfile {
	rpm.mu.RLock()
	defer rpm.mu.RUnlock()
	return rpm.profiles[rpm.currentRegion]
}

// GetMimicryWeights 获取拟态权重（结合时段）
func (rpm *RegionalProfileManager) GetMimicryWeights(period TimePeriod) map[string]float64 {
	rpm.mu.RLock()
	defer rpm.mu.RUnlock()

	profile := rpm.profiles[rpm.currentRegion]
	if profile == nil {
		profile = rpm.profiles[RegionGlobal]
	}

	// 优先使用时段特定覆盖
	if periodWeights, ok := profile.PeriodOverrides[period]; ok {
		return periodWeights
	}
	return profile.MimicryOverrides
}

// GetNoiseDomains 获取区域噪声域名
func (rpm *RegionalProfileManager) GetNoiseDomains() []string {
	rpm.mu.RLock()
	defer rpm.mu.RUnlock()

	profile := rpm.profiles[rpm.currentRegion]
	if profile == nil {
		profile = rpm.profiles[RegionGlobal]
	}
	return profile.NoiseDomains
}

// GetRandomNoiseDomain 随机获取一个噪声域名
func (rpm *RegionalProfileManager) GetRandomNoiseDomain() string {
	rpm.mu.Lock()
	defer rpm.mu.Unlock()

	profile := rpm.profiles[rpm.currentRegion]
	if profile == nil {
		profile = rpm.profiles[RegionGlobal]
	}

	domains := profile.NoiseDomains
	if len(domains) == 0 {
		return "google.com"
	}

	rpm.stats.NoiseDomainsUsed++
	return domains[rand.Intn(len(domains))]
}

// GetTLSFingerprints 获取 TLS 指纹列表
func (rpm *RegionalProfileManager) GetTLSFingerprints() []string {
	rpm.mu.RLock()
	defer rpm.mu.RUnlock()

	profile := rpm.profiles[rpm.currentRegion]
	if profile == nil {
		profile = rpm.profiles[RegionGlobal]
	}
	return profile.TLSFingerprints
}

// GetRandomTLSFingerprint 随机获取一个 TLS 指纹
func (rpm *RegionalProfileManager) GetRandomTLSFingerprint() string {
	rpm.mu.RLock()
	defer rpm.mu.RUnlock()

	profile := rpm.profiles[rpm.currentRegion]
	if profile == nil {
		profile = rpm.profiles[RegionGlobal]
	}

	fps := profile.TLSFingerprints
	if len(fps) == 0 {
		return "chrome-windows"
	}
	return fps[rand.Intn(len(fps))]
}

// GetNTPServers 获取 NTP 服务器列表
func (rpm *RegionalProfileManager) GetNTPServers() []string {
	rpm.mu.RLock()
	defer rpm.mu.RUnlock()

	profile := rpm.profiles[rpm.currentRegion]
	if profile == nil {
		profile = rpm.profiles[RegionGlobal]
	}
	return profile.NTPServers
}

// SetRegion 设置当前区域
func (rpm *RegionalProfileManager) SetRegion(region RegionID) {
	rpm.mu.Lock()
	defer rpm.mu.Unlock()

	if _, ok := rpm.profiles[region]; ok {
		rpm.currentRegion = region
		rpm.stats.CurrentRegion = region
		rpm.stats.ProfileSwitches++
	}
}

// CalculateCompliance 计算环境融合度
func (rpm *RegionalProfileManager) CalculateCompliance(activeProfiles map[string]float64) float64 {
	rpm.mu.Lock()
	defer rpm.mu.Unlock()

	profile := rpm.profiles[rpm.currentRegion]
	if profile == nil {
		return 0.0
	}

	var matchScore float64
	var totalWeight float64

	for mimicry, expectedWeight := range profile.MimicryOverrides {
		totalWeight += expectedWeight
		if actualWeight, ok := activeProfiles[mimicry]; ok {
			diff := expectedWeight - actualWeight
			if diff < 0 {
				diff = -diff
			}
			matchScore += expectedWeight * (1.0 - diff)
		}
	}

	if totalWeight == 0 {
		return 0.0
	}

	compliance := (matchScore / totalWeight) * 100.0
	rpm.stats.CompliancePercent = compliance
	return compliance
}

// GetStats 获取统计信息
func (rpm *RegionalProfileManager) GetStats() RegionalStats {
	rpm.mu.RLock()
	defer rpm.mu.RUnlock()
	return rpm.stats
}

// DetectRegionFromIP 根据 IP 检测区域
func DetectRegionFromIP(ip string) RegionID {
	prefixMap := map[string]RegionID{
		"103.": RegionAsiaPacific,
		"104.": RegionNorthAmerica,
		"185.": RegionEurope,
		"188.": RegionEurope,
		"193.": RegionEurope,
		"194.": RegionEurope,
		"195.": RegionEurope,
		"212.": RegionMiddleEast,
		"213.": RegionMiddleEast,
		"223.": RegionChina,
		"116.": RegionChina,
		"117.": RegionChina,
		"118.": RegionChina,
		"119.": RegionChina,
	}

	for prefix, region := range prefixMap {
		if len(ip) >= len(prefix) && ip[:len(prefix)] == prefix {
			return region
		}
	}
	return RegionGlobal
}

// GetRegionTimezone 获取区域时区
func (rpm *RegionalProfileManager) GetRegionTimezone() *time.Location {
	rpm.mu.RLock()
	defer rpm.mu.RUnlock()

	profile := rpm.profiles[rpm.currentRegion]
	if profile == nil {
		return time.UTC
	}

	loc, err := time.LoadLocation(profile.Timezone)
	if err != nil {
		return time.UTC
	}
	return loc
}
