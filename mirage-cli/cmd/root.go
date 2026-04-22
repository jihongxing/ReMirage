package cmd

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

var (
	Version   = "2.0.0"
	BuildTime = "unknown"
	GitCommit = "unknown"
)

var (
	gatewayAddr string
	configPath  string
)

var (
	outputJSON bool
	osAddr     string
)

var rootCmd = &cobra.Command{
	Use:   "mirage-cli",
	Short: "Mirage Gateway 管理工具",
	Long:  "Mirage CLI v2 — 融合网关管理、隧道控制、认证签名、证书管理、策略调整、运维诊断",
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&gatewayAddr, "gateway", "g", "127.0.0.1:9090", "Gateway 健康检查地址")
	rootCmd.PersistentFlags().StringVarP(&osAddr, "os", "o", "", "Mirage-OS API 地址 (例: https://mirage-os:3000)")
	rootCmd.PersistentFlags().StringVarP(&configPath, "config", "c", "", "配置文件路径")
	rootCmd.PersistentFlags().BoolVar(&outputJSON, "json", false, "JSON 格式输出")

	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(tunnelCmd)
	rootCmd.AddCommand(threatCmd)
	rootCmd.AddCommand(quotaCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(diagCmd)
	rootCmd.AddCommand(signCmd)
	rootCmd.AddCommand(keygenCmd)
	rootCmd.AddCommand(certCmd)
	rootCmd.AddCommand(backupCmd)
	rootCmd.AddCommand(wipeCmd)
	rootCmd.AddCommand(preflightCmd)
	rootCmd.AddCommand(tuneCmd)
	rootCmd.AddCommand(topologyCmd)
	rootCmd.AddCommand(strategyCmd)
	rootCmd.AddCommand(phantomCmd)
	rootCmd.AddCommand(logCmd)
	rootCmd.AddCommand(healthCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "显示版本信息",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("Mirage CLI v%s\n", Version)
		fmt.Printf("  Build:   %s\n", BuildTime)
		fmt.Printf("  Commit:  %s\n", GitCommit)
		fmt.Printf("  Go:      %s\n", runtime.Version())
		fmt.Printf("  OS/Arch: %s/%s\n", runtime.GOOS, runtime.GOARCH)
	},
}

func Execute() error {
	return rootCmd.Execute()
}
