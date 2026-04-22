package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"github.com/spf13/cobra"
)

var tuneApply bool

var tuneCmd = &cobra.Command{
	Use:   "tune",
	Short: "系统参数调优",
	Long:  "检查并优化 sysctl 网络参数（UDP 缓冲区、连接跟踪、BBR 等）",
}

var tuneCheckCmd = &cobra.Command{
	Use:   "check",
	Short: "检查当前系统参数",
	RunE: func(cmd *cobra.Command, args []string) error {
		if runtime.GOOS != "linux" {
			return fmt.Errorf("系统调优仅支持 Linux")
		}

		params := []struct {
			key      string
			desc     string
			expected string
		}{
			{"net.core.rmem_max", "UDP 接收缓冲区上限", "26214400"},
			{"net.core.wmem_max", "UDP 发送缓冲区上限", "26214400"},
			{"net.core.rmem_default", "UDP 接收缓冲区默认", "1048576"},
			{"net.core.wmem_default", "UDP 发送缓冲区默认", "1048576"},
			{"net.ipv4.tcp_congestion_control", "TCP 拥塞控制", "bbr"},
			{"net.core.default_qdisc", "默认队列调度", "fq"},
			{"net.ipv4.ip_forward", "IP 转发", "1"},
			{"net.core.netdev_max_backlog", "网卡队列积压上限", "10000"},
			{"net.ipv4.tcp_max_syn_backlog", "SYN 队列上限", "8192"},
			{"net.netfilter.nf_conntrack_max", "连接跟踪上限", "1048576"},
		}

		fmt.Println("🔧 系统参数检查")
		fmt.Println()

		allOK := true
		for _, p := range params {
			out, err := exec.Command("sysctl", "-n", p.key).Output()
			if err != nil {
				fmt.Printf("  ⚠️  %-42s  无法读取\n", p.key)
				continue
			}
			current := trimOutput(out)
			ok := current == p.expected
			icon := "✅"
			if !ok {
				icon = "❌"
				allOK = false
			}
			fmt.Printf("  %s %-42s  当前: %-12s  推荐: %s  (%s)\n",
				icon, p.key, current, p.expected, p.desc)
		}

		fmt.Println()
		if allOK {
			fmt.Println("✅ 所有参数已优化")
		} else {
			fmt.Println("⚠️  部分参数未优化，执行 mirage-cli tune apply 应用推荐值")
		}
		return nil
	},
}

var tuneApplyCmd = &cobra.Command{
	Use:   "apply",
	Short: "应用推荐系统参数（需要 root）",
	RunE: func(cmd *cobra.Command, args []string) error {
		if runtime.GOOS != "linux" {
			return fmt.Errorf("系统调优仅支持 Linux")
		}

		scriptPaths := []string{
			"/opt/mirage/scripts/sysctl-tuning.sh",
			"/usr/local/bin/sysctl-tuning.sh",
			"deploy/scripts/sysctl-tuning.sh",
		}

		var scriptPath string
		for _, p := range scriptPaths {
			if _, err := os.Stat(p); err == nil {
				scriptPath = p
				break
			}
		}
		if scriptPath == "" {
			return fmt.Errorf("找不到 sysctl-tuning.sh 脚本")
		}

		fmt.Printf("应用系统参数调优: %s\n", scriptPath)
		execCmd := exec.Command("bash", scriptPath)
		execCmd.Stdout = os.Stdout
		execCmd.Stderr = os.Stderr
		return execCmd.Run()
	},
}

func init() {
	tuneCmd.AddCommand(tuneCheckCmd)
	tuneCmd.AddCommand(tuneApplyCmd)
}

func trimOutput(b []byte) string {
	s := string(b)
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r' || s[len(s)-1] == ' ') {
		s = s[:len(s)-1]
	}
	return s
}
