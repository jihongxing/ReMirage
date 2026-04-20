// Package strategy - Gateway 生命周期管理
package strategy

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"
)

// CellPhase 蜂窝阶段
type CellPhase int

const (
	PhaseIncubation  CellPhase = 0 // 潜伏期
	PhaseCalibration CellPhase = 1 // 校准期
	PhaseActive      CellPhase = 2 // 服役期
)

const (
	CalibrationSamples = 100 // 校准期采样次数
)

// EBPFPhaseUpdater eBPF 阶段更新接口（解耦 ebpf 包依赖）
type EBPFPhaseUpdater interface {
	UpdatePhaseMap(phase uint32) error
}

// VPCNoiseInjector VPC 噪声注入接口（通过 eBPF Map 控制 C 数据面）
type VPCNoiseInjector interface {
	// SetNoiseProfile 设置噪声配置（写入 vpc_noise_profiles Map）
	SetNoiseProfile(regionID uint32, profile *NoiseProfile) error
	// SetActiveProfile 激活指定区域配置（写入 active_noise_profile Map）
	SetActiveProfile(regionID uint32) error
	// GetNoiseStats 读取噪声统计
	GetNoiseStats() (*NoiseStats, error)
}

// NoiseProfile VPC 噪声配置（对应 C 结构体 vpc_noise_profile）
type NoiseProfile struct {
	FiberBaseUs      uint32 // 光缆基础延迟（微秒）
	FiberVarianceUs  uint32 // 光缆抖动方差
	RouterHops       uint32 // 路由器跳数
	RouterQueueUs    uint32 // 每跳队列延迟
	CongestionFactor uint32 // 拥塞因子 (0-100)
	PacketLossRate   uint32 // 丢包率 (千分比)
	ReorderRate      uint32 // 乱序率 (0-100)
	DuplicateRate    uint32 // 重复率 (0-100)
}

// NoiseStats VPC 噪声统计
type NoiseStats struct {
	TotalPackets      uint64
	DelayedPackets    uint64
	TotalDelayUs      uint64
	DroppedPackets    uint64
	ReorderedPackets  uint64
	DuplicatedPackets uint64
}

// NetworkProber 网络质量探测接口
type NetworkProber interface {
	// ProbeRTT 发送 ICMP/TCP 探测并返回 RTT（微秒）
	ProbeRTT(target string) (int, error)
	// ProbePacketLoss 发送 N 个探测包并返回丢包率
	ProbePacketLoss(target string, count int) (float64, error)
}

// CalibrationReporter 校准结果上报接口（上报到 Mirage-OS）
type CalibrationReporter interface {
	// ReportCalibration 上报校准结果，返回 OS 下发的 B-DNA 模板调整参数
	ReportCalibration(avgRTT int, avgLoss float64) (*DNATuning, error)
}

// DNATuning B-DNA 模板微调参数（由 Mirage-OS 下发）
type DNATuning struct {
	TemplateID      uint32 // 推荐模板 ID
	IATMeanUs       uint32 // 调整后的 IAT 均值
	IATSigmaUs      uint32 // 调整后的 IAT 标准差
	PaddingStrategy uint32 // 填充策略
	TargetMTU       uint16 // 目标 MTU
}

// DNATemplateUpdater B-DNA 模板更新接口（写入 eBPF Map）
type DNATemplateUpdater interface {
	UpdateDNATemplate(templateID uint32, tuning *DNATuning) error
}

// CellLifecycleManager Gateway 生命周期管理器
type CellLifecycleManager struct {
	phase       CellPhase
	ebpfUpdater EBPFPhaseUpdater
	ctx         context.Context
	cancel      context.CancelFunc

	// 依赖注入
	noiseInjector VPCNoiseInjector
	networkProber NetworkProber
	calibReporter CalibrationReporter
	dnaUpdater    DNATemplateUpdater
	probeTarget   string // 探测目标地址

	// 校准期统计
	rttSamples   []int
	packetLosses []float64
	sampleCount  int
}

// CellLifecycleOption 配置选项
type CellLifecycleOption func(*CellLifecycleManager)

func WithVPCNoiseInjector(injector VPCNoiseInjector) CellLifecycleOption {
	return func(m *CellLifecycleManager) { m.noiseInjector = injector }
}

func WithNetworkProber(prober NetworkProber) CellLifecycleOption {
	return func(m *CellLifecycleManager) { m.networkProber = prober }
}

func WithCalibrationReporter(reporter CalibrationReporter) CellLifecycleOption {
	return func(m *CellLifecycleManager) { m.calibReporter = reporter }
}

func WithDNATemplateUpdater(updater DNATemplateUpdater) CellLifecycleOption {
	return func(m *CellLifecycleManager) { m.dnaUpdater = updater }
}

func WithProbeTarget(target string) CellLifecycleOption {
	return func(m *CellLifecycleManager) { m.probeTarget = target }
}

