//go:build linux

package security

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"syscall"
)

func mlockBuffer(data []byte) error {
	return syscall.Mlock(data)
}

func munlockBuffer(data []byte) error {
	return syscall.Munlock(data)
}

func disableCoreDump() error {
	var rlim syscall.Rlimit
	rlim.Cur = 0
	rlim.Max = 0
	if err := syscall.Setrlimit(syscall.RLIMIT_CORE, &rlim); err != nil {
		log.Printf("[RAMShield] ⚠️ 禁用 core dump 失败: %v", err)
		return err
	}
	log.Println("[RAMShield] ✅ core dump 已禁用")
	return nil
}

func checkSwapUsage() (int64, error) {
	data, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return 0, fmt.Errorf("读取 /proc/self/status 失败: %w", err)
	}
	return ParseVmSwap(string(data))
}

// ParseVmSwap 解析 /proc/self/status 中的 VmSwap 字段
func ParseVmSwap(content string) (int64, error) {
	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "VmSwap:") {
			fields := strings.Fields(line)
			if len(fields) < 2 {
				return 0, fmt.Errorf("VmSwap 字段格式错误: %s", line)
			}
			val, err := strconv.ParseInt(fields[1], 10, 64)
			if err != nil {
				return 0, fmt.Errorf("解析 VmSwap 值失败: %w", err)
			}
			if val > 0 {
				log.Printf("[RAMShield] ⚠️ 检测到 swap 使用: %d KB", val)
			}
			return val, nil
		}
	}
	return 0, fmt.Errorf("未找到 VmSwap 字段")
}
