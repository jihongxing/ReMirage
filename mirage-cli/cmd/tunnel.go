package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/spf13/cobra"
)

// TunnelStatus 隧道状态
type TunnelStatus struct {
	Active     bool           `json:"active"`
	Paths      []PathInfo     `json:"paths"`
	FEC        FECInfo        `json:"fec"`
	Throughput ThroughputInfo `json:"throughput"`
}

// PathInfo 路径信息
type PathInfo struct {
	Type     string  `json:"type"`      // quic/wss/webrtc/icmp/dns
	Level    int     `json:"level"`     // 优先级 0-3
	Status   string  `json:"status"`    // active/standby/degraded/dead
	RTT      string  `json:"rtt"`       // 往返延迟
	LossRate float64 `json:"loss_rate"` // 丢包率
	Endpoint string  `json:"endpoint"`  // 远端地址
}

// FECInfo FEC 状态
type FECInfo struct {
	Enabled    bool    `json:"enabled"`
	Ratio      float64 `json:"ratio"`       // 冗余率
	Recovered  int64   `json:"recovered"`   // 已恢复包数
	DataShards int     `json:"data_shards"` // 数据分片数
}

// ThroughputInfo 吞吐量
type ThroughputInfo struct {
	UpBytesPerSec   int64 `json:"up_bytes_per_sec"`
	DownBytesPerSec int64 `json:"down_bytes_per_sec"`
}

var tunnelCmd = &cobra.Command{
	Use:   "tunnel",
	Short: "隧道状态与管理",
	Long:  "查看 G-Tunnel 多路径隧道状态、路径信息、FEC 统计",
}

var tunnelStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "查看隧道状态",
	RunE: func(cmd *cobra.Command, args []string) error {
		url := fmt.Sprintf("http://%s/api/tunnel/status", gatewayAddr)
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

		var status TunnelStatus
		if err := json.Unmarshal(body, &status); err != nil {
			fmt.Printf("原始响应:\n%s\n", string(body))
			return nil
		}

		if !status.Active {
			fmt.Println("⚫ 隧道未激活")
			return nil
		}

		fmt.Println("🟢 隧道已激活")
		fmt.Println()

		// 路径列表
		fmt.Println("📡 传输路径:")
		for _, p := range status.Paths {
			icon := pathIcon(p.Status)
			fmt.Printf("  %s [L%d] %-7s  RTT: %-8s  丢包: %.1f%%  → %s\n",
				icon, p.Level, p.Type, p.RTT, p.LossRate*100, p.Endpoint)
		}
		fmt.Println()

		// FEC
		fmt.Printf("🛡️  FEC: enabled=%v  ratio=%.2f  recovered=%d  shards=%d\n",
			status.FEC.Enabled, status.FEC.Ratio, status.FEC.Recovered, status.FEC.DataShards)

		// 吞吐
		fmt.Printf("📊 吞吐: ↑ %s/s  ↓ %s/s\n",
			humanBytes(status.Throughput.UpBytesPerSec),
			humanBytes(status.Throughput.DownBytesPerSec))

		return nil
	},
}

var tunnelPathsCmd = &cobra.Command{
	Use:   "paths",
	Short: "列出所有传输路径",
	RunE: func(cmd *cobra.Command, args []string) error {
		url := fmt.Sprintf("http://%s/api/tunnel/paths", gatewayAddr)
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

		var paths []PathInfo
		if err := json.Unmarshal(body, &paths); err != nil {
			fmt.Printf("原始响应:\n%s\n", string(body))
			return nil
		}

		fmt.Printf("%-6s %-8s %-10s %-10s %-8s %s\n", "Level", "Type", "Status", "RTT", "Loss", "Endpoint")
		fmt.Println("─────────────────────────────────────────────────────────────────")
		for _, p := range paths {
			fmt.Printf("L%-5d %-8s %-10s %-10s %-7.1f%% %s\n",
				p.Level, p.Type, p.Status, p.RTT, p.LossRate*100, p.Endpoint)
		}

		return nil
	},
}

func init() {
	tunnelCmd.AddCommand(tunnelStatusCmd)
	tunnelCmd.AddCommand(tunnelPathsCmd)
}

func pathIcon(status string) string {
	switch status {
	case "active":
		return "🟢"
	case "standby":
		return "🟡"
	case "degraded":
		return "🟠"
	default:
		return "🔴"
	}
}

func humanBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
