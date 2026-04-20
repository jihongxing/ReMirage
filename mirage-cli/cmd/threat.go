package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/spf13/cobra"
)

// ThreatEvent 威胁事件
type ThreatEvent struct {
	Timestamp   int64  `json:"timestamp"`
	Type        string `json:"type"`
	SourceIP    string `json:"source_ip"`
	SourcePort  int    `json:"source_port"`
	Severity    int    `json:"severity"`
	PacketCount int    `json:"packet_count"`
	Action      string `json:"action"` // blocked/logged/escalated
}

// ThreatSummary 威胁摘要
type ThreatSummary struct {
	CurrentLevel  int    `json:"current_level"`
	TotalEvents   int64  `json:"total_events"`
	BlockedIPs    int    `json:"blocked_ips"`
	Last24h       int64  `json:"last_24h"`
	TopThreatType string `json:"top_threat_type"`
	CortexActive  bool   `json:"cortex_active"`
	PhantomActive bool   `json:"phantom_active"`
}

var threatCmd = &cobra.Command{
	Use:   "threat",
	Short: "威胁事件查看",
	Long:  "查看威胁检测事件、黑名单状态、Cortex 感知中枢状态",
}

var threatSummaryCmd = &cobra.Command{
	Use:   "summary",
	Short: "威胁摘要",
	RunE: func(cmd *cobra.Command, args []string) error {
		url := fmt.Sprintf("http://%s/api/threat/summary", gatewayAddr)
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

		var summary ThreatSummary
		if err := json.Unmarshal(body, &summary); err != nil {
			fmt.Printf("原始响应:\n%s\n", string(body))
			return nil
		}

		levelIcon := "🟢"
		if summary.CurrentLevel >= 3 {
			levelIcon = "🔴"
		} else if summary.CurrentLevel >= 2 {
			levelIcon = "🟠"
		} else if summary.CurrentLevel >= 1 {
			levelIcon = "🟡"
		}

		fmt.Printf("%s 威胁等级: %d\n", levelIcon, summary.CurrentLevel)
		fmt.Printf("  总事件数:     %d\n", summary.TotalEvents)
		fmt.Printf("  24h 事件:     %d\n", summary.Last24h)
		fmt.Printf("  封禁 IP 数:   %d\n", summary.BlockedIPs)
		fmt.Printf("  主要威胁类型: %s\n", summary.TopThreatType)
		fmt.Printf("  Cortex 中枢:  %v\n", summary.CortexActive)
		fmt.Printf("  Phantom 欺骗: %v\n", summary.PhantomActive)

		return nil
	},
}

var threatListCmd = &cobra.Command{
	Use:   "list",
	Short: "列出最近威胁事件",
	RunE: func(cmd *cobra.Command, args []string) error {
		limit, _ := cmd.Flags().GetInt("limit")
		url := fmt.Sprintf("http://%s/api/threat/events?limit=%d", gatewayAddr, limit)
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

		var events []ThreatEvent
		if err := json.Unmarshal(body, &events); err != nil {
			fmt.Printf("原始响应:\n%s\n", string(body))
			return nil
		}

		if len(events) == 0 {
			fmt.Println("暂无威胁事件")
			return nil
		}

		fmt.Printf("%-20s %-16s %-18s %-8s %-10s\n", "时间", "类型", "来源", "严重度", "动作")
		fmt.Println("────────────────────────────────────────────────────────────────────────────")
		for _, e := range events {
			t := time.Unix(e.Timestamp, 0).Format("01-02 15:04:05")
			fmt.Printf("%-20s %-16s %-15s:%-4d %-8d %-10s\n",
				t, e.Type, e.SourceIP, e.SourcePort, e.Severity, e.Action)
		}

		return nil
	},
}

var threatBlacklistCmd = &cobra.Command{
	Use:   "blacklist",
	Short: "查看当前黑名单",
	RunE: func(cmd *cobra.Command, args []string) error {
		url := fmt.Sprintf("http://%s/api/threat/blacklist", gatewayAddr)
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

		// 直接输出 JSON 格式化
		var raw json.RawMessage
		if err := json.Unmarshal(body, &raw); err != nil {
			fmt.Printf("%s\n", string(body))
			return nil
		}
		formatted, _ := json.MarshalIndent(raw, "", "  ")
		fmt.Println(string(formatted))

		return nil
	},
}

func init() {
	threatListCmd.Flags().IntP("limit", "n", 20, "显示条数")
	threatCmd.AddCommand(threatSummaryCmd)
	threatCmd.AddCommand(threatListCmd)
	threatCmd.AddCommand(threatBlacklistCmd)
}
