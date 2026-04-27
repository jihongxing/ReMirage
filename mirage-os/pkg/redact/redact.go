// Package redact 提供统一的日志脱敏函数。
package redact

import "strings"

// IP 对 IPv4 地址脱敏：保留前三段，最后一段替换为 ***。
func IP(ip string) string {
	if ip == "" {
		return ""
	}
	parts := strings.Split(ip, ".")
	if len(parts) != 4 {
		return ip
	}
	for _, p := range parts {
		if p == "" {
			return ip
		}
		for _, c := range p {
			if c < '0' || c > '9' {
				return ip
			}
		}
	}
	return parts[0] + "." + parts[1] + "." + parts[2] + ".***"
}

// Token 对 userID/token/session 等标识符脱敏。
func Token(s string) string {
	if s == "" {
		return ""
	}
	return "***"
}

// Secret 对密码/密钥脱敏。
func Secret(s string) string {
	if s == "" {
		return ""
	}
	return "[REDACTED]"
}
