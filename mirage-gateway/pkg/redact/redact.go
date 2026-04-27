// Package redact 提供统一的日志脱敏函数，用于 Gateway/OS/Client 各组件的敏感字段脱敏。
//
// 脱敏规则：
//   - IP 地址 → x.x.x.***
//   - Token/Authorization → ***
//   - Password/Secret → [REDACTED]
package redact

import (
	"regexp"
	"strings"
)

// ipv4Re 匹配 IPv4 地址（简化版，覆盖日志中常见格式）
var ipv4Re = regexp.MustCompile(`\b(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.\d{1,3}\b`)

// RedactIP 对 IPv4 地址执行脱敏，保留前三段，最后一段替换为 ***。
// 例如: "192.168.1.100" → "192.168.1.***"
// 非 IPv4 格式的输入原样返回。
func RedactIP(ip string) string {
	if ip == "" {
		return ""
	}
	parts := strings.Split(ip, ".")
	if len(parts) != 4 {
		return ip
	}
	// 验证每段是否为数字
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

// RedactToken 对 token/Authorization 值执行脱敏，替换为 ***。
func RedactToken(token string) string {
	if token == "" {
		return ""
	}
	return "***"
}

// RedactSecret 对 password/secret 值执行脱敏，替换为 [REDACTED]。
func RedactSecret(secret string) string {
	if secret == "" {
		return ""
	}
	return "[REDACTED]"
}

// RedactIPInText 在文本中查找所有 IPv4 地址并脱敏。
// 例如: "client 10.0.0.5 connected" → "client 10.0.0.*** connected"
func RedactIPInText(text string) string {
	return ipv4Re.ReplaceAllString(text, "${1}.${2}.${3}.***")
}
