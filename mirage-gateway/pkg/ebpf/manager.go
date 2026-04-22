// Package ebpf - eBPF 管理器
// 负责 Map 的读写封装和策略应用
package ebpf

import (
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// DefenseApplier 防御策略应用器
type DefenseApplier struct {
	loader           *Loader
	config           *DefenseConfig
	stopCh           chan struct{}
	updateCh         chan *DefenseConfig
	consecutiveFails int // 连续刷新失败计数
	maxConsecFails   int // 触发紧急降级的阈值
}

// DefenseConfig 防御配置
type DefenseConfig struct {
	Level          uint32        // 防御等级 (10/20/30)
	JitterMeanUs   uint32        // Jitter 平均值 (微秒)
	JitterStddevUs uint32        // Jitter 标准差 (微秒)
	PaddingRate    uint32        // NPM 填充率 (%)
	NoiseIntensity uint32        // VPC 噪声强度 (%)
	UpdateInterval time.Duration // 更新间隔
}

// NewDefenseApplier 创建防御应用器
func NewDefenseApplier(loader *Loader, config *DefenseConfig) *DefenseApplier {
	return &DefenseApplier{
		loader:           loader,
		config:           config,
		stopCh:           make(chan struct{}),
		updateCh:         make(chan *DefenseConfig, 10),
		consecutiveFails: 0,
		maxConsecFails:   3,
	}
}

// Start 启动防御应用器
func (da *DefenseApplier) Start() {
	// 应用初始策略
	if err := da.applyStrategy(da.config); err != nil {
		log.Printf("❌ 应用初始策略失败: %v", err)
		return
	}

	// 同步默认入口画像（失败不阻断启动）
	if err := da.SyncIngressProfiles(DefaultIngressProfiles()); err != nil {
		log.Printf("⚠️ 入口画像同步失败（降级）: %v", err)
	}

	// 启动定期更新
	go da.updateLoop()

	// 监听系统信号
	go da.signalHandler()

	log.Println("✅ 防御应用器已启动")
}

// Stop 停止防御应用器
func (da *DefenseApplier) Stop() {
	close(da.stopCh)
	log.Println("🛑 防御应用器已停止")
}

// UpdateConfig 更新配置
func (da *DefenseApplier) UpdateConfig(config *DefenseConfig) error {
	select {
	case da.updateCh <- config:
		log.Printf("📝 配置更新请求已提交: Level=%d%%", config.Level)
		return nil
	default:
		log.Println("⚠️  配置更新队列已满，跳过")
		return fmt.Errorf("配置更新队列已满")
	}
}

// updateLoop 定期更新循环
func (da *DefenseApplier) updateLoop() {
	ticker := time.NewTicker(da.config.UpdateInterval)
	defer ticker.Stop()

	for {
		select {
		case <-da.stopCh:
			return

		case newConfig := <-da.updateCh:
			if err := da.applyStrategy(newConfig); err != nil {
				log.Printf("❌ 应用策略失败: %v", err)
				da.consecutiveFails++
				da.checkEmergencyDegradation()
			} else {
				da.config = newConfig
				da.consecutiveFails = 0
				log.Printf("✅ 策略已更新: Level=%d%%", newConfig.Level)
			}

		case <-ticker.C:
			// 定期刷新策略（防止 Map 被意外清空）
			if err := da.applyStrategy(da.config); err != nil {
				log.Printf("⚠️  策略刷新失败: %v", err)
				da.consecutiveFails++
				da.checkEmergencyDegradation()
			} else {
				da.consecutiveFails = 0
			}
		}
	}
}

// checkEmergencyDegradation 检查是否需要触发紧急降级
func (da *DefenseApplier) checkEmergencyDegradation() {
	if da.consecutiveFails < da.maxConsecFails {
		return
	}

	log.Printf("🚨 [DefenseApplier] 连续 %d 次 eBPF Map 写入失败，触发紧急降级", da.consecutiveFails)

	// 写入 emergency_ctrl_map 通知内核进入降级模式
	emergencyMap := da.loader.GetMap("emergency_ctrl_map")
	if emergencyMap == nil {
		log.Printf("🚨 [DefenseApplier] emergency_ctrl_map 不存在，无法降级")
		return
	}

	key := uint32(0)
	value := uint32(1) // 1 = 降级模式（内核侧所有协议回退到 PASS-THROUGH）
	if err := emergencyMap.Put(&key, &value); err != nil {
		log.Printf("🚨 [DefenseApplier] 写入 emergency_ctrl_map 也失败: %v（内核可能内存耗尽）", err)
	} else {
		log.Printf("🚨 [DefenseApplier] 已写入紧急降级标志，内核数据面进入 PASS-THROUGH 模式")
	}
}

// applyStrategy 应用防御策略
func (da *DefenseApplier) applyStrategy(config *DefenseConfig) error {
	strategy := &DefenseStrategy{
		JitterMeanUs:   config.JitterMeanUs,
		JitterStddevUs: config.JitterStddevUs,
		TemplateID:     1,
		PaddingRate:    config.PaddingRate,
		FiberJitterUs:  config.JitterMeanUs / 5,
		RouterDelayUs:  config.JitterMeanUs / 10,
		NoiseIntensity: config.NoiseIntensity,
	}

	return da.loader.UpdateStrategy(strategy)
}

// signalHandler 信号处理器
func (da *DefenseApplier) signalHandler() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	for {
		select {
		case <-da.stopCh:
			return

		case sig := <-sigCh:
			switch sig {
			case syscall.SIGINT, syscall.SIGTERM:
				log.Printf("🛑 收到退出信号: %v", sig)
				da.cleanup()
				os.Exit(0)

			case syscall.SIGHUP:
				log.Println("🔄 收到重载信号，刷新策略...")
				if err := da.applyStrategy(da.config); err != nil {
					log.Printf("❌ 策略刷新失败: %v", err)
				}
			}
		}
	}
}

