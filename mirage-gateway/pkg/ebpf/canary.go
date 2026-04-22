//go:build linux

package ebpf

import (
	"fmt"
	"log"
	"os/exec"
)

// CanaryAttach 金丝雀挂载流程
// 1. 创建 dummy0 虚拟接口
// 2. 在 dummy0 上挂载所有 eBPF 程序
// 3. 执行自检（验证 Map 读写）
// 4. 卸载 dummy0 上的程序
// 5. 删除 dummy0
// 6. 返回校验结果
func (l *Loader) CanaryAttach() error {
	log.Println("[Canary] 开始金丝雀挂载验证...")

	// 1. 创建 dummy0 虚拟接口
	if err := exec.Command("ip", "link", "add", "dummy0", "type", "dummy").Run(); err != nil {
		return fmt.Errorf("创建 dummy0 失败: %w", err)
	}
	defer func() {
		exec.Command("ip", "link", "del", "dummy0").Run()
		log.Println("[Canary] dummy0 已清理")
	}()

	if err := exec.Command("ip", "link", "set", "dummy0", "up").Run(); err != nil {
		return fmt.Errorf("启用 dummy0 失败: %w", err)
	}

	// 2. 创建临时 Loader 在 dummy0 上挂载
	canaryLoader := NewLoader("dummy0")
	if err := canaryLoader.LoadAndAttach(); err != nil {
		return fmt.Errorf("金丝雀挂载失败: %w", err)
	}
	defer canaryLoader.Close()

	// 3. 自检：验证关键 Map 可读写
	testMaps := []string{"ctrl_map", "jitter_config_map", "npm_config_map"}
	for _, name := range testMaps {
		m := canaryLoader.GetMap(name)
		if m == nil {
			return fmt.Errorf("金丝雀自检失败: Map %s 不存在", name)
		}
		// 尝试写入测试值
		key := uint32(0xCAFE)
		value := uint32(0xBEEF)
		if err := m.Put(&key, &value); err != nil {
			return fmt.Errorf("金丝雀自检失败: Map %s 写入失败: %w", name, err)
		}
		// 读回验证
		var readVal uint32
		if err := m.Lookup(&key, &readVal); err != nil {
			return fmt.Errorf("金丝雀自检失败: Map %s 读取失败: %w", name, err)
		}
		if readVal != value {
			return fmt.Errorf("金丝雀自检失败: Map %s 读写不一致", name)
		}
		// 清理测试数据
		m.Delete(&key)
	}

	log.Println("[Canary] ✅ 金丝雀挂载验证通过")
	return nil
}
