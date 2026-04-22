package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
)

// PreflightResult 前置检查结果
type PreflightResult struct {
	Kernel       string `json:"kernel"`
	KernelOK     bool   `json:"kernel_ok"`
	BPFEnabled   bool   `json:"bpf_enabled"`
	BTFAvailable bool   `json:"btf_available"`
	XDPSupport   bool   `json:"xdp_support"`
	TCSupport    bool   `json:"tc_support"`
	MemlockLimit string `json:"memlock_limit"`
	MemlockOK    bool   `json:"memlock_ok"`
	Overall      bool   `json:"overall"`
}

var preflightCmd = &cobra.Command{
	Use:   "preflight",
	Short: "eBPF 环境前置检查",
	Long:  "检查内核版本、BPF 支持、BTF、XDP/TC 能力、memlock 限制等",
	RunE: func(cmd *cobra.Command, args []string) error {
		if runtime.GOOS != "linux" {
			fmt.Println("⚠️  eBPF 前置检查仅支持 Linux")
			fmt.Println("  当前系统: " + runtime.GOOS + "/" + runtime.GOARCH)
			return nil
		}

		result := PreflightResult{Overall: true}

		// 内核版本
		if out, err := exec.Command("uname", "-r").Output(); err == nil {
			result.Kernel = strings.TrimSpace(string(out))
			// 检查 >= 5.15
			parts := strings.Split(result.Kernel, ".")
			if len(parts) >= 2 {
				major, minor := 0, 0
				fmt.Sscanf(parts[0], "%d", &major)
				fmt.Sscanf(parts[1], "%d", &minor)
				result.KernelOK = major > 5 || (major == 5 && minor >= 15)
			}
		}

		// BPF 文件系统
		if _, err := os.Stat("/sys/fs/bpf"); err == nil {
			result.BPFEnabled = true
		}

		// BTF 支持
		if _, err := os.Stat("/sys/kernel/btf/vmlinux"); err == nil {
			result.BTFAvailable = true
		}

		// XDP 支持（检查 ip link 是否支持 xdp）
		if out, err := exec.Command("ip", "link", "help").CombinedOutput(); err == nil || len(out) > 0 {
			result.XDPSupport = strings.Contains(string(out), "xdp")
		}

		// TC 支持
		if _, err := exec.LookPath("tc"); err == nil {
			result.TCSupport = true
		}

		// memlock 限制
		if out, err := exec.Command("bash", "-c", "ulimit -l").Output(); err == nil {
			result.MemlockLimit = strings.TrimSpace(string(out))
			result.MemlockOK = result.MemlockLimit == "unlimited"
		}

		// 综合判定
		result.Overall = result.KernelOK && result.BPFEnabled && result.BTFAvailable

		if outputJSON {
			printJSON(result)
			return nil
		}

		fmt.Println("🔍 eBPF 环境前置检查")
		fmt.Println()
		printCheck("内核版本", result.Kernel, result.KernelOK, ">= 5.15")
		printCheck("BPF 文件系统", "/sys/fs/bpf", result.BPFEnabled, "已挂载")
		printCheck("BTF 支持", "/sys/kernel/btf/vmlinux", result.BTFAvailable, "可用")
		printCheck("XDP 支持", "ip link xdp", result.XDPSupport, "可用")
		printCheck("TC 支持", "tc 命令", result.TCSupport, "可用")
		printCheck("memlock 限制", result.MemlockLimit, result.MemlockOK, "unlimited")
		fmt.Println()

		if result.Overall {
			fmt.Println("✅ 环境检查通过，可以部署 Mirage Gateway")
		} else {
			fmt.Println("❌ 环境检查未通过，请修复上述问题")
			if !result.KernelOK {
				fmt.Println("  提示: 需要内核 >= 5.15（推荐 5.19+），Fallback 模式需要 >= 4.19")
			}
			if !result.MemlockOK {
				fmt.Println("  提示: 执行 ulimit -l unlimited 或修改 /etc/security/limits.conf")
			}
		}

		return nil
	},
}

func printCheck(name, value string, ok bool, expect string) {
	icon := "✅"
	if !ok {
		icon = "❌"
	}
	fmt.Printf("  %s %-16s  %-30s  (期望: %s)\n", icon, name, value, expect)
}
