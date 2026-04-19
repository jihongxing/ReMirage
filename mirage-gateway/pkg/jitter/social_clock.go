// Package jitter 社交时钟调度器
// 基于时区的拟态强度自动调节
package jitter

import (
	"math/rand"
	"sync"
	"time"
)

// SocialClock 社交时钟
type SocialClock struct {
	mu sync.RWMutex

	// 时区
	location *time.Location

	// 当前时段
	currentPeriod TimePeriod

	// 时段配置
	periodConfigs map[TimePeriod]*PeriodConfig

	// 过渡状态
	transition TransitionState

	// 噪声配置
	noiseConfig NoiseConfig

	// 上次噪声时间
	lastNoiseTime time.Time

	// eBPF Map 更新回调
	onWeightUpdate func(weights *JitterWeights)

	// 噪声注入回调
	onNoiseInject func(noiseType string, target string)

	// 统计
	stats SocialClockStats

	// 停止信号
	stopChan chan struct{}

	// 区域配置管理器
	regionalManager *RegionalProfileManager
}

// TimePeriod 时段类型
type TimePeriod string

const (
	PeriodWorking TimePeriod = "working"  // 08:00 - 18:00
	PeriodLeisure TimePeriod = "leisure"  // 18:00 - 24:00
	PeriodSleep   TimePeriod = "sleep"    // 00:00 - 08:00
)

// PeriodConfig 时段配置
type PeriodConfig struct {
	Period          TimePeriod
	StartHour       int
	EndHour         int
	MimicryProfiles []string  // 拟态配置文件
	MaxBandwidthMbps int      // 最大带宽
	BurstAllowed    bool      // 是否允许突发
	JitterIntensity float64   // 抖动强度 0-1
	PacketSizeRange [2]int    // 包大小范围
}

// JitterWeights eBPF 抖动权重
type JitterWeights struct {
	BaseDelayNs     uint64
	JitterRangeNs   uint64
	BurstProbability float32
	MaxPacketSize   uint16
	MimicryType     uint8
}

// SocialClockStats 统计
type SocialClockStats struct {
	PeriodTransitions  int64
	WeightUpdates      int64
	CurrentPeriod      TimePeriod
	SocialMatchPercent float64
	TransitionProgress float64  // 过渡进度 0-1
	NoiseInjections    int64    // 噪声注入次数
	RegionalCompliance float64  // 区域融合度
	CurrentRegion      RegionID // 当前区域
}

// TransitionState 过渡状态
type TransitionState struct {
	InTransition   bool
	FromPeriod     TimePeriod
	ToPeriod       TimePeriod
	Progress       float64   // 0-1
	StartTime      time.Time
	DurationMin    int       // 过渡时长（分钟）
}

// NoiseConfig 噪声配置
type NoiseConfig struct {
	Enabled         bool
	Probability     float64   // 噪声触发概率 0.02-0.05
	DNSDomains      []string  // DNS 查询域名
	NTPServers      []string  // NTP 服务器
	MinIntervalSec  int       // 最小间隔
}

// NewSocialClock 创建社交时钟
func NewSocialClock(timezone string) (*SocialClock, error) {
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		// 回退到本地时区
		loc = time.Local
	}

	sc := &SocialClock{
		location:        loc,
		periodConfigs:   make(map[TimePeriod]*PeriodConfig),
		stopChan:        make(chan struct{}),
		regionalManager: NewRegionalProfileManager(RegionGlobal),
	}

	sc.initDefaultConfigs()
	sc.initNoiseConfig()
	sc.currentPeriod = sc.detectPeriod(time.Now().In(loc))

	return sc, nil
}

// NewSocialClockWithRegion 创建带区域的社交时钟
func NewSocialClockWithRegion(timezone string, region RegionID) (*SocialClock, error) {
	sc, err := NewSocialClock(timezone)
	if err != nil {
		return nil, err
	}
	sc.SetRegion(region)
	return sc, nil
}

