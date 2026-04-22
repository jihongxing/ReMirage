// Package logger - 日志脱敏与分级
package logger

import (
	"fmt"
	"strings"
)

// Sanitizer 日志脱敏处理器
type Sanitizer struct{}

// NewSanitizer 创建脱敏处理器
func NewSanitizer() *Sanitizer {
	return &Sanitizer{}
}

// SanitizeIP 截断 IP 为 /24 网段: "1.2.3.4" → "1.2.3.*/24"
func (s *Sanitizer) SanitizeIP(ip string) string {
	if ip == "" {
		return ""
	}
	parts := strings.Split(ip, ".")
	if len(parts) != 4 {
		return ip // 非 IPv4，原样返回
	}
	return fmt.Sprintf("%s.%s.%s.*/24", parts[0], parts[1], parts[2])
}

// SanitizeUserID 截断 userID: "abc12345-xxxx-yyyy" → "abc12345..."
func (s *Sanitizer) SanitizeUserID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8] + "..."
}

// SanitizeToken 只保留前 4 位: "abcdef123456" → "abcd****"
func (s *Sanitizer) SanitizeToken(token string) string {
	if len(token) <= 4 {
		return token
	}
	return token[:4] + "****"
}
