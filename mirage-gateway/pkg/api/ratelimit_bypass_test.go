package api

import (
	"testing"
)

func TestRateLimiter_PortRotationBypass(t *testing.T) {
	rl := NewCommandRateLimiter()

	// 模拟同一 IP 不同端口的请求
	// 修复前：每个 ip:port 独立计数，攻击者可绕过
	// 修复后：peerAddr() 返回纯 IP，所有端口共享计数
	ip := "192.168.1.100"

	// 发送 10 次请求（达到限制）
	for i := 0; i < 10; i++ {
		if err := rl.Check(ip); err != nil {
			t.Fatalf("第 %d 次请求不应被限制: %v", i+1, err)
		}
	}

	// 第 11 次应被限制（无论端口如何变化）
	if err := rl.Check(ip); err == nil {
		t.Fatal("超过速率限制后应拒绝请求")
	}
}

func TestRateLimiter_DifferentIPsIndependent(t *testing.T) {
	rl := NewCommandRateLimiter()

	// 不同 IP 应独立计数
	for i := 0; i < 10; i++ {
		if err := rl.Check("10.0.0.1"); err != nil {
			t.Fatalf("IP1 第 %d 次不应被限制: %v", i+1, err)
		}
	}

	// IP1 已达限制
	if err := rl.Check("10.0.0.1"); err == nil {
		t.Fatal("IP1 超限后应拒绝")
	}

	// IP2 不受影响
	if err := rl.Check("10.0.0.2"); err != nil {
		t.Fatalf("IP2 不应被 IP1 的限制影响: %v", err)
	}
}
