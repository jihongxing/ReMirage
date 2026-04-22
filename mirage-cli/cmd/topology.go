package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/spf13/cobra"
)

// TopologyResponse 拓扑响应
type TopologyResponse struct {
	Version     int64         `json:"version"`
	PublishedAt string        `json:"published_at"`
	Gateways    []GatewayNode `json:"gateways"`
	Signature   string        `json:"signature"`
}

// GatewayNode 网关节点
type GatewayNode struct {
	GatewayID string `json:"gateway_id"`
	IPAddress string `json:"ip_address"`
	CellID    string `json:"cell_id"`
	Status    string `json:"status"`
	Region    string `json:"region,omitempty"`
	Load      int    `json:"load,omitempty"`
}

var topologyCmd = &cobra.Command{
	Use:   "topology",
	Short: "网关拓扑查询",
	Long:  "查询 Mirage-OS 网关拓扑信息、节点状态",
}

var topologyListCmd = &cobra.Command{
	Use:   "list",
	Short: "列出所有网关节点",
	RunE: func(cmd *cobra.Command, args []string) error {
		// 优先从 OS API 获取
		if osAddr != "" {
			return fetchTopologyFromOS()
		}
		// 降级从 Gateway 获取
		return fetchTopologyFromGateway()
	},
}

var topologyPingCmd = &cobra.Command{
	Use:   "ping",
	Short: "测试到各网关节点的延迟",
	RunE: func(cmd *cobra.Command, args []string) error {
		body, err := gatewayGet("/api/topology")
		if err != nil {
			return err
		}

		var topo TopologyResponse
		if err := json.Unmarshal(body, &topo); err != nil {
			printRawJSON(body)
			return nil
		}

		fmt.Println("📡 网关节点延迟测试")
		fmt.Println()

		client := &http.Client{Timeout: 5 * time.Second}
		for _, gw := range topo.Gateways {
			start := time.Now()
			url := fmt.Sprintf("http://%s:9090/health", gw.IPAddress)
			resp, err := client.Get(url)
			latency := time.Since(start)

			icon := "🔴"
			status := "不可达"
			if err == nil {
				resp.Body.Close()
				icon = "🟢"
				status = fmt.Sprintf("%dms", latency.Milliseconds())
			}

			fmt.Printf("  %s %-12s  %-16s  %-10s  %s\n",
				icon, gw.GatewayID, gw.IPAddress, gw.CellID, status)
		}
		return nil
	},
}

func init() {
	topologyCmd.AddCommand(topologyListCmd)
	topologyCmd.AddCommand(topologyPingCmd)
}

func fetchTopologyFromOS() error {
	url := fmt.Sprintf("%s/api/v2/topology", osAddr)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("无法连接 Mirage-OS (%s): %w", osAddr, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("读取响应失败: %w", err)
	}

	return displayTopology(body)
}

func fetchTopologyFromGateway() error {
	body, err := gatewayGet("/api/topology")
	if err != nil {
		return err
	}
	return displayTopology(body)
}

func displayTopology(body []byte) error {
	var topo TopologyResponse
	if err := json.Unmarshal(body, &topo); err != nil {
		printRawJSON(body)
		return nil
	}

	if outputJSON {
		printJSON(topo)
		return nil
	}

	fmt.Printf("🌐 网关拓扑 (版本: %d, 发布: %s)\n\n", topo.Version, topo.PublishedAt)
	fmt.Printf("%-14s %-18s %-14s %-10s\n", "Gateway ID", "IP", "Cell", "Status")
	fmt.Println("──────────────────────────────────────────────────────────")
	for _, gw := range topo.Gateways {
		icon := statusIcon(gw.Status)
		fmt.Printf("%s %-12s %-18s %-14s %-10s\n",
			icon, gw.GatewayID, gw.IPAddress, gw.CellID, gw.Status)
	}
	fmt.Printf("\n共 %d 个节点\n", len(topo.Gateways))
	return nil
}

func statusIcon(status string) string {
	switch status {
	case "ONLINE":
		return "🟢"
	case "DEGRADED":
		return "🟡"
	case "OFFLINE":
		return "🔴"
	default:
		return "⚪"
	}
}