// initNoiseConfig 初始化噪声配置
func (sc *SocialClock) initNoiseConfig() {
	// 从区域配置获取噪声域名和 NTP 服务器
	profile := sc.regionalManager.GetCurrentProfile()
	
	noiseDomains := []string{
		"www.google.com", "www.apple.com", "www.microsoft.com",
		"www.cloudflare.com", "www.amazon.com", "www.facebook.com",
	}
	ntpServers := []string{
		"time.google.com", "time.apple.com", "pool.ntp.org",
	}
	
	if profile != nil {
		if len(profile.NoiseDomains) > 0 {
			noiseDomains = profile.NoiseDomains
		}
		if len(profile.NTPServers) > 0 {
			ntpServers = profile.NTPServers
		}
	}

	sc.noiseConfig = NoiseConfig{
		Enabled:        true,
		Probability:    0.03, // 3% 概率
		DNSDomains:     noiseDomains,
		NTPServers:     ntpServers,
		MinIntervalSec: 30,
	}
}

// initDefaultConfigs 初始化默认配置
func (sc *SocialClock) initDefaultConfigs() {
	// 工作时段 08:00 - 18:00
	sc.periodConfigs[PeriodWorking] = &PeriodConfig{
		Period:           PeriodWorking,
		StartHour:        8,
		EndHour:          18,
		MimicryProfiles:  []string{"video_conference", "cloud_storage", "enterprise_saas", "webrtc"},
		MaxBandwidthMbps: 100,
		BurstAllowed:     true,
		JitterIntensity:  0.8,
		PacketSizeRange:  [2]int{64, 1500},
	}

	// 休闲时段 18:00 - 24:00
	sc.periodConfigs[PeriodLeisure] = &PeriodConfig{
		Period:           PeriodLeisure,
		StartHour:        18,
		EndHour:          24,
		MimicryProfiles:  []string{"netflix", "youtube", "spotify", "social_media", "gaming"},
		MaxBandwidthMbps: 50,
		BurstAllowed:     true,
		JitterIntensity:  0.6,
		PacketSizeRange:  [2]int{128, 1400},
	}

	// 睡眠时段 00:00 - 08:00
	sc.periodConfigs[PeriodSleep] = &PeriodConfig{
		Period:           PeriodSleep,
		StartHour:        0,
		EndHour:          8,
		MimicryProfiles:  []string{"system_update", "ntp_sync", "backup", "heartbeat"},
		MaxBandwidthMbps: 5,
		BurstAllowed:     false,
		JitterIntensity:  0.2,
		PacketSizeRange:  [2]int{40, 576},
	}
}

// detectPeriod 检测当前时段
func (sc *SocialClock) detectPeriod(t time.Time) TimePeriod {
	hour := t.Hour()

	if hour >= 8 && hour < 18 {
		return PeriodWorking
	} else if hour >= 18 && hour < 24 {
		return PeriodLeisure
	}
	return PeriodSleep
}

// Start 启动时钟
func (sc *SocialClock) Start() {
	go sc.runScheduler()
}

// Stop 停止
func (sc *SocialClock) Stop() {
	close(sc.stopChan)
}

// runScheduler 运行调度器
func (sc *SocialClock) runScheduler() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-sc.stopChan:
			return
		case <-ticker.C:
			sc.checkPeriodTransition()
		}
	}
}

// checkPeriodTransition 检查时段转换
func (sc *SocialClock) checkPeriodTransition() {
	now := time.Now().In(sc.location)
	newPeriod := sc.detectPeriod(now)

	sc.mu.Lock()
	
	// 检查是否需要开始过渡
	if newPeriod != sc.currentPeriod && !sc.transition.InTransition {
		sc.startTransition(sc.currentPeriod, newPeriod)
	}

	// 更新过渡进度
	if sc.transition.InTransition {
		sc.updateTransitionProgress(now)
	}

	sc.mu.Unlock()

	// 更新权重（考虑过渡状态）
	sc.updateWeightsWithTransition()

	// 更新社会化吻合度
	sc.updateSocialMatch(now)

	// 尝试注入噪声
	sc.tryInjectNoise(now)
}

// startTransition 开始过渡
func (sc *SocialClock) startTransition(from, to TimePeriod) {
	sc.transition = TransitionState{
		InTransition: true,
		FromPeriod:   from,
		ToPeriod:     to,
		Progress:     0,
		StartTime:    time.Now(),
		DurationMin:  30, // 30 分钟过渡期
	}
	sc.stats.PeriodTransitions++
}

