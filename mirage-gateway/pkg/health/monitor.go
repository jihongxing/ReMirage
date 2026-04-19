// Package health - 战损自检引擎
// 实时计算链路质量，区分"网络拥塞"与"人为干扰"
package health

import (
	"context"
	"fmt"
	"log"
	"math"
	"sync"
	"time"

	"github.com/cilium/ebpf"
)

// AlertLevel 告警等级
type AlertLevel int

const (
	AlertNone   AlertLevel = 0
	AlertYellow AlertLevel = 1 // 丢包 > 10%
	AlertRed    AlertLevel = 2 // 丢包 > 25% 或连接被 Reset
)

// TCPStats 内核 TCP 统计（从 eBPF 读取）
type TCPStats struct {
	RetransOut uint32  // 重传数
	LostOut    uint32  // 丢包数
	SRTT       uint32  // 平滑 RTT (微秒)
	RTTVar     uint32  // RTT 方差
	TotalSent  uint64  // 总发送包数
	TotalRecv  uint64  // 总接收包数
	ResetCount uint32  // RST 计数
	Timestamp  uint64  // 时间戳
}

// LinkQuality 链路质量评估
type LinkQuality struct {
	LossRate      float64    // 丢包率 (0-100)
	RTTMean       float64    // RTT 均值 (ms)
	RTTJitter     float64    // RTT 抖动 (ms)
	JitterEntropy float64    // 抖动熵（用于检测人为干扰）
	BandwidthUtil float64    // 带宽利用率 (0-100)
	AlertLevel    AlertLevel // 告警等级
	IsPreciseJam  bool       // 是否为精准干扰
}

