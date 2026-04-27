package redact

import (
	"fmt"
	"strings"
	"testing"
)

func TestIP_Standard(t *testing.T) {
	cases := []struct{ in, want string }{
		{"192.168.1.100", "192.168.1.***"},
		{"10.0.0.5", "10.0.0.***"},
		{"172.16.50.99", "172.16.50.***"},
	}
	for _, tc := range cases {
		if got := IP(tc.in); got != tc.want {
			t.Errorf("IP(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestIP_Empty(t *testing.T) {
	if got := IP(""); got != "" {
		t.Errorf("IP(\"\") = %q, want \"\"", got)
	}
}

func TestIP_NonIPv4(t *testing.T) {
	for _, in := range []string{"not-ip", "::1", "abc"} {
		if got := IP(in); got != in {
			t.Errorf("IP(%q) = %q, want unchanged", in, got)
		}
	}
}

func TestToken_Standard(t *testing.T) {
	for _, in := range []string{"u-abc123", "Bearer eyJ...", "sk-live-xxx"} {
		if got := Token(in); got != "***" {
			t.Errorf("Token(%q) = %q, want '***'", in, got)
		}
	}
}

func TestToken_Empty(t *testing.T) {
	if got := Token(""); got != "" {
		t.Errorf("Token(\"\") = %q, want \"\"", got)
	}
}

func TestSecret_Standard(t *testing.T) {
	if got := Secret("my-password"); got != "[REDACTED]" {
		t.Errorf("Secret(\"my-password\") = %q, want [REDACTED]", got)
	}
}

func TestLogRedaction_NoLeakUserID(t *testing.T) {
	uid := "user_sensitive_123"
	logLine := fmt.Sprintf("[Handler] user=%s", Token(uid))
	if strings.Contains(logLine, uid) {
		t.Errorf("日志泄露了原始 userID: %s", logLine)
	}
}

func TestLogRedaction_NoLeakIPLastOctet(t *testing.T) {
	ip := "172.16.50.99"
	logLine := fmt.Sprintf("[Access] ip=%s", IP(ip))
	if strings.Contains(logLine, ".99") {
		t.Errorf("日志泄露了 IP 最后一段: %s", logLine)
	}
}