// updateTransitionProgress 更新过渡进度
func (sc *SocialClock) updateTransitionProgress(now time.Time) {
	elapsed := now.Sub(sc.transition.StartTime)
	duration := time.Duration(sc.transition.DurationMin) * time.Minute

	progress := float64(elapsed) / float64(duration)
	if progress >= 1.0 {
		// 过渡完成
		sc.currentPeriod = sc.transition.ToPeriod
		sc.transition.InTransition = false
		sc.transition.Progress = 1.0
	} else {
		sc.transition.Progress = progress
	}

	sc.stats.TransitionProgress = sc.transition.Progress
	sc.stats.CurrentPeriod = sc.currentPeriod
}

// updateWeightsWithTransition 更新权重（考虑过渡）
func (sc *SocialClock) updateWeightsWithTransition() {
	sc.mu.RLock()
	inTransition := sc.transition.InTransition
	progress := sc.transition.Progress
	fromPeriod := sc.transition.FromPeriod
	toPeriod := sc.transition.ToPeriod
	currentPeriod := sc.currentPeriod
	sc.mu.RUnlock()

	var weights *JitterWeights

	if inTransition {
		// 混合两个时段的权重
		weights = sc.blendWeights(fromPeriod, toPeriod, progress)
	} else {
		config := sc.periodConfigs[currentPeriod]
		if config != nil {
			weights = sc.calculateWeights(config)
		}
	}

	if weights == nil {
		return
	}

	sc.mu.Lock()
	sc.stats.WeightUpdates++
	sc.mu.Unlock()

	if sc.onWeightUpdate != nil {
		sc.onWeightUpdate(weights)
	}
}

// blendWeights 混合两个时段的权重
func (sc *SocialClock) blendWeights(from, to TimePeriod, progress float64) *JitterWeights {
	fromConfig := sc.periodConfigs[from]
	toConfig := sc.periodConfigs[to]

	if fromConfig == nil || toConfig == nil {
		return nil
	}

	fromWeights := sc.calculateWeights(fromConfig)
	toWeights := sc.calculateWeights(toConfig)

	// 线性插值
	blended := &JitterWeights{
		BaseDelayNs:      uint64(float64(fromWeights.BaseDelayNs)*(1-progress) + float64(toWeights.BaseDelayNs)*progress),
		JitterRangeNs:    uint64(float64(fromWeights.JitterRangeNs)*(1-progress) + float64(toWeights.JitterRangeNs)*progress),
		BurstProbability: float32(float64(fromWeights.BurstProbability)*(1-progress) + float64(toWeights.BurstProbability)*progress),
		MaxPacketSize:    uint16(float64(fromWeights.MaxPacketSize)*(1-progress) + float64(toWeights.MaxPacketSize)*progress),
	}

	// MimicryType 在 50% 进度时切换
	if progress < 0.5 {
		blended.MimicryType = fromWeights.MimicryType
	} else {
		blended.MimicryType = toWeights.MimicryType
	}

	return blended
}

// tryInjectNoise 尝试注入噪声
func (sc *SocialClock) tryInjectNoise(now time.Time) {
	sc.mu.RLock()
	config := sc.noiseConfig
	lastNoise := sc.lastNoiseTime
	sc.mu.RUnlock()

	if !config.Enabled {
		return
	}

	// 检查最小间隔
	if now.Sub(lastNoise) < time.Duration(config.MinIntervalSec)*time.Second {
		return
	}

	// 概率触发
	rng := rand.New(rand.NewSource(now.UnixNano()))
	if rng.Float64() > config.Probability {
		return
	}

	// 选择噪声类型
	noiseType := "dns"
	target := ""

	if rng.Float32() < 0.7 {
		// 70% DNS 查询
		noiseType = "dns"
		target = config.DNSDomains[rng.Intn(len(config.DNSDomains))]
	} else {
		// 30% NTP 同步
		noiseType = "ntp"
		target = config.NTPServers[rng.Intn(len(config.NTPServers))]
	}

	sc.mu.Lock()
	sc.lastNoiseTime = now
	sc.stats.NoiseInjections++
	sc.mu.Unlock()

	if sc.onNoiseInject != nil {
		go sc.onNoiseInject(noiseType, target)
	}
}

