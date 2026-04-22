package cmd

import (
	"fmt"
	"net/http"
	"time"

	"github.com/spf13/cobra"
)

// HealthCheckResult 深度健康巡检结果
type HealthCheckResult struct {
	Timestamp string        `json:"timestamp"`
	Gateway   string        `json:"gateway"`
	Overall   string        `json:"overall"`
	Checks    []CheckItem   `json:"checks"`
	Latency   LatencyReport `json:"latency"`
}

// CheckItem 单项检查
type CheckItem struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail"`
}

// LatencyReport 延迟报告
type LatencyReport struct {
	HealthMs int64 `json:"health_ms"`
	TunnelMs int64 `json:"tunnel_ms"`
	ThreatMs int64 `json:"threat_ms"`
}

var (
	healthWatch    bool
	healthInterval int
	healthWebhook  string
)

var healthCmd = &cobra.Command{
	Use:   "health",
	Short: "深度健康巡检",
	Long:  "对 Gateway 各模块进行深度健康检查（健康端点、eBPF、隧道、威胁、配额）",
	RunE: func(cmd *cobra.Command, args []string) error {
		result := runHealthCheck()

		if outputJSON {
			printJSON(result)
			return nil
		}

		printHealthResult(result)
		return nil
	},
}

func runHealthCheck() HealthCheckResult {
	now := time.Now().UTC().Format(time.RFC3339)
	result := HealthCheckResult{
		Timestamp: now,
		Gateway:   gatewayAddr,
		Overall:   "healthy",
	}

	client := &http.Client{Timeout: 5 * time.Second}

	// 检查项列表
	endpoints := []struct {
		name string
		path string
	}{
		{"health", "/health"},
		{"tunnel", "/api/tunnel/status"},
		{"threat", "/api/threat/summary"},
		{"quota", "/api/quota"},
		{"ebpf", "/api/diag/ebpf"},
		{"phantom", "/api/phantom/status"},
		{"strategy", "/api/strategy"},
	}

	for _, ep := range endpoints {
		start := time.Now()
		url := fmt.Sprintf("http://%s%s", gatewayAddr, ep.path)
		resp, err := client.Get(url)
		latency := time.Since(start)

		item := CheckItem{Name: ep.name}

		if err != nil {
			item.Status = "unreachable"
			item.Detail = fmt.Sprintf("连接失败: %v", err)
			if result.Overall == "healthy" {
				result.Overall = "critical"
			}
		} else {
			resp.Body.Close()
			if resp.StatusCode == 200 {
				item.Status = "ok"
				item.Detail = fmt.Sprintf("%dms", latency.Milliseconds())
			} else {
				item.Status = "error"
				item.Detail = fmt.Sprintf("HTTP %d (%dms)", resp.StatusCode, latency.Milliseconds())
				if result.Overall == "healthy" {
					result.Overall = "degraded"
				}
			}
		}

		// 记录关键延迟
		switch ep.name {
		case "health":
			result.Latency.HealthMs = latency.Milliseconds()
		case "tunnel":
			result.Latency.TunnelMs = latency.Milliseconds()
		case "threat":
			result.Latency.ThreatMs = latency.Milliseconds()
		}

		result.Checks = append(result.Checks, item)
	}

	return result
}

func printHealthResult(result HealthCheckResult) {
	overallIcon := "🟢"
	switch result.Overall {
	case "degraded":
		overallIcon = "🟡"
	case "critical":
		overallIcon = "🔴"
	}

	fmt.Printf("%s Gateway 健康巡检 (%s)\n", overallIcon, result.Gateway)
	fmt.Printf("  时间: %s\n\n", result.Timestamp)

	for _, c := range result.Checks {
		icon := "✅"
		switch c.Status {
		case "error":
			icon = "❌"
		case "unreachable":
			icon = "⚫"
		case "warning":
			icon = "⚠️ "
		}
		fmt.Printf("  %s %-12s %s\n", icon, c.Name, c.Detail)
	}

	fmt.Println()
	fmt.Printf("  延迟: health=%dms  tunnel=%dms  threat=%dms\n",
		result.Latency.HealthMs, result.Latency.TunnelMs, result.Latency.ThreatMs)
	fmt.Println()

	switch result.Overall {
	case "healthy":
		fmt.Println("  🟢 总体状态: 健康")
	case "degraded":
		fmt.Println("  🟡 总体状态: 降级")
	case "critical":
		fmt.Println("  🔴 总体状态: 异常")
	}
}