// cleanup 清理资源
func (da *DefenseApplier) cleanup() {
	log.Println("🧹 开始清理资源...")

	// 关闭 Loader
	if err := da.loader.Close(); err != nil {
		log.Printf("⚠️  关闭 Loader 失败: %v", err)
	}

	// 清理 TC 钩子
	if err := da.cleanupTC(); err != nil {
		log.Printf("⚠️  清理 TC 钩子失败: %v", err)
	}

	log.Println("✅ 资源清理完成")
}

// cleanupTC 清理 TC 钩子（委托给 Loader.Close）
func (da *DefenseApplier) cleanupTC() error {
	if da.loader == nil {
		return nil
	}
	// Loader.Close() 已实现完整的 TC filter 卸载（netlink.FilterDel）
	return da.loader.Close()
}

// GetCurrentConfig 获取当前配置
func (da *DefenseApplier) GetCurrentConfig() *DefenseConfig {
	return da.config
}

// ASNBlockEntry ASN 黑名单条目
type ASNBlockEntry struct {
	CIDR string // CIDR 网段（如 "3.0.0.0/15"）
	ASN  uint32 // ASN 编号
}

// SyncASNBlocklist 将 ASN 黑名单条目同步到 asn_blocklist_lpm eBPF Map
func (da *DefenseApplier) SyncASNBlocklist(entries []ASNBlockEntry) error {
	lpmMap := da.loader.GetMap("asn_blocklist_lpm")
	if lpmMap == nil {
		return fmt.Errorf("asn_blocklist_lpm Map 不存在")
	}

	var failCount int
	for _, entry := range entries {
		_, ipNet, err := net.ParseCIDR(entry.CIDR)
		if err != nil {
			failCount++
			continue
		}

		ip4 := ipNet.IP.To4()
		if ip4 == nil {
			failCount++
			continue
		}

		ones, _ := ipNet.Mask.Size()

		// LPM Trie key: prefixlen(4 bytes) + ip(4 bytes)
		key := make([]byte, 8)
		binary.LittleEndian.PutUint32(key[0:4], uint32(ones))
		copy(key[4:8], ip4)

		value := entry.ASN
		if err := lpmMap.Put(key, &value); err != nil {
			log.Printf("[L1Defense] ❌ ASN LPM 写入失败: %s (ASN %d): %v", entry.CIDR, entry.ASN, err)
			failCount++
		}
	}

	if failCount > 0 {
		log.Printf("[L1Defense] ASN 黑名单同步完成: %d/%d 成功, %d 失败",
			len(entries)-failCount, len(entries), failCount)
	} else {
		log.Printf("[L1Defense] ✅ ASN 黑名单同步完成: %d 条目", len(entries))
	}

	return nil
}

// SyncRateLimitConfig 将速率限制配置同步到 rate_config_map eBPF Map
func (da *DefenseApplier) SyncRateLimitConfig(cfg *RateLimitConfig) error {
	rateConfigMap := da.loader.GetMap("rate_config_map")
	if rateConfigMap == nil {
		return fmt.Errorf("rate_config_map Map 不存在")
	}

	key := uint32(0)
	if err := rateConfigMap.Put(&key, cfg); err != nil {
		return fmt.Errorf("[L1Defense] 写入 rate_config_map 失败: %w", err)
	}

	log.Printf("[L1Defense] ✅ 速率限制配置已同步: SYN=%d/s, CONN=%d/s, Enabled=%d",
		cfg.SynPPSLimit, cfg.ConnPPSLimit, cfg.Enabled)
	return nil
}

