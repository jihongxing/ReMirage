package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// DiagInfo 诊断信息
type DiagInfo struct {
	Kernel       string            `json:"kernel"`
	EBPFPrograms []EBPFProgramInfo `json:"ebpf_programs"`
	Maps         []EBPFMapInfo     `json:"maps"`
	Interfaces   []InterfaceInfo   `json:"interfaces"`
}

// EBPFProgramInfo eBPF 程序信息
type EBPFProgramInfo struct {
	Name     string `json:"name"`
	Type     string `json:"type"` // xdp/tc/sockops/sk_msg
	Attached bool   `json:"attached"`
	RunCount int64  `json:"run_count"`
}

// EBPFMapInfo eBPF Map 信息
type EBPFMapInfo struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	KeySize    int    `json:"key_size"`
	ValSize    int    `json:"value_size"`
	MaxEntries int    `json:"max_entries"`
}

// InterfaceInfo 网络接口信息
type InterfaceInfo struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	MTU    int    `json:"mtu"`
}

var diagCmd = &cobra.Command{
	Use:   "diag",
	Short: "系统诊断",
	Long:  "收集系统诊断信息：内核版本、eBPF 程序状态、网络接口",
}

var diagAllCmd = &cobra.Command{
	Use:   "all",
	Short: "完整诊断报告",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("═══════════════════════════════════════════════════════")
		fmt.Println("  Mirage Gateway 诊断报告")
		fmt.Println("═══════════════════════════════════════════════════════")
		fmt.Println()

		// 系统信息
		fmt.Println("📋 系统信息:")
		fmt.Printf("  OS/Arch:  %s/%s\n", runtime.GOOS, runtime.GOARCH)
		if runtime.GOOS == "linux" {
			if out, err := exec.Command("uname", "-r").Output(); err == nil {
				fmt.Printf("  Kernel:   %s", string(out))
			}
		}
		fmt.Println()

		// Gateway 状态
		fmt.Println("🔌 Gateway 连接:")
		url := fmt.Sprintf("http://%s/health", gatewayAddr)
		client := &http.Client{Timeout: 3 * time.Second}
		resp, err := client.Get(url)
		if err != nil {
			fmt.Printf("  ❌ 无法连接 (%s)\n", gatewayAddr)
		} else {
			resp.Body.Close()
			fmt.Printf("  ✅ 已连接 (%s) HTTP %d\n", gatewayAddr, resp.StatusCode)
		}
		fmt.Println()

		// eBPF 诊断
		fmt.Println("🔧 eBPF 状态:")
		diagURL := fmt.Sprintf("http://%s/api/diag/ebpf", gatewayAddr)
		resp, err = client.Get(diagURL)
		if err != nil {
			fmt.Println("  无法获取 eBPF 诊断信息")
		} else {
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			var diag DiagInfo
			if err := json.Unmarshal(body, &diag); err == nil {
				for _, prog := range diag.EBPFPrograms {
					icon := "✅"
					if !prog.Attached {
						icon = "❌"
					}
					fmt.Printf("  %s %-20s [%s] runs=%d\n", icon, prog.Name, prog.Type, prog.RunCount)
				}
				fmt.Printf("\n  Maps: %d 个\n", len(diag.Maps))
			} else {
				fmt.Printf("  原始: %s\n", string(body))
			}
		}
		fmt.Println()

		// 网络检查
		fmt.Println("🌐 网络检查:")
		if runtime.GOOS == "linux" {
			if out, err := exec.Command("ip", "link", "show", "up").Output(); err == nil {
				lines := strings.Split(string(out), "\n")
				for _, line := range lines {
					if strings.Contains(line, "mtu") {
						fmt.Printf("  %s\n", strings.TrimSpace(line))
					}
				}
			}
		} else {
			fmt.Println("  (仅 Linux 支持完整网络诊断)")
		}

		fmt.Println()
		fmt.Println("═══════════════════════════════════════════════════════")
		return nil
	},
}

var diagEbpfCmd = &cobra.Command{
	Use:   "ebpf",
	Short: "eBPF 程序诊断",
	RunE: func(cmd *cobra.Command, args []string) error {
		url := fmt.Sprintf("http://%s/api/diag/ebpf", gatewayAddr)
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

		var diag DiagInfo
		if err := json.Unmarshal(body, &diag); err != nil {
			fmt.Printf("原始响应:\n%s\n", string(body))
			return nil
		}

		fmt.Println("eBPF 程序:")
		fmt.Printf("%-22s %-10s %-10s %s\n", "名称", "类型", "状态", "执行次数")
		fmt.Println("─────────────────────────────────────────────────────────")
		for _, prog := range diag.EBPFPrograms {
			status := "attached"
			if !prog.Attached {
				status = "detached"
			}
			fmt.Printf("%-22s %-10s %-10s %d\n", prog.Name, prog.Type, status, prog.RunCount)
		}

		fmt.Printf("\neBPF Maps (%d):\n", len(diag.Maps))
		fmt.Printf("%-24s %-12s %-6s %-6s %s\n", "名称", "类型", "Key", "Val", "MaxEntries")
		fmt.Println("─────────────────────────────────────────────────────────────────")
		for _, m := range diag.Maps {
			fmt.Printf("%-24s %-12s %-6d %-6d %d\n", m.Name, m.Type, m.KeySize, m.ValSize, m.MaxEntries)
		}

		return nil
	},
}

var diagConnCmd = &cobra.Command{
	Use:   "conn",
	Short: "连接诊断",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("连接诊断:")
		fmt.Println()

		client := &http.Client{Timeout: 3 * time.Second}

		// Health check
		start := time.Now()
		url := fmt.Sprintf("http://%s/health", gatewayAddr)
		resp, err := client.Get(url)
		latency := time.Since(start)
		if err != nil {
			fmt.Printf("  ❌ Gateway 健康检查: 不可达 (%v)\n", err)
		} else {
			resp.Body.Close()
			fmt.Printf("  ✅ Gateway 健康检查: %dms (HTTP %d)\n", latency.Milliseconds(), resp.StatusCode)
		}

		// gRPC 状态
		url = fmt.Sprintf("http://%s/api/diag/grpc", gatewayAddr)
		resp, err = client.Get(url)
		if err != nil {
			fmt.Printf("  ❌ gRPC 状态: 无法获取\n")
		} else {
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			fmt.Printf("  📡 gRPC: %s\n", string(body))
		}

		return nil
	},
}

func init() {
	diagCmd.AddCommand(diagAllCmd)
	diagCmd.AddCommand(diagEbpfCmd)
	diagCmd.AddCommand(diagConnCmd)
}