// NewCellLifecycleManager 创建生命周期管理器
func NewCellLifecycleManager(ebpfUpdater EBPFPhaseUpdater, opts ...CellLifecycleOption) *CellLifecycleManager {
	ctx, cancel := context.WithCancel(context.Background())
	m := &CellLifecycleManager{
		phase:        PhaseIncubation,
		ebpfUpdater:  ebpfUpdater,
		ctx:          ctx,
		cancel:       cancel,
		probeTarget:  "8.8.8.8",
		rttSamples:   make([]int, 0, CalibrationSamples),
		packetLosses: make([]float64, 0, CalibrationSamples),
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// Start 启动生命周期管理
func (m *CellLifecycleManager) Start(initialPhase CellPhase) error {
	m.phase = initialPhase

	log.Printf("[CellLifecycle] 启动生命周期管理，当前阶段: %d", initialPhase)

	// 更新 eBPF Map
	if err := m.updateEBPFPhase(); err != nil {
		return fmt.Errorf("更新 eBPF 阶段失败: %w", err)
	}

	switch initialPhase {
	case PhaseIncubation:
		go m.runIncubationPhase()
	case PhaseCalibration:
		go m.runCalibrationPhase()
	case PhaseActive:
		go m.runActivePhase()
	}

	return nil
}

// Stop 停止生命周期管理
func (m *CellLifecycleManager) Stop() {
	log.Println("[CellLifecycle] 停止生命周期管理")
	m.cancel()
}

// runIncubationPhase 潜伏期：VPC 噪声注入
func (m *CellLifecycleManager) runIncubationPhase() {
	log.Println("[CellLifecycle] 进入潜伏期：开始 VPC 噪声注入")

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			// 注入背景噪声
			m.injectBackgroundNoise()
		}
	}
}

// injectBackgroundNoise 注入背景噪声（通过 eBPF Map 控制 VPC 数据面）
func (m *CellLifecycleManager) injectBackgroundNoise() {
	if m.noiseInjector == nil {
		log.Println("[CellLifecycle] VPC 噪声注入器未设置，跳过")
		return
	}

	// 潜伏期使用低强度噪声配置，模拟机房内正常流量
	// 建立"邻里信誉"：让流量特征与同 VPC 内其他实例一致
	profile := &NoiseProfile{
		FiberBaseUs:      200, // 200μs 基础延迟（同城机房）
		FiberVarianceUs:  50,  // 50μs 抖动
		RouterHops:       2,   // 2 跳（同 VPC 内）
		RouterQueueUs:    100, // 100μs 队列延迟
		CongestionFactor: 10,  // 低拥塞
		PacketLossRate:   1,   // 0.1% 丢包（正常水平）
		ReorderRate:      0,   // 无乱序
		DuplicateRate:    0,   // 无重复
	}

	// 写入 Region 0（本地/同城）配置
	if err := m.noiseInjector.SetNoiseProfile(0, profile); err != nil {
		log.Printf("[CellLifecycle] 设置噪声配置失败: %v", err)
		return
	}

	// 激活 Region 0
	if err := m.noiseInjector.SetActiveProfile(0); err != nil {
		log.Printf("[CellLifecycle] 激活噪声配置失败: %v", err)
		return
	}

	log.Println("[CellLifecycle] VPC 噪声注入已激活（潜伏期低强度模式）")
}

// runCalibrationPhase 校准期：网络质量测量
func (m *CellLifecycleManager) runCalibrationPhase() {
	log.Println("[CellLifecycle] 进入校准期：开始网络质量测量")

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.measureNetworkQuality()
		}
	}
}

// measureNetworkQuality 测量网络质量（真实探测）
func (m *CellLifecycleManager) measureNetworkQuality() {
	var rtt int
	var packetLoss float64

	if m.networkProber != nil {
		// 使用注入的探测器进行真实测量
		var err error
		rtt, err = m.networkProber.ProbeRTT(m.probeTarget)
		if err != nil {
			log.Printf("[CellLifecycle] RTT 探测失败: %v，使用 ICMP fallback", err)
			rtt = m.fallbackProbeRTT()
		}

		packetLoss, err = m.networkProber.ProbePacketLoss(m.probeTarget, 10)
		if err != nil {
			log.Printf("[CellLifecycle] 丢包探测失败: %v", err)
			packetLoss = 0.0
		}
	} else {
		// Fallback：使用原生 ICMP 探测
		rtt = m.fallbackProbeRTT()
		packetLoss = 0.0
	}

	m.rttSamples = append(m.rttSamples, rtt)
	m.packetLosses = append(m.packetLosses, packetLoss)
	m.sampleCount++

	log.Printf("[CellLifecycle] 网络测量 #%d: RTT=%dμs, 丢包=%.4f",
		m.sampleCount, rtt, packetLoss)

	// 达到采样数后计算平均值并上报
	if m.sampleCount >= CalibrationSamples {
		avgRTT := m.calculateAvgRTT()
		avgLoss := m.calculateAvgPacketLoss()

		log.Printf("[CellLifecycle] 校准完成: 平均RTT=%dμs, 平均丢包=%.4f",
			avgRTT, avgLoss)

		// 上报到 Mirage-OS 并获取 B-DNA 模板微调参数
		m.reportAndTuneDNA(avgRTT, avgLoss)
	}
}

