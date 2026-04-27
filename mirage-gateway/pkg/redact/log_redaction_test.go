package redact

import (
	"fmt"
	"strings"
	"testing"
)

// TestLogRedaction_UserID 验证 RedactToken 对 userID 的脱敏效果
func TestLogRedaction_UserID(t *testing.T) {
	userIDs := []string{"u-abc123", "user_001", "gw-deadbeef1234"}
	for _, uid := range userIDs {
		redacted := RedactToken(uid)
		if redacted != "***" {
			t.Errorf("RedactToken(%q) = %q, want '***'", uid, redacted)
		}
		// 模拟日志输出，确认原始值不泄露
		logLine := fmt.Sprintf("[Handler] 配额桶已更新: user=%s", RedactToken(uid))
		if strings.Contains(logLine, uid) {
			t.Errorf("日志仍包含原始 userID %q: %s", uid, logLine)
		}
	}
}

// TestLogRedaction_SourceIP 验证 RedactIP 对 SourceIP 的脱敏效果
func TestLogRedaction_SourceIP(t *testing.T) {
	cases := []struct {
		ip       string
		wantLast string // 最后一段应被替换
	}{
		{"192.168.1.100", "192.168.1.***"},
		{"10.0.0.5", "10.0.0.***"},
		{"172.16.50.99", "172.16.50.***"},
	}
	for _, tc := range cases {
		redacted := RedactIP(tc.ip)
		if redacted != tc.wantLast {
			t.Errorf("RedactIP(%q) = %q, want %q", tc.ip, redacted, tc.wantLast)
		}
		// 模拟日志输出，确认最后一段不泄露
		logLine := fmt.Sprintf("[Responder] 自动封禁: %s", RedactIP(tc.ip))
		lastOctet := tc.ip[strings.LastIndex(tc.ip, ".")+1:]
		if strings.Contains(logLine, "."+lastOctet) {
			t.Errorf("日志泄露了 IP 最后一段 %q: %s", lastOctet, logLine)
		}
	}
}

// TestLogRedaction_RemoteAddr 验证 RedactIP 对 host:port 格式 RemoteAddr 的处理
func TestLogRedaction_RemoteAddr(t *testing.T) {
	// RemoteAddr 通常是 "ip:port" 格式，调用方应先 SplitHostPort 或直接传 IP
	// 这里验证纯 IP 输入的脱敏
	addr := "203.0.113.42"
	redacted := RedactIP(addr)
	if redacted != "203.0.113.***" {
		t.Errorf("RedactIP(%q) = %q, want 203.0.113.***", addr, redacted)
	}
}

// TestLogRedaction_Token 验证 RedactToken 对 JWT/API token 的脱敏效果
func TestLogRedaction_Token(t *testing.T) {
	tokens := []string{
		"Bearer eyJhbGciOiJIUzI1NiJ9.payload.signature",
		"sk-live-abc123def456",
		"ghp_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
	}
	for _, tok := range tokens {
		redacted := RedactToken(tok)
		if redacted != "***" {
			t.Errorf("RedactToken(%q) = %q, want '***'", tok, redacted)
		}
	}
}

// TestLogRedaction_IPInText 验证 RedactIPInText 对日志文本中嵌入 IP 的批量脱敏
func TestLogRedaction_IPInText(t *testing.T) {
	text := "[Responder] 策略封禁: 192.168.1.100 (action=drop) from 10.0.0.5"
	redacted := RedactIPInText(text)
	if strings.Contains(redacted, ".100") {
		t.Errorf("RedactIPInText 泄露了 .100: %s", redacted)
	}
	if strings.Contains(redacted, ".5 ") || strings.HasSuffix(redacted, ".5") {
		t.Errorf("RedactIPInText 泄露了 .5: %s", redacted)
	}
	if !strings.Contains(redacted, "192.168.1.***") {
		t.Errorf("RedactIPInText 未正确脱敏第一个 IP: %s", redacted)
	}
}