// updateWeights 更新 eBPF 权重
func (sc *SocialClock) updateWeights() {
	sc.mu.RLock()
	config := sc.periodConfigs[sc.currentPeriod]
	sc.mu.RUnlock()

	if config == nil {
		return
	}

	weights := sc.calculateWeights(config)

	sc.mu.Lock()
	sc.stats.WeightUpdates++
	sc.mu.Unlock()

	if sc.onWeightUpdate != nil {
		sc.onWeightUpdate(weights)
	}
}

// calculateWeights 计算权重
func (sc *SocialClock) calculateWeights(config *PeriodConfig) *JitterWeights {
	weights := &JitterWeights{}

	// 基础延迟（纳秒）
	switch config.Period {
	case PeriodWorking:
		weights.BaseDelayNs = 1000000    // 1ms
		weights.JitterRangeNs = 5000000  // 5ms
		weights.MimicryType = 1          // 企业流量
	case PeriodLeisure:
		weights.BaseDelayNs = 2000000    // 2ms
		weights.JitterRangeNs = 10000000 // 10ms
		weights.MimicryType = 2          // 娱乐流量
	case PeriodSleep:
		weights.BaseDelayNs = 5000000    // 5ms
		weights.JitterRangeNs = 20000000 // 20ms
		weights.MimicryType = 3          // 维护流量
	}

	// 突发概率
	if config.BurstAllowed {
		weights.BurstProbability = float32(config.JitterIntensity) * 0.3
	} else {
		weights.BurstProbability = 0.01
	}

	// 最大包大小
	weights.MaxPacketSize = uint16(config.PacketSizeRange[1])

	return weights
}

// updateSocialMatch 更新社会化吻合度
func (sc *SocialClock) updateSocialMatch(now time.Time) {
	hour := now.Hour()
	minute := now.Minute()

	// 计算当前时间在时段内的位置
	var matchPercent float64

	sc.mu.RLock()
	config := sc.periodConfigs[sc.currentPeriod]
	sc.mu.RUnlock()

	if config == nil {
		return
	}

	// 时段中心点吻合度更高
	periodMid := (config.StartHour + config.EndHour) / 2
	hourDiff := abs(hour - periodMid)
	periodLen := config.EndHour - config.StartHour

	// 基础吻合度
	matchPercent = 100.0 - (float64(hourDiff) / float64(periodLen) * 40)

	// 分钟微调
	if minute >= 15 && minute <= 45 {
		matchPercent += 5 // 整点附近更自然
	}

	// 工作日加成
	weekday := now.Weekday()
	if weekday >= time.Monday && weekday <= time.Friday {
		if sc.currentPeriod == PeriodWorking {
			matchPercent += 10
		}
	} else {
		if sc.currentPeriod == PeriodLeisure {
			matchPercent += 10
		}
	}

	if matchPercent > 100 {
		matchPercent = 100
	}

	sc.mu.Lock()
	sc.stats.SocialMatchPercent = matchPercent
	sc.stats.CurrentPeriod = sc.currentPeriod
	sc.mu.Unlock()
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// GetCurrentPeriod 获取当前时段
func (sc *SocialClock) GetCurrentPeriod() TimePeriod {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.currentPeriod
}

// GetCurrentConfig 获取当前配置
func (sc *SocialClock) GetCurrentConfig() *PeriodConfig {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.periodConfigs[sc.currentPeriod]
}

// GetMimicryProfiles 获取当前拟态配置
func (sc *SocialClock) GetMimicryProfiles() []string {
	config := sc.GetCurrentConfig()
	if config == nil {
		return nil
	}
	return config.MimicryProfiles
}

// GetStats 获取统计
func (sc *SocialClock) GetStats() SocialClockStats {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.stats
}

// OnWeightUpdate 设置权重更新回调
func (sc *SocialClock) OnWeightUpdate(fn func(weights *JitterWeights)) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.onWeightUpdate = fn
}

// SetTimezone 设置时区
func (sc *SocialClock) SetTimezone(timezone string) error {
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return err
	}

	sc.mu.Lock()
	sc.location = loc
	sc.mu.Unlock()

	sc.checkPeriodTransition()
	return nil
}

