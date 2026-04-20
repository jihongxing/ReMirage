package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "配置管理",
	Long:  "查看和管理 Gateway 配置",
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "显示当前配置",
	RunE: func(cmd *cobra.Command, args []string) error {
		// 优先从本地文件读取
		path := configPath
		if path == "" {
			path = "/etc/mirage/gateway.yaml"
		}

		data, err := os.ReadFile(path)
		if err != nil {
			// 尝试从 Gateway API 获取
			url := fmt.Sprintf("http://%s/api/config", gatewayAddr)
			client := &http.Client{Timeout: 5 * time.Second}
			resp, err2 := client.Get(url)
			if err2 != nil {
				return fmt.Errorf("无法读取配置文件 (%s) 且无法连接 Gateway: %v", path, err)
			}
			defer resp.Body.Close()
			data, _ = io.ReadAll(resp.Body)
		}

		// 尝试格式化 YAML
		var node yaml.Node
		if err := yaml.Unmarshal(data, &node); err == nil {
			enc := yaml.NewEncoder(os.Stdout)
			enc.SetIndent(2)
			enc.Encode(&node)
			return nil
		}

		// 原样输出
		fmt.Println(string(data))
		return nil
	},
}

var configProtocolsCmd = &cobra.Command{
	Use:   "protocols",
	Short: "显示协议启用状态",
	RunE: func(cmd *cobra.Command, args []string) error {
		url := fmt.Sprintf("http://%s/api/config/protocols", gatewayAddr)
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

		var protocols map[string]bool
		if err := json.Unmarshal(body, &protocols); err != nil {
			fmt.Printf("原始响应:\n%s\n", string(body))
			return nil
		}

		fmt.Println("协议状态:")
		protoNames := []struct {
			key  string
			name string
			desc string
		}{
			{"npm", "NPM", "流量伪装 (XDP)"},
			{"bdna", "B-DNA", "行为识别 (TC)"},
			{"jitter", "Jitter-Lite", "时域扰动 (TC)"},
			{"vpc", "VPC", "噪声注入 (TC)"},
			{"gtunnel", "G-Tunnel", "多路径传输"},
			{"gswitch", "G-Switch", "域名转生"},
		}

		for _, p := range protoNames {
			enabled, ok := protocols[p.key]
			icon := "❓"
			if ok {
				if enabled {
					icon = "✅"
				} else {
					icon = "❌"
				}
			}
			fmt.Printf("  %s %-12s %s\n", icon, p.name, p.desc)
		}

		return nil
	},
}

var configDefenseCmd = &cobra.Command{
	Use:   "defense",
	Short: "显示防御策略参数",
	RunE: func(cmd *cobra.Command, args []string) error {
		url := fmt.Sprintf("http://%s/api/config/defense", gatewayAddr)
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
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configProtocolsCmd)
	configCmd.AddCommand(configDefenseCmd)
}