// fallbackProbeRTT 原生 TCP 连接探测 RTT（不依赖外部接口）
func (m *CellLifecycleManager) fallbackProbeRTT() int {
	start := time.Now()
	conn, err := net.DialTimeout("tcp", m.probeTarget+":443", 3*time.Second)
	if err != nil {
		return 50000 // 默认 50ms
	}
	conn.Close()
	return int(time.Since(start).Microseconds())
}

// reportAndTuneDNA 上报校准结果并微调 B-DNA 模板
func (m *CellLifecycleManager) reportAndTuneDNA(avgRTT int, avgLoss float64) {
	if m.calibReporter == nil {
		log.Println("[CellLifecycle] 校准上报器未设置，跳过上报")
		return
	}

	tuning, err := m.calibReporter.ReportCalibration(avgRTT, avgLoss)
	if err != nil {
		log.Printf("[CellLifecycle] 校准结果上报失败: %v", err)
		return
	}

	if tuning == nil {
		log.Println("[CellLifecycle] OS 未返回模板调整参数")
		return
	}

	// 将 OS 下发的参数写入 eBPF dna_template_map
	if m.dnaUpdater != nil {
		if err := m.dnaUpdater.UpdateDNATemplate(tuning.TemplateID, tuning); err != nil {
			log.Printf("[CellLifecycle] B-DNA 模板更新失败: %v", err)
		} else {
			log.Printf("[CellLifecycle] B-DNA 模板已微调: template=%d, IAT=%dμs±%dμs, MTU=%d",
				tuning.TemplateID, tuning.IATMeanUs, tuning.IATSigmaUs, tuning.TargetMTU)
		}
	}
}

// calculateAvgRTT 计算平均 RTT
func (m *CellLifecycleManager) calculateAvgRTT() int {
	if len(m.rttSamples) == 0 {
		return 0
	}

	sum := 0
	for _, rtt := range m.rttSamples {
		sum += rtt
	}

	return sum / len(m.rttSamples)
}

// calculateAvgPacketLoss 计算平均丢包率
func (m *CellLifecycleManager) calculateAvgPacketLoss() float64 {
	if len(m.packetLosses) == 0 {
		return 0
	}

	sum := 0.0
	for _, loss := range m.packetLosses {
		sum += loss
	}

	return sum / float64(len(m.packetLosses))
}

// runActivePhase 服役期：承载真实流量
func (m *CellLifecycleManager) runActivePhase() {
	log.Println("[CellLifecycle] 进入服役期：开始承载真实流量")

	// 服役期主要由其他模块处理
	// 这里只需要保持心跳和监控
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			log.Println("[CellLifecycle] 服役期心跳")
		}
	}
}

// updateEBPFPhase 更新 eBPF Map 中的阶段
func (m *CellLifecycleManager) updateEBPFPhase() error {
	if m.ebpfUpdater == nil {
		log.Printf("[CellLifecycle] eBPF updater 未设置，跳过阶段更新: %d", m.phase)
		return nil
	}

	if err := m.ebpfUpdater.UpdatePhaseMap(uint32(m.phase)); err != nil {
		return fmt.Errorf("写入 cell_phase_map 失败: %w", err)
	}

	log.Printf("[CellLifecycle] eBPF 阶段已更新: %d", m.phase)
	return nil
}

// TransitionTo 转换到新阶段
func (m *CellLifecycleManager) TransitionTo(newPhase CellPhase) error {
	if m.phase == newPhase {
		return nil
	}

	log.Printf("[CellLifecycle] 阶段转换: %d → %d", m.phase, newPhase)

	// 停止当前阶段
	m.cancel()

	// 更新阶段
	m.phase = newPhase

	// 更新 eBPF
	if err := m.updateEBPFPhase(); err != nil {
		return err
	}

	// 重新创建 context
	m.ctx, m.cancel = context.WithCancel(context.Background())

	// 启动新阶段
	switch newPhase {
	case PhaseIncubation:
		go m.runIncubationPhase()
	case PhaseCalibration:
		go m.runCalibrationPhase()
	case PhaseActive:
		go m.runActivePhase()
	}

	return nil
}

// GetPhase 获取当前阶段
func (m *CellLifecycleManager) GetPhase() CellPhase {
	return m.phase
}

// GetCalibrationProgress 获取校准进度
func (m *CellLifecycleManager) GetCalibrationProgress() float64 {
	if m.phase != PhaseCalibration {
		return 0
	}

	return float64(m.sampleCount) / float64(CalibrationSamples) * 100.0
}
