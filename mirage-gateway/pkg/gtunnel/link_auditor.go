// Package gtunnel - 链路审计器
// 持续监控各传输路径的丢包率和延迟波动，为 Orchestrator 提供降格/升格决策依据
package gtunnel

import (
	"sync"
	"time"
)

// PathMetrics 路径质量指标
type PathMetrics struct {
	RTT         time.Duration // 最近一次 RTT
	BaselineRTT time.Duration // 基线 RTT（初始连接时记录）
	LossRate    float64       // 丢包率 (0.0~1.0)
	Jitter      time.Duration // 延迟抖动
	LastUpdate  time.Time     // 最后更新时间
	SampleCount int64         // 采样总数

	// 滑动窗口内部状态
	totalSamples int64
	lostSamples  int64
	rttSum       time.Duration
}

// AuditThresholds 审计阈值
type AuditThresholds struct {
	MaxLossRate    float64       // 最大丢包率阈值，默认 0.30 (30%)
	MaxRTTMultiple float64       // RTT 倍数阈值，默认 2.0 (200% 基线)
	WindowSize     time.Duration // 滑动窗口大小，默认 30s
}

// DefaultAuditThresholds 默认审计阈值
func DefaultAuditThresholds() AuditThresholds {
	return AuditThresholds{
		MaxLossRate:    0.30,
		MaxRTTMultiple: 2.0,
		WindowSize:     30 * time.Second,
	}
}

// LinkAuditor 链路审计器
type LinkAuditor struct {
	mu         sync.RWMutex
	metrics    map[TransportType]*PathMetrics
	thresholds AuditThresholds
	onDegrade  func(TransportType, *PathMetrics)
}

// NewLinkAuditor 创建链路审计器
func NewLinkAuditor(thresholds AuditThresholds) *LinkAuditor {
	return &LinkAuditor{
		metrics:    make(map[TransportType]*PathMetrics),
		thresholds: thresholds,
	}
}

// SetDegradeCallback 设置劣化回调
func (la *LinkAuditor) SetDegradeCallback(cb func(TransportType, *PathMetrics)) {
	la.mu.Lock()
	defer la.mu.Unlock()
	la.onDegrade = cb
}

// RecordSample 记录一次采样
func (la *LinkAuditor) RecordSample(t TransportType, rtt time.Duration, lost bool) {
	la.mu.Lock()
	defer la.mu.Unlock()

	m, ok := la.metrics[t]
	if !ok {
		m = &PathMetrics{
			BaselineRTT: rtt,
		}
		la.metrics[t] = m
	}

	m.totalSamples++
	m.SampleCount = m.totalSamples
	if lost {
		m.lostSamples++
	} else {
		m.RTT = rtt
		m.rttSum += rtt
	}

	if m.totalSamples > 0 {
		m.LossRate = float64(m.lostSamples) / float64(m.totalSamples)
	}
	m.LastUpdate = time.Now()

	// 更新基线 RTT（取前 10 个成功采样的平均值）
	successSamples := m.totalSamples - m.lostSamples
	if successSamples > 0 && successSamples <= 10 {
		m.BaselineRTT = m.rttSum / time.Duration(successSamples)
	}
}

// ShouldDegrade 判断是否应该降格
// 返回 true 当且仅当：丢包率 > MaxLossRate 或 RTT > MaxRTTMultiple * BaselineRTT
func (la *LinkAuditor) ShouldDegrade(t TransportType) bool {
	la.mu.RLock()
	defer la.mu.RUnlock()

	m, ok := la.metrics[t]
	if !ok {
		return false
	}

	return la.shouldDegradeMetrics(m)
}

// shouldDegradeMetrics 内部判定逻辑（无锁）
func (la *LinkAuditor) shouldDegradeMetrics(m *PathMetrics) bool {
	if m.LossRate > la.thresholds.MaxLossRate {
		return true
	}
	if m.BaselineRTT > 0 && m.RTT > time.Duration(float64(m.BaselineRTT)*la.thresholds.MaxRTTMultiple) {
		return true
	}
	return false
}

// GetMetrics 获取指定路径的质量指标（返回副本）
func (la *LinkAuditor) GetMetrics(t TransportType) *PathMetrics {
	la.mu.RLock()
	defer la.mu.RUnlock()

	m, ok := la.metrics[t]
	if !ok {
		return nil
	}

	// 返回副本
	cp := *m
	return &cp
}

// Reset 重置指定路径的指标
func (la *LinkAuditor) Reset(t TransportType) {
	la.mu.Lock()
	defer la.mu.Unlock()
	delete(la.metrics, t)
}

// CheckShouldPromote 检查是否满足升格条件
// probeResults 为连续探测结果序列，threshold 为连续成功次数阈值
func CheckShouldPromote(probeResults []bool, threshold int) bool {
	consecutive := 0
	for _, ok := range probeResults {
		if ok {
			consecutive++
			if consecutive >= threshold {
				return true
			}
		} else {
			consecutive = 0
		}
	}
	return false
}
