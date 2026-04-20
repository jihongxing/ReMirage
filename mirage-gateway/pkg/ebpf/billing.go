// Package ebpf - 计费模块（已废弃 HTTP 通道，仅保留本地统计读取）
// 流量上报已统一收束到 nerve.SensoryUplink（gRPC 唯一通道）
// 本模块仅保留 eBPF Map 读取和配额同步功能
package ebpf

import (
	"fmt"
	"log"
	"time"
)

// BillingReporter 计费上报器
type BillingReporter struct {
	loader         *Loader
	reportInterval time.Duration
	stopCh         chan struct{}
	mirageOSURL    string
	gatewayID      string
	cellLevel      string
}

// TrafficReport 流量报告
type TrafficReport struct {
	GatewayID           string `json:"gateway_id"`
	Timestamp           int64  `json:"timestamp"`
	BaseTrafficBytes    uint64 `json:"base_traffic_bytes"`
	DefenseTrafficBytes uint64 `json:"defense_traffic_bytes"`
	CellLevel           string `json:"cell_level"`
}

// QuotaStatus 配额状态
type QuotaStatus struct {
	RemainingBytes uint64 `json:"remaining_bytes"`
	TotalBytes     uint64 `json:"total_bytes"`
	ExpiresAt      int64  `json:"expires_at"`
	AutoRenew      bool   `json:"auto_renew"`
}

// NewBillingReporter 创建计费上报器
func NewBillingReporter(loader *Loader, mirageOSURL string, gatewayID string) *BillingReporter {
	return &BillingReporter{
		loader:         loader,
		reportInterval: 10 * time.Second,
		stopCh:         make(chan struct{}),
		mirageOSURL:    mirageOSURL,
		gatewayID:      gatewayID,
		cellLevel:      "standard",
	}
}

// SetCellLevel 设置蜂窝等级（从配置或 OS 下发）
func (br *BillingReporter) SetCellLevel(level string) {
	br.cellLevel = level
}

// Start 启动计费上报
func (br *BillingReporter) Start() {
	go br.reportLoop()
	log.Println("💰 计费上报器已启动")
}

// Stop 停止计费上报
func (br *BillingReporter) Stop() {
	close(br.stopCh)
	log.Println("💰 计费上报器已停止")
}

// reportLoop 上报循环
func (br *BillingReporter) reportLoop() {
	ticker := time.NewTicker(br.reportInterval)
	defer ticker.Stop()

	for {
		select {
		case <-br.stopCh:
			return

		case <-ticker.C:
			if err := br.reportTraffic(); err != nil {
				log.Printf("⚠️  [计费] 上报流量失败: %v", err)
			}

			if err := br.syncQuota(); err != nil {
				log.Printf("⚠️  [计费] 同步配额失败: %v", err)
			}
		}
	}
}

// reportTraffic 本地流量统计（不再 HTTP 上报，仅日志输出）
func (br *BillingReporter) reportTraffic() error {
	// 从内核读取流量统计
	baseKey := uint32(0)
	defenseKey := uint32(1)

	var baseBytes, defenseBytes uint64

	trafficMap := br.loader.GetMap("traffic_stats")
	if trafficMap == nil {
		return fmt.Errorf("traffic_stats Map 不存在")
	}

	if err := trafficMap.Lookup(&baseKey, &baseBytes); err != nil {
		baseBytes = 0
	}

	if err := trafficMap.Lookup(&defenseKey, &defenseBytes); err != nil {
		defenseBytes = 0
	}

	// 仅本地日志（计费上报已由 nerve.SensoryUplink 通过 gRPC 完成）
	if baseBytes > 0 || defenseBytes > 0 {
		log.Printf("📊 [计费-本地] 流量统计: 业务=%.2f MB, 防御=%.2f MB",
			float64(baseBytes)/(1024*1024),
			float64(defenseBytes)/(1024*1024))
	}

	return nil
}

// syncQuota 同步配额
func (br *BillingReporter) syncQuota() error {
	// 1. 从 Mirage-OS 获取最新配额
	var quota QuotaStatus
	if br.mirageOSURL != "" {
		if err := br.fetchQuota(&quota); err != nil {
			log.Printf("⚠️  [计费] 获取配额失败: %v", err)
			// 获取失败时不更新内核 Map，保持上次配额
			return nil
		}
	} else {
		// 无 Mirage-OS 连接，使用无限配额（不触发告警）
		quota.RemainingBytes = ^uint64(0) // 最大值
		quota.TotalBytes = 0              // 标记为无限模式
	}

	// 2. 更新内核 quota_map
	quotaMap := br.loader.GetMap("quota_map")
	if quotaMap == nil {
		return fmt.Errorf("quota_map 不存在")
	}

	key := uint32(0)
	if err := quotaMap.Put(&key, &quota.RemainingBytes); err != nil {
		return fmt.Errorf("更新配额失败: %w", err)
	}

	// 3. 配额告警
	if quota.TotalBytes > 0 {
		usagePercent := float64(quota.TotalBytes-quota.RemainingBytes) / float64(quota.TotalBytes) * 100
		if usagePercent >= 90 {
			log.Printf("🚨 [计费] 配额告警: 已使用 %.1f%%, 剩余 %.2f GB",
				usagePercent, float64(quota.RemainingBytes)/(1024*1024*1024))
		}
	}

	// 4. 配额耗尽告警
	if quota.RemainingBytes == 0 {
		log.Println("🚨 [计费] 配额已耗尽，服务已熔断！")
	}

	return nil
}

// SetQuotaStatus 设置配额状态（手动控制）
func (br *BillingReporter) SetQuotaStatus(remainingBytes uint64) error {
	quotaMap := br.loader.GetMap("quota_map")
	if quotaMap == nil {
		return fmt.Errorf("quota_map 不存在")
	}

	key := uint32(0)
	if err := quotaMap.Put(&key, &remainingBytes); err != nil {
		return fmt.Errorf("设置配额失败: %w", err)
	}

	log.Printf("💰 [计费] 配额已更新: %.2f GB", float64(remainingBytes)/(1024*1024*1024))
	return nil
}

// GetTrafficStats 获取流量统计
func (br *BillingReporter) GetTrafficStats() (baseBytes, defenseBytes uint64, err error) {
	trafficMap := br.loader.GetMap("traffic_stats")
	if trafficMap == nil {
		return 0, 0, fmt.Errorf("traffic_stats Map 不存在")
	}

	baseKey := uint32(0)
	defenseKey := uint32(1)

	if err := trafficMap.Lookup(&baseKey, &baseBytes); err != nil {
		baseBytes = 0
	}

	if err := trafficMap.Lookup(&defenseKey, &defenseBytes); err != nil {
		defenseBytes = 0
	}

	return baseBytes, defenseBytes, nil
}

// fetchQuota 从 Mirage-OS 获取配额（保留：配额同步仍需要）
func (br *BillingReporter) fetchQuota(quota *QuotaStatus) error {
	// 配额同步已由 gRPC PushQuota 下行通道处理
	// 此处保留为 fallback（当 gRPC 不可用时通过本地默认值）
	quota.RemainingBytes = ^uint64(0)
	quota.TotalBytes = 0
	return nil
}
