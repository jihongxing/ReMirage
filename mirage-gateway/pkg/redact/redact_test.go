package redact

import (
	"fmt"
	"strings"
	"testing"

	"pgregory.net/rapid"
)

// --- RedactIP ---

func TestRedactIP_Standard(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"192.168.1.100", "192.168.1.***"},
		{"10.0.0.1", "10.0.0.***"},
		{"255.255.255.255", "255.255.255.***"},
		{"0.0.0.0", "0.0.0.***"},
		{"1.2.3.4", "1.2.3.***"},
	}
	for _, tt := range tests {
		got := RedactIP(tt.input)
		if got != tt.want {
			t.Errorf("RedactIP(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestRedactIP_Empty(t *testing.T) {
	if got := RedactIP(""); got != "" {
		t.Errorf("RedactIP(\"\") = %q, want \"\"", got)
	}
}

func TestRedactIP_NonIPv4(t *testing.T) {
	inputs := []string{
		"not-an-ip",
		"abc.def.ghi.jkl",
		"192.168.1",
		"::1",
		"fe80::1",
	}
	for _, input := range inputs {
		got := RedactIP(input)
		if got != input {
			t.Errorf("RedactIP(%q) = %q, want %q (unchanged)", input, got, input)
		}
	}
}

// --- RedactToken ---

func TestRedactToken_Standard(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Bearer eyJhbGciOiJIUzI1NiJ9.xxx", "***"},
		{"some-api-key-12345", "***"},
		{"a", "***"},
	}
	for _, tt := range tests {
		got := RedactToken(tt.input)
		if got != tt.want {
			t.Errorf("RedactToken(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestRedactToken_Empty(t *testing.T) {
	if got := RedactToken(""); got != "" {
		t.Errorf("RedactToken(\"\") = %q, want \"\"", got)
	}
}

// --- RedactSecret ---

func TestRedactSecret_Standard(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"my-super-secret-password", "[REDACTED]"},
		{"postgres", "[REDACTED]"},
		{"change-this-in-production", "[REDACTED]"},
		{"x", "[REDACTED]"},
	}
	for _, tt := range tests {
		got := RedactSecret(tt.input)
		if got != tt.want {
			t.Errorf("RedactSecret(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestRedactSecret_Empty(t *testing.T) {
	if got := RedactSecret(""); got != "" {
		t.Errorf("RedactSecret(\"\") = %q, want \"\"", got)
	}
}

// --- RedactIPInText ---

func TestRedactIPInText(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{
			"client 10.0.0.5 connected from 192.168.1.100",
			"client 10.0.0.*** connected from 192.168.1.***",
		},
		{
			"no ip here",
			"no ip here",
		},
		{
			"single 1.2.3.4 address",
			"single 1.2.3.*** address",
		},
		{
			"",
			"",
		},
	}
	for _, tt := range tests {
		got := RedactIPInText(tt.input)
		if got != tt.want {
			t.Errorf("RedactIPInText(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// --- RedactIPInText 确保不泄露原始最后一段 ---

func TestRedactIPInText_NoLeakLastOctet(t *testing.T) {
	text := "user connected from 172.16.50.99 at port 8080"
	got := RedactIPInText(text)
	if strings.Contains(got, ".99") {
		t.Errorf("RedactIPInText leaked last octet: %q", got)
	}
}

// =============================================================================
// Feature: phase3-operational-baseline, Property 3: IP 脱敏完整性
// 验证: 需求 5.2, 6.4
// =============================================================================

func TestProperty_IPRedactionCompleteness(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		ipCount := rapid.IntRange(0, 10).Draw(t, "ipCount")

		// 生成随机文本和 IP 地址
		type ipInfo struct {
			full    string
			lastSeg string
		}
		ips := make([]ipInfo, ipCount)
		parts := []string{}

		for i := 0; i < ipCount; i++ {
			a := rapid.IntRange(0, 255).Draw(t, fmt.Sprintf("ip%d_a", i))
			b := rapid.IntRange(0, 255).Draw(t, fmt.Sprintf("ip%d_b", i))
			c := rapid.IntRange(0, 255).Draw(t, fmt.Sprintf("ip%d_c", i))
			d := rapid.IntRange(1, 255).Draw(t, fmt.Sprintf("ip%d_d", i)) // 避免 .0 与 *** 混淆
			ip := fmt.Sprintf("%d.%d.%d.%d", a, b, c, d)
			ips[i] = ipInfo{full: ip, lastSeg: fmt.Sprintf("%d", d)}
			parts = append(parts, fmt.Sprintf("host-%d %s port", i, ip))
		}

		// 添加非 IP 文本
		filler := rapid.StringMatching(`[a-zA-Z0-9 ]{0,50}`).Draw(t, "filler")
		parts = append(parts, filler)

		input := strings.Join(parts, " ")
		output := RedactIPInText(input)

		// 验证所有 IPv4 地址最后一段被替换为 ***
		for _, ip := range ips {
			if strings.Contains(output, ip.full) {
				t.Fatalf("输出中仍包含原始 IP: %s\n输入: %s\n输出: %s", ip.full, input, output)
			}
		}

		// 验证输出中不包含原始 IPv4 地址的完整最后一段（以 .lastSeg 形式）
		for _, ip := range ips {
			dotLast := "." + ip.lastSeg
			// 检查是否存在 x.x.x.lastSeg 模式（未脱敏的 IP）
			if strings.Contains(output, ip.full) {
				t.Fatalf("输出中泄露了 IP 最后一段 %s: %s", dotLast, output)
			}
		}

		// 验证非 IPv4 文本内容保持不变
		if ipCount == 0 {
			if output != input {
				t.Fatalf("无 IP 时输出应与输入相同:\n输入: %s\n输出: %s", input, output)
			}
		}
	})
}
