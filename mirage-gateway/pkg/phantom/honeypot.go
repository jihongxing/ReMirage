// Package phantom 蜜罐服务实现
package phantom

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"sync"
	"time"
)

// HoneypotServer 蜜罐服务器
type HoneypotServer struct {
	mu sync.RWMutex

	// 业务画像
	persona Persona

	// 访问记录
	accessLog []AccessRecord

	// 金丝雀令牌
	canaryTokens map[string]*CanaryToken

	// 延迟配置
	minDelay time.Duration
	maxDelay time.Duration

	// 回调
	onAccess func(record *AccessRecord)
	onCanary func(token *CanaryToken, ip string)
}

// AccessRecord 访问记录
type AccessRecord struct {
	Timestamp  time.Time
	RemoteAddr string
	Method     string
	Path       string
	UserAgent  string
	Headers    map[string]string
	ResponseMS int64
}

// CanaryToken 金丝雀令牌
type CanaryToken struct {
	ID        string
	Type      string // pdf, json, docx
	Created   time.Time
	Triggered bool
	TriggerIP string
	TriggerAt time.Time
}

// NewHoneypotServer 创建蜜罐服务器
func NewHoneypotServer() *HoneypotServer {
	return &HoneypotServer{
		persona:      DefaultPersona,
		accessLog:    make([]AccessRecord, 0, 10000),
		canaryTokens: make(map[string]*CanaryToken),
		minDelay:     100 * time.Millisecond,
		maxDelay:     3 * time.Second,
	}
}

// SetPersona 设置业务画像
func (h *HoneypotServer) SetPersona(p Persona) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.persona = p
}

// SetDelayRange 设置延迟范围
func (h *HoneypotServer) SetDelayRange(min, max time.Duration) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.minDelay = min
	h.maxDelay = max
}

// OnAccess 设置访问回调
func (h *HoneypotServer) OnAccess(fn func(record *AccessRecord)) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.onAccess = fn
}

// OnCanaryTrigger 设置金丝雀触发回调
func (h *HoneypotServer) OnCanaryTrigger(fn func(token *CanaryToken, ip string)) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.onCanary = fn
}

// Handler 返回 HTTP 处理器
func (h *HoneypotServer) Handler() http.Handler {
	mux := http.NewServeMux()

	// 伪造的 API 端点
	mux.HandleFunc("/api/users", h.handleFakeUsers)
	mux.HandleFunc("/api/config", h.handleFakeConfig)
	mux.HandleFunc("/api/logs", h.handleFakeLogs)

	// 金丝雀文件
	mux.HandleFunc("/files/", h.handleCanaryFile)
	mux.HandleFunc("/static/img/", h.handleCanaryCallback)
	mux.HandleFunc("/collect", h.handleCanaryCallback)

	// 默认处理
	mux.HandleFunc("/", h.handleDefault)

	return h.withLogging(h.withDelay(mux))
}

// withDelay 添加随机延迟
func (h *HoneypotServer) withDelay(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.mu.RLock()
		min, max := h.minDelay, h.maxDelay
		h.mu.RUnlock()

		// 随机延迟
		delayRange := max - min
		if delayRange > 0 {
			n, _ := rand.Int(rand.Reader, big.NewInt(int64(delayRange)))
			delay := min + time.Duration(n.Int64())
			time.Sleep(delay)
		}

		next.ServeHTTP(w, r)
	})
}

// withLogging 添加日志记录
func (h *HoneypotServer) withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		next.ServeHTTP(w, r)

		record := AccessRecord{
			Timestamp:  start,
			RemoteAddr: r.RemoteAddr,
			Method:     r.Method,
			Path:       r.URL.Path,
			UserAgent:  r.UserAgent(),
			Headers:    make(map[string]string),
			ResponseMS: time.Since(start).Milliseconds(),
		}

		// 记录关键 Headers
		for _, key := range []string{"Accept", "Accept-Language", "X-Forwarded-For"} {
			if v := r.Header.Get(key); v != "" {
				record.Headers[key] = v
			}
		}

		h.mu.Lock()
		h.accessLog = append(h.accessLog, record)
		if len(h.accessLog) > 10000 {
			h.accessLog = h.accessLog[1000:]
		}
		callback := h.onAccess
		h.mu.Unlock()

		if callback != nil {
			callback(&record)
		}
	})
}

// handleFakeUsers 伪造用户数据
func (h *HoneypotServer) handleFakeUsers(w http.ResponseWriter, r *http.Request) {
	users := h.generateFakeUsers(10)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "success",
		"data":   users,
		"total":  len(users),
	})
}

// handleFakeConfig 伪造配置
func (h *HoneypotServer) handleFakeConfig(w http.ResponseWriter, r *http.Request) {
	config := map[string]interface{}{
		"version":     "1.2.3",
		"environment": "production",
		"features": map[string]bool{
			"analytics": true,
			"logging":   true,
			"debug":     false,
		},
		"limits": map[string]int{
			"max_connections": 1000,
			"timeout_seconds": 30,
		},
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(config)
}

// handleFakeLogs 伪造日志
func (h *HoneypotServer) handleFakeLogs(w http.ResponseWriter, r *http.Request) {
	logs := h.generateFakeLogs(20)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"logs":  logs,
		"count": len(logs),
	})
}

