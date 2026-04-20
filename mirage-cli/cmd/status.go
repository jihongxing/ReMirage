package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/spf13/cobra"
)

// HealthResponse Gateway 健康检查响应
type HealthResponse struct {
	Status       string `json:"status"`
	Uptime       string `json:"uptime"`
	EBPFLoaded   bool   `json:"ebpf_loaded"`
	GRPCUplink   string `json:"grpc_uplink"`
	GRPCDownlink string `json:"grpc_downlink"`
	ThreatLevel  int    `json:"threat_level"`
	Connections  int64  `json:"active_connections"`
	MemoryMB     int    `json:"memory_mb"`
	Version      string `json:"version"`
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "查询 Gateway 运行状态",
	Long:  "通过健康检查端点获取 Gateway 实时运行状态",
	RunE: func(cmd *cobra.Command, args []string) error {
		url := fmt.Sprintf("http://%s/health", gatewayAddr)

		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Get(url)
		if err != nil {
			return fmt.Errorf("无法连接 Gateway (%s): %w", gatewayAddr, err)
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("读取响应失败: %w", err)
		}

		var health HealthResponse
		if err := json.Unmarshal(body, &health); err != nil {
			// 非 JSON 响应，直接输出
			fmt.Printf("Gateway 响应 (%d):\n%s\n", resp.StatusCode, string(body))
			return nil
		}

		// 状态图标
		statusIcon := "🟢"
		switch health.Status {
		case "degraded":
			statusIcon = "🟡"
		case "emergency":
			statusIcon = "🔴"
		case "offline":
			statusIcon = "⚫"
		}

		fmt.Printf("%s Gateway 状态: %s\n", statusIcon, health.Status)
		fmt.Printf("  运行时间:    %s\n", health.Uptime)
		fmt.Printf("  eBPF 加载:   %v\n", health.EBPFLoaded)
		fmt.Printf("  gRPC 上行:   %s\n", health.GRPCUplink)
		fmt.Printf("  gRPC 下行:   %s\n", health.GRPCDownlink)
		fmt.Printf("  威胁等级:    %d\n", health.ThreatLevel)
		fmt.Printf("  活跃连接:    %d\n", health.Connections)
		fmt.Printf("  内存占用:    %d MB\n", health.MemoryMB)

		return nil
	},
}