// GetLocalTime 获取本地时间
func (sc *SocialClock) GetLocalTime() time.Time {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return time.Now().In(sc.location)
}

// GetTransitionState 获取过渡状态
func (sc *SocialClock) GetTransitionState() TransitionState {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.transition
}

// IsInTransition 是否在过渡中
func (sc *SocialClock) IsInTransition() bool {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.transition.InTransition
}

// GetBlendedProfiles 获取混合拟态配置（过渡期间）
func (sc *SocialClock) GetBlendedProfiles() ([]string, []float64) {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	if !sc.transition.InTransition {
		config := sc.periodConfigs[sc.currentPeriod]
		if config == nil {
			return nil, nil
		}
		weights := make([]float64, len(config.MimicryProfiles))
		for i := range weights {
			weights[i] = 1.0 / float64(len(config.MimicryProfiles))
		}
		return config.MimicryProfiles, weights
	}

	// 混合两个时段的配置
	fromConfig := sc.periodConfigs[sc.transition.FromPeriod]
	toConfig := sc.periodConfigs[sc.transition.ToPeriod]

	if fromConfig == nil || toConfig == nil {
		return nil, nil
	}

	progress := sc.transition.Progress
	var profiles []string
	var weights []float64

	// 添加 from 配置（权重递减）
	fromWeight := (1 - progress) / float64(len(fromConfig.MimicryProfiles))
	for _, p := range fromConfig.MimicryProfiles {
		profiles = append(profiles, p)
		weights = append(weights, fromWeight)
	}

	// 添加 to 配置（权重递增）
	toWeight := progress / float64(len(toConfig.MimicryProfiles))
	for _, p := range toConfig.MimicryProfiles {
		profiles = append(profiles, p)
		weights = append(weights, toWeight)
	}

	return profiles, weights
}

// OnNoiseInject 设置噪声注入回调
func (sc *SocialClock) OnNoiseInject(fn func(noiseType string, target string)) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.onNoiseInject = fn
}

// SetNoiseConfig 设置噪声配置
func (sc *SocialClock) SetNoiseConfig(config NoiseConfig) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.noiseConfig = config
}

// SetTransitionDuration 设置过渡时长
func (sc *SocialClock) SetTransitionDuration(minutes int) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	if sc.transition.InTransition {
		sc.transition.DurationMin = minutes
	}
}

// SetRegion 设置区域
func (sc *SocialClock) SetRegion(region RegionID) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	
	sc.regionalManager.SetRegion(region)
	sc.stats.CurrentRegion = region
	
	// 更新噪声配置
	profile := sc.regionalManager.GetCurrentProfile()
	if profile != nil {
		if len(profile.NoiseDomains) > 0 {
			sc.noiseConfig.DNSDomains = profile.NoiseDomains
		}
		if len(profile.NTPServers) > 0 {
			sc.noiseConfig.NTPServers = profile.NTPServers
		}
	}
}

// SetRegionFromIP 根据 IP 自动设置区域
func (sc *SocialClock) SetRegionFromIP(ip string) {
	region := DetectRegionFromIP(ip)
	sc.SetRegion(region)
}

// GetRegionalMimicryWeights 获取区域拟态权重
func (sc *SocialClock) GetRegionalMimicryWeights() map[string]float64 {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.regionalManager.GetMimicryWeights(sc.currentPeriod)
}

// GetRegionalTLSFingerprint 获取区域 TLS 指纹
func (sc *SocialClock) GetRegionalTLSFingerprint() string {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.regionalManager.GetRandomTLSFingerprint()
}

// GetRegionalCompliance 获取区域融合度
func (sc *SocialClock) GetRegionalCompliance(activeProfiles map[string]float64) float64 {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	
	compliance := sc.regionalManager.CalculateCompliance(activeProfiles)
	sc.stats.RegionalCompliance = compliance
	return compliance
}

// GetRegionalProfile 获取当前区域配置
func (sc *SocialClock) GetRegionalProfile() *RegionalProfile {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.regionalManager.GetCurrentProfile()
}

// GetCurrentRegion 获取当前区域
func (sc *SocialClock) GetCurrentRegion() RegionID {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.stats.CurrentRegion
}