//go:build linux

package ebpf

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// HealthThresholds 健康阈值
type HealthThresholds struct {
	MaxSoftIRQDelta  uint64        // SoftIRQ 增量阈值
	MaxDropRate      float64       // 丢包率阈值
	MaxRingBufErrors uint64        // Ring Buffer 错误阈值
	CheckInterval    time.Duration // 采样间隔
}

// DefaultHealthThresholds 默认阈值
func DefaultHealthThresholds() HealthThresholds {
	return HealthThresholds{
		MaxSoftIRQDelta:  100000,
		MaxDropRate:      0.05,
		MaxRingBufErrors: 1000,
		CheckInterval:    10 * time.Second,
	}
}

// HealthMetrics 健康指标
type HealthMetrics struct {
	SoftIRQDelta  uint64
	DropRate      float64
	RingBufErrors uint64
	LastCheck     time.Time
}

// HealthStatus 单个程序健康状态
type HealthStatus struct {
	Name    string
	Healthy bool
	Metrics HealthMetrics
}

// HealthChecker eBPF 程序健康检查器
type HealthChecker struct {
	mu         sync.RWMutex
	programs   map[string]*HealthStatus
	thresholds HealthThresholds
	fallbackFn func(progName string) error
	loader     *Loader
}

// NewHealthChecker 创建健康检查器
func NewHealthChecker(loader *Loader, thresholds HealthThresholds, fallbackFn func(string) error) *HealthChecker {
	return &HealthChecker{
		programs:   make(map[string]*HealthStatus),
		thresholds: thresholds,
		fallbackFn: fallbackFn,
		loader:     loader,
	}
}

// RegisterProgram 注册需要监控的 eBPF 程序
func (hc *HealthChecker) RegisterProgram(name string) {
	hc.mu.Lock()
	defer hc.mu.Unlock()
	hc.programs[name] = &HealthStatus{Name: name, Healthy: true}
}

// Start 启动健康检查循环
func (hc *HealthChecker) Start(ctx context.Context) {
	ticker := time.NewTicker(hc.thresholds.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			hc.check()
		}
	}
}

// check 执行一次健康检查
func (hc *HealthChecker) check() {
	dropRate := hc.readDropRate()
	softIRQ := hc.readSoftIRQDelta()

	hc.mu.Lock()
	defer hc.mu.Unlock()

	for name, status := range hc.programs {
		if !status.Healthy {
			continue // 已摘钩，跳过
		}

		metrics := HealthMetrics{
			DropRate:     dropRate,
			SoftIRQDelta: softIRQ,
			LastCheck:    time.Now(),
		}
		status.Metrics = metrics

		// 检查是否超过阈值
		if dropRate > hc.thresholds.MaxDropRate || softIRQ > hc.thresholds.MaxSoftIRQDelta {
			log.Printf("[HealthChecker] 🚨 程序 %s 健康异常: dropRate=%.4f, softIRQ=%d", name, dropRate, softIRQ)
			status.Healthy = false
			if hc.fallbackFn != nil {
				if err := hc.fallbackFn(name); err != nil {
					log.Printf("[HealthChecker] ❌ 程序 %s fallback 失败: %v", name, err)
				} else {
					log.Printf("[HealthChecker] ✅ 程序 %s 已切换到 iptables fallback", name)
				}
			}
		}
	}
}

// GetStatus 获取指定程序的健康状态
func (hc *HealthChecker) GetStatus(name string) (*HealthStatus, bool) {
	hc.mu.RLock()
	defer hc.mu.RUnlock()
	s, ok := hc.programs[name]
	return s, ok
}

// IsHealthy 检查指定程序是否健康
func (hc *HealthChecker) IsHealthy(name string) bool {
	hc.mu.RLock()
	defer hc.mu.RUnlock()
	if s, ok := hc.programs[name]; ok {
		return s.Healthy
	}
	return false
}

// readDropRate 从 /proc/net/dev 读取丢包率
func (hc *HealthChecker) readDropRate() float64 {
	data, err := os.ReadFile("/proc/net/dev")
	if err != nil {
		return 0
	}
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.Contains(line, hc.loader.iface) {
			fields := strings.Fields(line)
			if len(fields) >= 12 {
				rxPackets, _ := strconv.ParseUint(fields[2], 10, 64)
				rxDrop, _ := strconv.ParseUint(fields[4], 10, 64)
				if rxPackets > 0 {
					return float64(rxDrop) / float64(rxPackets)
				}
			}
		}
	}
	return 0
}

// readSoftIRQDelta 从 /proc/net/softnet_stat 读取 SoftIRQ 统计
func (hc *HealthChecker) readSoftIRQDelta() uint64 {
	data, err := os.ReadFile("/proc/net/softnet_stat")
	if err != nil {
		return 0
	}
	lines := strings.Split(string(data), "\n")
	var total uint64
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			val, err := strconv.ParseUint(fields[1], 16, 64)
			if err == nil {
				total += val
			}
		}
	}
	return total
}

// DetachProgram 手动摘除指定程序
func (hc *HealthChecker) DetachProgram(name string) error {
	hc.mu.Lock()
	defer hc.mu.Unlock()
	if s, ok := hc.programs[name]; ok {
		s.Healthy = false
		return nil
	}
	return fmt.Errorf("程序 %s 未注册", name)
}
