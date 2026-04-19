package security

import (
	"fmt"
	"testing"

	"pgregory.net/rapid"
)

// Property 1: SecureWipe 内存清零
func TestProperty_SecureWipeZeroes(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		size := rapid.IntRange(1, 4096).Draw(t, "size")
		rs := NewRAMShield()

		buf, err := rs.SecureAlloc(size)
		if err != nil {
			t.Fatalf("SecureAlloc 失败: %v", err)
		}

		// 写入随机数据
		for i := range buf.Data {
			buf.Data[i] = byte(rapid.IntRange(1, 255).Draw(t, fmt.Sprintf("byte_%d", i)))
		}

		if err := rs.SecureWipe(buf); err != nil {
			t.Fatalf("SecureWipe 失败: %v", err)
		}

		for i, b := range buf.Data {
			if b != 0 {
				t.Fatalf("SecureWipe 后字节 %d 不为零: %d", i, b)
			}
		}
	})
}

// Property 2: SecureAlloc/SecureWipe 往返一致性
func TestProperty_SecureAllocWipeRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		size := rapid.IntRange(1, 4096).Draw(t, "size")
		rs := NewRAMShield()

		buf, err := rs.SecureAlloc(size)
		if err != nil {
			t.Fatalf("SecureAlloc 失败: %v", err)
		}

		if !rs.ContainsBuffer(buf) {
			t.Fatal("缓冲区未注册")
		}

		// 写入随机数据
		for i := range buf.Data {
			buf.Data[i] = byte(rapid.IntRange(1, 255).Draw(t, fmt.Sprintf("b_%d", i)))
		}

		if err := rs.SecureWipe(buf); err != nil {
			t.Fatalf("SecureWipe 失败: %v", err)
		}

		// 验证清零
		for i, b := range buf.Data {
			if b != 0 {
				t.Fatalf("字节 %d 不为零", i)
			}
		}

		// 验证已从注册列表移除
		if rs.ContainsBuffer(buf) {
			t.Fatal("缓冲区未从注册列表移除")
		}
	})
}

// Property 3: CheckSwapUsage 解析正确性
func TestProperty_ParseVmSwap(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		vmSwap := rapid.Int64Range(0, 1000000).Draw(t, "vmswap")
		content := fmt.Sprintf(
			"Name:\ttest\nVmPeak:\t1000 kB\nVmSize:\t500 kB\nVmSwap:\t%d kB\nThreads:\t1\n",
			vmSwap,
		)

		parsed, err := ParseVmSwap(content)
		if err != nil {
			t.Fatalf("ParseVmSwap 失败: %v", err)
		}

		if parsed != vmSwap {
			t.Fatalf("解析值不匹配: 期望 %d, 实际 %d", vmSwap, parsed)
		}
	})
}

// 单元测试: DisableCoreDump 调用
func TestDisableCoreDump(t *testing.T) {
	rs := NewRAMShield()
	// 不应 panic
	_ = rs.DisableCoreDump()
}

// 单元测试: VmSwap 告警
func TestParseVmSwap_NoField(t *testing.T) {
	content := "Name:\ttest\nVmPeak:\t1000 kB\n"
	_, err := ParseVmSwap(content)
	if err == nil {
		t.Fatal("应返回错误")
	}
}

// 单元测试: WipeAll 空列表
func TestWipeAll_Empty(t *testing.T) {
	rs := NewRAMShield()
	if err := rs.WipeAll(); err != nil {
		t.Fatalf("WipeAll 空列表不应失败: %v", err)
	}
}

// 单元测试: SecureAlloc 无效大小
func TestSecureAlloc_InvalidSize(t *testing.T) {
	rs := NewRAMShield()
	_, err := rs.SecureAlloc(0)
	if err == nil {
		t.Fatal("应返回错误")
	}
	_, err = rs.SecureAlloc(-1)
	if err == nil {
		t.Fatal("应返回错误")
	}
}