// HealthMonitor 战损自检引擎
type HealthMonitor struct {
	mu sync.RWMutex

	// eBPF Map 引用
	tcpStatsMap   *ebpf.Map
	healthCtrlMap *ebpf.Map

	// 历史数据（用于计算方差）
	rttHistory    []float64
	lossHistory   []float64
	historySize   int
	historyIdx    int

	// 当前状态
	currentStats  TCPStats
	currentQuality LinkQuality
	lastAlert     AlertLevel

	// 回调
	onYellowAlert func()
	onRedAlert    func()

	// 控制
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewHealthMonitor 创建战损自检引擎
func NewHealthMonitor(tcpStatsMap, healthCtrlMap *ebpf.Map) *HealthMonitor {
	ctx, cancel := context.WithCancel(context.Background())
	return &HealthMonitor{
		tcpStatsMap:   tcpStatsMap,
		healthCtrlMap: healthCtrlMap,
		rttHistory:    make([]float64, 100),
		lossHistory:   make([]float64, 100),
		historySize:   100,
		ctx:           ctx,
		cancel:        cancel,
	}
}

// SetAlertCallbacks 设置告警回调
func (hm *HealthMonitor) SetAlertCallbacks(onYellow, onRed func()) {
	hm.mu.Lock()
	defer hm.mu.Unlock()
	hm.onYellowAlert = onYellow
	hm.onRedAlert = onRed
}

// Start 启动监控
func (hm *HealthMonitor) Start() {
	hm.wg.Add(1)
	go hm.monitorLoop()
	log.Println("🧠 战损自检引擎已启动")
}

// Stop 停止监控
func (hm *HealthMonitor) Stop() {
	hm.cancel()
	hm.wg.Wait()
	log.Println("🛑 战损自检引擎已停止")
}

// monitorLoop 监控循环
func (hm *HealthMonitor) monitorLoop() {
	defer hm.wg.Done()

	ticker := time.NewTicker(100 * time.Millisecond) // 100ms 采样
	defer ticker.Stop()

	for {
		select {
		case <-hm.ctx.Done():
			return
		case <-ticker.C:
			hm.sample()
			hm.evaluate()
		}
	}
}

// sample 采样内核数据
func (hm *HealthMonitor) sample() {
	if hm.tcpStatsMap == nil {
		return
	}

	var stats TCPStats
	key := uint32(0)

	if err := hm.tcpStatsMap.Lookup(&key, &stats); err != nil {
		return
	}

	hm.mu.Lock()
	defer hm.mu.Unlock()

	// 计算增量
	prevStats := hm.currentStats
	hm.currentStats = stats

	// 计算丢包率
	totalSent := stats.TotalSent - prevStats.TotalSent
	if totalSent > 0 {
		lostDelta := uint64(stats.LostOut - prevStats.LostOut)
		lossRate := float64(lostDelta) / float64(totalSent) * 100
		hm.lossHistory[hm.historyIdx%hm.historySize] = lossRate
	}

	// 记录 RTT
	rttMs := float64(stats.SRTT) / 1000.0
	hm.rttHistory[hm.historyIdx%hm.historySize] = rttMs

	hm.historyIdx++
}

// evaluate 评估链路质量
func (hm *HealthMonitor) evaluate() {
	hm.mu.Lock()

	// 计算统计量
	quality := hm.calculateQuality()
	hm.currentQuality = quality

	// 检测精准干扰
	quality.IsPreciseJam = hm.detectPreciseJamming(quality)

	// 判定告警等级
	quality.AlertLevel = hm.determineAlertLevel(quality)

	prevAlert := hm.lastAlert
	hm.lastAlert = quality.AlertLevel

	// 获取回调
	onYellow := hm.onYellowAlert
	onRed := hm.onRedAlert

	hm.mu.Unlock()

	// 触发告警（在锁外执行）
	if quality.AlertLevel > prevAlert {
		switch quality.AlertLevel {
		case AlertYellow:
			log.Printf("⚠️  [Yellow Alert] 丢包率=%.1f%%, RTT=%.1fms±%.1fms",
				quality.LossRate, quality.RTTMean, quality.RTTJitter)
			if onYellow != nil {
				onYellow()
			}
		case AlertRed:
			log.Printf("🚨 [Red Alert] 丢包率=%.1f%%, 精准干扰=%v",
				quality.LossRate, quality.IsPreciseJam)
			if onRed != nil {
				onRed()
			}
		}
	}
}

// calculateQuality 计算链路质量
func (hm *HealthMonitor) calculateQuality() LinkQuality {
	quality := LinkQuality{}

	// 计算丢包率均值
	var lossSum float64
	count := min(hm.historyIdx, hm.historySize)
	for i := 0; i < count; i++ {
		lossSum += hm.lossHistory[i]
	}
	if count > 0 {
		quality.LossRate = lossSum / float64(count)
	}

	// 计算 RTT 均值和方差
	var rttSum, rttSqSum float64
	for i := 0; i < count; i++ {
		rttSum += hm.rttHistory[i]
		rttSqSum += hm.rttHistory[i] * hm.rttHistory[i]
	}
	if count > 0 {
		quality.RTTMean = rttSum / float64(count)
		variance := rttSqSum/float64(count) - quality.RTTMean*quality.RTTMean
		if variance > 0 {
			quality.RTTJitter = math.Sqrt(variance)
		}
	}

	// 计算抖动熵
	quality.JitterEntropy = hm.calculateJitterEntropy()

	return quality
}

// calculateJitterEntropy 计算抖动熵（用于检测人为干扰）
func (hm *HealthMonitor) calculateJitterEntropy() float64 {
	count := min(hm.historyIdx, hm.historySize)
	if count < 10 {
		return 0
	}

	// 计算 RTT 差分序列
	diffs := make([]float64, count-1)
	for i := 1; i < count; i++ {
		diffs[i-1] = hm.rttHistory[i] - hm.rttHistory[i-1]
	}

	// 量化到 bins
	bins := make(map[int]int)
	binSize := 1.0 // 1ms 精度
	for _, d := range diffs {
		bin := int(d / binSize)
		bins[bin]++
	}

	// 计算香农熵
	var entropy float64
	total := float64(len(diffs))
	for _, c := range bins {
		p := float64(c) / total
		if p > 0 {
			entropy -= p * math.Log2(p)
		}
	}

	return entropy
}

// detectPreciseJamming 检测精准干扰
func (hm *HealthMonitor) detectPreciseJamming(quality LinkQuality) bool {
	// 精准干扰特征：
	// 1. RTT 波动剧烈（抖动熵高）
	// 2. 丢包率突然飙升
	// 3. 但带宽并未饱和

	// 抖动熵阈值（正常网络约 2-4，干扰时 > 5）
	if quality.JitterEntropy < 5.0 {
		return false
	}

	// 丢包率阈值
	if quality.LossRate < 5.0 {
		return false
	}

	// 带宽利用率低但丢包高 = 精准干扰
	if quality.BandwidthUtil < 70 && quality.LossRate > 10 {
		return true
	}

	// RTT 方差异常高
	if quality.RTTJitter > quality.RTTMean*0.5 {
		return true
	}

	return false
}

// determineAlertLevel 判定告警等级
func (hm *HealthMonitor) determineAlertLevel(quality LinkQuality) AlertLevel {
	// RST 攻击检测
	if hm.currentStats.ResetCount > 10 {
		return AlertRed
	}

	// 丢包率判定
	if quality.LossRate > 25 {
		return AlertRed
	}
	if quality.LossRate > 10 {
		return AlertYellow
	}

	// 精准干扰直接升级
	if quality.IsPreciseJam {
		return AlertRed
	}

	return AlertNone
}

// GetCurrentQuality 获取当前链路质量
func (hm *HealthMonitor) GetCurrentQuality() LinkQuality {
	hm.mu.RLock()
	defer hm.mu.RUnlock()
	return hm.currentQuality
}

// GetAlertLevel 获取当前告警等级
func (hm *HealthMonitor) GetAlertLevel() AlertLevel {
	hm.mu.RLock()
	defer hm.mu.RUnlock()
	return hm.lastAlert
}

// UpdateHealthCtrl 更新健康控制参数到 eBPF
func (hm *HealthMonitor) UpdateHealthCtrl(paddingDensity, fecRedundancy uint32) error {
	if hm.healthCtrlMap == nil {
		return fmt.Errorf("healthCtrlMap 未初始化")
	}

	type HealthCtrl struct {
		PaddingDensity uint32
		FECRedundancy  uint32
		AlertLevel     uint32
		Timestamp      uint64
	}

	ctrl := HealthCtrl{
		PaddingDensity: paddingDensity,
		FECRedundancy:  fecRedundancy,
		AlertLevel:     uint32(hm.lastAlert),
		Timestamp:      uint64(time.Now().UnixNano()),
	}

	key := uint32(0)
	return hm.healthCtrlMap.Put(&key, &ctrl)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
