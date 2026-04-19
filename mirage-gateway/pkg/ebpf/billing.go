// Package ebpf - 计费模块
// 负责从内核读取流量统计并上报到 Mirage-OS
package ebpf

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

// BillingReporter 计费上报器
type BillingReporter struct {
	loader         *Loader
	reportInterval time.Duration
	stopCh         chan struct{}
	mirageOSURL    string
	gatewayID      string
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
	}
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

// reportTraffic 上报流量统计
func (br *BillingReporter) reportTraffic() error {
	// 1. 从内核读取流量统计
	baseKey := uint32(0)
	defenseKey := uint32(1)

	var baseBytes, defenseBytes uint64

	trafficMap := br.loader.GetMap("traffic_stats")
	if trafficMap == nil {
		return fmt.Errorf("traffic_stats Map 不存在")
	}

	if err := trafficMap.Lookup(&baseKey, &baseBytes); err != nil {
		// Map 可能为空，不报错
		baseBytes = 0
	}

	if err := trafficMap.Lookup(&defenseKey, &defenseBytes); err != nil {
		defenseBytes = 0
	}

	// 2. 构建报告
	report := &TrafficReport{
		GatewayID:           br.gatewayID,
		Timestamp:           time.Now().Unix(),
		BaseTrafficBytes:    baseBytes,
		DefenseTrafficBytes: defenseBytes,
		CellLevel:           "standard", // TODO: 从配置读取
	}

	// 3. 上报到 Mirage-OS
	if br.mirageOSURL != "" {
		if err := br.postTrafficReport(report); err != nil {
			log.Printf("⚠️  [计费] HTTP 上报失败: %v", err)
		} else {
			log.Printf("📊 [计费] 上报流量: 业务=%d字节, 防御=%d字节",
				report.BaseTrafficBytes, report.DefenseTrafficBytes)
		}
	} else {
		// 本地模式：仅在有流量时输出
		if report.BaseTrafficBytes > 0 || report.DefenseTrafficBytes > 0 {
			log.Printf("📊 [计费] 流量统计: 业务=%.2f MB, 防御=%.2f MB",
				float64(report.BaseTrafficBytes)/(1024*1024),
				float64(report.DefenseTrafficBytes)/(1024*1024))
		}
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

// postTrafficReport 上报流量到 Mirage-OS
func (br *BillingReporter) postTrafficReport(report *TrafficReport) error {
	body, err := json.Marshal(report)
	if err != nil {
		return fmt.Errorf("序列化失败: %w", err)
	}

	url := fmt.Sprintf("%s/api/v1/traffic/report", br.mirageOSURL)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Gateway-ID", br.gatewayID)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("上报返回 %d", resp.StatusCode)
	}
	return nil
}

// fetchQuota 从 Mirage-OS 获取配额
func (br *BillingReporter) fetchQuota(quota *QuotaStatus) error {
	url := fmt.Sprintf("%s/api/v1/quota/%s", br.mirageOSURL, br.gatewayID)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Gateway-ID", br.gatewayID)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("获取配额返回 %d", resp.StatusCode)
	}

	return json.NewDecoder(resp.Body).Decode(quota)
}
