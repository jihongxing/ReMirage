package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/spf13/cobra"
)

// QuotaInfo 配额信息
type QuotaInfo struct {
	RemainingBytes  uint64  `json:"remaining_bytes"`
	TotalBytes      uint64  `json:"total_bytes"`
	UsedBytes       uint64  `json:"used_bytes"`
	UsagePercent    float64 `json:"usage_percent"`
	BusinessBytes   uint64  `json:"business_bytes"`
	DefenseBytes    uint64  `json:"defense_bytes"`
	DefenseOverhead float64 `json:"defense_overhead"` // 防御开销比
	ExpiresAt       string  `json:"expires_at"`
	Throttled       bool    `json:"throttled"`
}

var quotaCmd = &cobra.Command{
	Use:   "quota",
	Short: "配额与流量查询",
	Long:  "查看当前配额使用情况、流量统计、防御开销",
	RunE: func(cmd *cobra.Command, args []string) error {
		url := fmt.Sprintf("http://%s/api/quota", gatewayAddr)
		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Get(url)
		if err != nil {
			return fmt.Errorf("无法连接 Gateway: %w", err)
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("读取响应失败: %w", err)
		}

		var quota QuotaInfo
		if err := json.Unmarshal(body, &quota); err != nil {
			fmt.Printf("原始响应:\n%s\n", string(body))
			return nil
		}

		// 配额条
		barLen := 30
		usedLen := int(quota.UsagePercent / 100 * float64(barLen))
		if usedLen > barLen {
			usedLen = barLen
		}

		barColor := "🟢"
		if quota.UsagePercent > 90 {
			barColor = "🔴"
		} else if quota.UsagePercent > 70 {
			barColor = "🟡"
		}

		fmt.Printf("%s 配额使用: %.1f%%\n", barColor, quota.UsagePercent)
		fmt.Printf("  [%s%s] %s / %s\n",
			repeat("█", usedLen), repeat("░", barLen-usedLen),
			humanBytes(int64(quota.UsedBytes)), humanBytes(int64(quota.TotalBytes)))
		fmt.Printf("  剩余:       %s\n", humanBytes(int64(quota.RemainingBytes)))
		fmt.Printf("  业务流量:   %s\n", humanBytes(int64(quota.BusinessBytes)))
		fmt.Printf("  防御流量:   %s\n", humanBytes(int64(quota.DefenseBytes)))
		fmt.Printf("  防御开销:   %.1f%%\n", quota.DefenseOverhead*100)
		if quota.ExpiresAt != "" {
			fmt.Printf("  到期时间:   %s\n", quota.ExpiresAt)
		}
		if quota.Throttled {
			fmt.Println("  ⚠️  流量已被限速")
		}

		return nil
	},
}

func repeat(s string, n int) string {
	result := ""
	for i := 0; i < n; i++ {
		result += s
	}
	return result
}
