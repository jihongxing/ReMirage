package cmd

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
)

var (
	wipeIncludeLogs bool
	wipeForce       bool
)

var wipeCmd = &cobra.Command{
	Use:   "wipe",
	Short: "焦土协议 — 安全擦除所有 Mirage 痕迹",
	Long: `⚠️  此操作不可逆！执行后 Gateway 将完全停止工作。
将销毁：mTLS 证书/私钥、配置文件、eBPF 程序/Map、认证密钥、运行时状态。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if runtime.GOOS != "linux" {
			return fmt.Errorf("焦土协议仅支持 Linux")
		}

		// 安全确认
		if !wipeForce {
			fmt.Println("╔══════════════════════════════════════════════════╗")
			fmt.Println("║         ⚠️  EMERGENCY WIPE / 焦土协议           ║")
			fmt.Println("║  此操作将永久销毁所有 Mirage 相关数据            ║")
			fmt.Println("║  执行后无法恢复！                                ║")
			fmt.Println("╚══════════════════════════════════════════════════╝")
			fmt.Print("\n输入 'WIPE' 确认执行: ")

			reader := bufio.NewReader(os.Stdin)
			input, _ := reader.ReadString('\n')
			input = strings.TrimSpace(input)
			if input != "WIPE" {
				fmt.Println("已取消")
				return nil
			}
		}

		scriptPaths := []string{
			"/opt/mirage/scripts/emergency-wipe.sh",
			"/usr/local/bin/emergency-wipe.sh",
			"deploy/scripts/emergency-wipe.sh",
		}

		var scriptPath string
		for _, p := range scriptPaths {
			if _, err := os.Stat(p); err == nil {
				scriptPath = p
				break
			}
		}
		if scriptPath == "" {
			return fmt.Errorf("找不到 emergency-wipe.sh 脚本")
		}

		wipeArgs := []string{scriptPath, "--confirm"}
		if wipeIncludeLogs {
			wipeArgs = append(wipeArgs, "--include-logs")
		}

		execCmd := exec.Command("bash", wipeArgs...)
		execCmd.Stdout = os.Stdout
		execCmd.Stderr = os.Stderr
		execCmd.Stdin = os.Stdin
		return execCmd.Run()
	},
}

func init() {
	wipeCmd.Flags().BoolVar(&wipeIncludeLogs, "include-logs", false, "同时清除日志")
	wipeCmd.Flags().BoolVar(&wipeForce, "force", false, "跳过交互确认（危险）")
}
