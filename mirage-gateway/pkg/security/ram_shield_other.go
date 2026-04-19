//go:build !linux

package security

import (
	"bufio"
	"fmt"
	"log"
	"strconv"
	"strings"
)

func mlockBuffer(data []byte) error {
	log.Printf("[RAMShield] ⚠️ 当前平台不支持 mlock，跳过内存锁定")
	return nil
}

func munlockBuffer(data []byte) error {
	log.Printf("[RAMShield] ⚠️ 当前平台不支持 munlock，跳过")
	return nil
}

func disableCoreDump() error {
	log.Printf("[RAMShield] ⚠️ 当前平台不支持禁用 core dump")
	return nil
}

func checkSwapUsage() (int64, error) {
	log.Printf("[RAMShield] ⚠️ 当前平台不支持 swap 检查")
	return 0, nil
}

// ParseVmSwap 解析 /proc/self/status 中的 VmSwap 字段（跨平台可测试）
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
			return val, nil
		}
	}
	return 0, fmt.Errorf("未找到 VmSwap 字段")
}