// handleCanaryFile 处理金丝雀文件下载
func (h *HoneypotServer) handleCanaryFile(w http.ResponseWriter, r *http.Request) {
	// 生成金丝雀令牌
	token := h.createCanaryToken("json")

	// 生成带追踪的伪造文件
	content := map[string]interface{}{
		"data":   h.generateRandomData(),
		"ref":    token.ID,
		"format": "json",
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", "attachment; filename=export.json")
	json.NewEncoder(w).Encode(content)
}

// handleCanaryCallback 处理金丝雀回调
func (h *HoneypotServer) handleCanaryCallback(w http.ResponseWriter, r *http.Request) {
	tokenID := r.URL.Query().Get("t")
	if tokenID == "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	h.mu.Lock()
	token, ok := h.canaryTokens[tokenID]
	if ok && !token.Triggered {
		token.Triggered = true
		token.TriggerIP = r.RemoteAddr
		token.TriggerAt = time.Now()
	}
	callback := h.onCanary
	h.mu.Unlock()

	if ok && callback != nil {
		callback(token, r.RemoteAddr)
	}

	// 返回透明像素
	w.Header().Set("Content-Type", "image/gif")
	w.Write([]byte{0x47, 0x49, 0x46, 0x38, 0x39, 0x61, 0x01, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x21, 0xf9, 0x04, 0x01, 0x00, 0x00, 0x00, 0x00, 0x2c, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0x02, 0x01, 0x00, 0x00})
}

// handleDefault 默认处理
func (h *HoneypotServer) handleDefault(w http.ResponseWriter, r *http.Request) {
	p := h.persona
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(fmt.Sprintf(`<!DOCTYPE html>
<html><head><title>%s</title>
<style>body{font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;text-align:center;padding:50px;background:#f5f5f5}h1{color:#333}p{color:#666}.footer{padding:40px;color:#999;font-size:0.85em}</style>
</head><body>
<h1>Welcome</h1><p>Please authenticate to continue.</p>
<div class="footer">&copy; %d %s</div>
</body></html>`, p.CompanyName, p.CopyrightYear, p.CompanyName)))
}

// createCanaryToken 创建金丝雀令牌
func (h *HoneypotServer) createCanaryToken(tokenType string) *CanaryToken {
	id := generateTokenID()
	token := &CanaryToken{
		ID:      id,
		Type:    tokenType,
		Created: time.Now(),
	}

	h.mu.Lock()
	h.canaryTokens[id] = token
	h.mu.Unlock()

	return token
}

// generateFakeUsers 生成伪造用户
func (h *HoneypotServer) generateFakeUsers(count int) []map[string]interface{} {
	users := make([]map[string]interface{}, count)
	for i := 0; i < count; i++ {
		users[i] = map[string]interface{}{
			"id":         fmt.Sprintf("usr_%s", generateTokenID()[:8]),
			"email":      fmt.Sprintf("user%d@example.com", i),
			"created_at": time.Now().Add(-time.Duration(i*24) * time.Hour).Format(time.RFC3339),
			"status":     "active",
		}
	}
	return users
}

// generateFakeLogs 生成伪造日志
func (h *HoneypotServer) generateFakeLogs(count int) []map[string]interface{} {
	logs := make([]map[string]interface{}, count)
	levels := []string{"INFO", "DEBUG", "WARN"}
	for i := 0; i < count; i++ {
		logs[i] = map[string]interface{}{
			"timestamp": time.Now().Add(-time.Duration(i) * time.Minute).Format(time.RFC3339),
			"level":     levels[i%len(levels)],
			"message":   fmt.Sprintf("Operation completed: %s", generateTokenID()[:6]),
		}
	}
	return logs
}

// generateRandomData 生成随机数据
func (h *HoneypotServer) generateRandomData() string {
	b := make([]byte, 64)
	rand.Read(b)
	return base64.StdEncoding.EncodeToString(b)
}

// GetAccessLog 获取访问日志
func (h *HoneypotServer) GetAccessLog() []AccessRecord {
	h.mu.RLock()
	defer h.mu.RUnlock()
	result := make([]AccessRecord, len(h.accessLog))
	copy(result, h.accessLog)
	return result
}

// GetCanaryTokens 获取金丝雀令牌
func (h *HoneypotServer) GetCanaryTokens() []*CanaryToken {
	h.mu.RLock()
	defer h.mu.RUnlock()
	tokens := make([]*CanaryToken, 0, len(h.canaryTokens))
	for _, t := range h.canaryTokens {
		tokens = append(tokens, t)
	}
	return tokens
}

func generateTokenID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return base64.URLEncoding.EncodeToString(b)[:22]
}