// SyncSilentConfig 将静默响应配置同步到 silent_config_map eBPF Map
func (da *DefenseApplier) SyncSilentConfig(cfg *SilentConfig) error {
	silentConfigMap := da.loader.GetMap("silent_config_map")
	if silentConfigMap == nil {
		return fmt.Errorf("silent_config_map Map 不存在")
	}

	key := uint32(0)
	if err := silentConfigMap.Put(&key, cfg); err != nil {
		return fmt.Errorf("[L1Defense] 写入 silent_config_map 失败: %w", err)
	}

	log.Printf("[L1Defense] ✅ 静默响应配置已同步: DropICMP=%d, DropRST=%d, Enabled=%d",
		cfg.DropICMPUnreachable, cfg.DropTCPRst, cfg.Enabled)
	return nil
}

// AdjustDefenseLevel 调整防御等级
func (da *DefenseApplier) AdjustDefenseLevel(level uint32) error {
	if level != 10 && level != 20 && level != 30 {
		return fmt.Errorf("无效的防御等级: %d (必须是 10/20/30)", level)
	}

	newConfig := &DefenseConfig{
		Level:          level,
		JitterMeanUs:   50000,                    // 50ms
		JitterStddevUs: 15000,                    // 15ms
		PaddingRate:    level,                    // 与防御等级一致
		NoiseIntensity: level,                    // 与防御等级一致
		UpdateInterval: da.config.UpdateInterval, // 保持不变
	}

	da.UpdateConfig(newConfig)
	return nil
}

// IngressProfile 入口画像配置（Go 侧结构体，对应 C 侧 struct ingress_profile）
type IngressProfile struct {
	Port          uint16 // 监听端口
	AllowedProto  uint8  // 0x01=TCP, 0x02=UDP, 0x03=BOTH
	RequireMinLen uint8  // 最小载荷长度
	UDPMinPayload uint32 // UDP 最小载荷（QUIC Initial ≥ 1200）
}

// SyncIngressProfiles 将入口画像配置同步到 ingress_profile_map eBPF Map（7.3）
func (da *DefenseApplier) SyncIngressProfiles(profiles []IngressProfile) error {
	profileMap := da.loader.GetMap("ingress_profile_map")
	if profileMap == nil {
		return fmt.Errorf("ingress_profile_map Map 不存在（降级：入口画像检查跳过）")
	}

	for _, p := range profiles {
		key := p.Port
		if err := profileMap.Put(&key, &p); err != nil {
			log.Printf("[L1Defense] ❌ 入口画像写入失败: port=%d: %v", p.Port, err)
			continue
		}
	}

	log.Printf("[L1Defense] ✅ 入口画像同步完成: %d 个入口", len(profiles))
	return nil
}

// DefaultIngressProfiles 返回 Mirage 默认入口画像
func DefaultIngressProfiles() []IngressProfile {
	return []IngressProfile{
		{Port: 443, AllowedProto: 0x03, UDPMinPayload: 1200}, // QUIC + TLS（QUIC Initial ≥ 1200）
		{Port: 8443, AllowedProto: 0x01},                     // WSS（仅 TCP）
		{Port: 3478, AllowedProto: 0x02, UDPMinPayload: 20},  // WebRTC TURN（UDP）
	}
}

// UpdateIngressProfiles 运行时动态更新入口画像（通过 updateCh 异步执行）
func (da *DefenseApplier) UpdateIngressProfiles(profiles []IngressProfile) error {
	go func() {
		if err := da.SyncIngressProfiles(profiles); err != nil {
			log.Printf("[L1Defense] ⚠️ 动态更新入口画像失败: %v", err)
		}
	}()
	return nil
}

// ReadL1Stats 从 l1_stats_map 读取统计数据
func (da *DefenseApplier) ReadL1Stats() (*L1Stats, error) {
	statsMap := da.loader.GetMap("l1_stats_map")
	if statsMap == nil {
		return nil, fmt.Errorf("l1_stats_map Map 不存在")
	}

	key := uint32(0)
	var stats L1Stats
	if err := statsMap.Lookup(&key, &stats); err != nil {
		return nil, fmt.Errorf("读取 l1_stats_map 失败: %w", err)
	}

	return &stats, nil
}

// SyncSynValidationConfig 将 SYN 验证配置同步到 syn_config_map eBPF Map
func (da *DefenseApplier) SyncSynValidationConfig(enabled bool, threshold uint32) error {
	synConfigMap := da.loader.GetMap("syn_config_map")
	if synConfigMap == nil {
		return fmt.Errorf("syn_config_map Map 不存在")
	}

	key := uint32(0)
	enabledVal := uint32(0)
	if enabled {
		enabledVal = 1
	}
	value := struct {
		Enabled            uint32
		ChallengeThreshold uint32
	}{
		Enabled:            enabledVal,
		ChallengeThreshold: threshold,
	}

	if err := synConfigMap.Put(&key, &value); err != nil {
		return fmt.Errorf("[L1Defense] 写入 syn_config_map 失败: %w", err)
	}

	log.Printf("[L1Defense] ✅ SYN 验证配置已同步: enabled=%v, threshold=%d", enabled, threshold)
	return nil
}
