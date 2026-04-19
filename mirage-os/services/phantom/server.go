// Package phantom 影子服务器
// 运行蜜罐服务并生成混淆内容
package phantom

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Server 影子服务器
type Server struct {
	mu sync.RWMutex

	honeypot *HoneypotServer
	httpSrv  *http.Server

	// WebSocket 广播
	wsClients map[*websocket.Conn]bool
	wsMu      sync.RWMutex
	upgrader  websocket.Upgrader

	// 统计
	stats PhantomStats
}

// PhantomStats 统计信息
type PhantomStats struct {
	TotalRedirected  int64 `json:"totalRedirected"`
	ActiveTraps      int   `json:"activeTraps"`
	RequestsConsumed int64 `json:"requestsConsumed"`
	CanaryTriggered  int   `json:"canaryTriggered"`
}

// PhantomEvent WebSocket 事件
type PhantomEvent struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload"`
}

// NewServer 创建影子服务器
func NewServer() *Server {
	s := &Server{
		honeypot:  NewHoneypotServer(),
		wsClients: make(map[*websocket.Conn]bool),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}

	// 设置回调
	s.honeypot.OnAccess(s.onHoneypotAccess)
	s.honeypot.OnCanaryTrigger(s.onCanaryTrigger)

	return s
}

// Start 启动服务器
func (s *Server) Start(addr string) error {
	mux := http.NewServeMux()

	// 蜜罐路由
	mux.Handle("/", s.honeypot.Handler())

	// 管理 API
	mux.HandleFunc("/admin/stats", s.handleStats)
	mux.HandleFunc("/admin/ws", s.handleWebSocket)

	s.httpSrv = &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	log.Printf("[Phantom] 影子服务器启动: %s", addr)
	return s.httpSrv.ListenAndServe()
}

// Stop 停止服务器
func (s *Server) Stop(ctx context.Context) error {
	return s.httpSrv.Shutdown(ctx)
}

// onHoneypotAccess 蜜罐访问回调
func (s *Server) onHoneypotAccess(record *AccessRecord) {
	s.mu.Lock()
	s.stats.RequestsConsumed++
	s.mu.Unlock()

	// 广播事件
	event := PhantomEvent{
		Type: "phantom_event",
		Payload: map[string]interface{}{
			"srcIP":        record.RemoteAddr,
			"path":         record.Path,
			"userAgent":    record.UserAgent,
			"responseMs":   record.ResponseMS,
			"requestCount": s.stats.RequestsConsumed,
			"honeypotId":   1,
			"country":      "Unknown", // 实际应从 GeoIP 获取
		},
	}
	s.broadcast(event)
}

// onCanaryTrigger 金丝雀触发回调
func (s *Server) onCanaryTrigger(token *CanaryToken, ip string) {
	s.mu.Lock()
	s.stats.CanaryTriggered++
	s.mu.Unlock()

	log.Printf("[Phantom] 金丝雀触发! Token: %s, IP: %s", token.ID, ip)

	event := PhantomEvent{
		Type: "canary_triggered",
		Payload: map[string]interface{}{
			"tokenId":   token.ID,
			"tokenType": token.Type,
			"triggerIP": ip,
			"timestamp": time.Now().Unix(),
		},
	}
	s.broadcast(event)
}

// handleStats 统计 API
func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	stats := s.stats
	s.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// handleWebSocket WebSocket 连接
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[Phantom] WebSocket 升级失败: %v", err)
		return
	}

	s.wsMu.Lock()
	s.wsClients[conn] = true
	s.wsMu.Unlock()

	defer func() {
		s.wsMu.Lock()
		delete(s.wsClients, conn)
		s.wsMu.Unlock()
		conn.Close()
	}()

	// 发送初始统计
	s.mu.RLock()
	stats := s.stats
	s.mu.RUnlock()

	conn.WriteJSON(PhantomEvent{
		Type:    "phantom_stats",
		Payload: stats,
	})

	// 保持连接
	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			break
		}
	}
}

// broadcast 广播消息
func (s *Server) broadcast(event PhantomEvent) {
	s.wsMu.RLock()
	defer s.wsMu.RUnlock()

	for conn := range s.wsClients {
		if err := conn.WriteJSON(event); err != nil {
			log.Printf("[Phantom] 广播失败: %v", err)
		}
	}
}

// SetStats 设置统计（供外部更新）
func (s *Server) SetStats(redirected int64, activeTraps int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stats.TotalRedirected = redirected
	s.stats.ActiveTraps = activeTraps
}

// GetHoneypot 获取蜜罐实例
func (s *Server) GetHoneypot() *HoneypotServer {
	return s.honeypot
}

// HoneypotServer 蜜罐服务器（从 gateway 复制，避免循环依赖）
// 实际部署时应使用 gateway 的 phantom 包
type HoneypotServer struct {
	mu           sync.RWMutex
	accessLog    []AccessRecord
	canaryTokens map[string]*CanaryToken
	minDelay     time.Duration
	maxDelay     time.Duration
	onAccess     func(record *AccessRecord)
	onCanary     func(token *CanaryToken, ip string)
}

type AccessRecord struct {
	Timestamp  time.Time
	RemoteAddr string
	Method     string
	Path       string
	UserAgent  string
	Headers    map[string]string
	ResponseMS int64
}

type CanaryToken struct {
	ID        string
	Type      string
	Created   time.Time
	Triggered bool
	TriggerIP string
	TriggerAt time.Time
}

func NewHoneypotServer() *HoneypotServer {
	return &HoneypotServer{
		accessLog:    make([]AccessRecord, 0, 10000),
		canaryTokens: make(map[string]*CanaryToken),
		minDelay:     100 * time.Millisecond,
		maxDelay:     3 * time.Second,
	}
}

func (h *HoneypotServer) OnAccess(fn func(record *AccessRecord)) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.onAccess = fn
}

func (h *HoneypotServer) OnCanaryTrigger(fn func(token *CanaryToken, ip string)) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.onCanary = fn
}

func (h *HoneypotServer) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/users", h.handleFakeUsers)
	mux.HandleFunc("/api/config", h.handleFakeConfig)
	mux.HandleFunc("/files/", h.handleCanaryFile)
	mux.HandleFunc("/canary/", h.handleCanaryCallback)
	mux.HandleFunc("/", h.handleDefault)
	return h.withLogging(h.withDelay(mux))
}

func (h *HoneypotServer) withDelay(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.mu.RLock()
		min, max := h.minDelay, h.maxDelay
		h.mu.RUnlock()
		delay := min + time.Duration(float64(max-min)*0.5)
		time.Sleep(delay)
		next.ServeHTTP(w, r)
	})
}

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
			ResponseMS: time.Since(start).Milliseconds(),
		}
		h.mu.Lock()
		h.accessLog = append(h.accessLog, record)
		callback := h.onAccess
		h.mu.Unlock()
		if callback != nil {
			callback(&record)
		}
	})
}

func (h *HoneypotServer) handleFakeUsers(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "success",
		"data": []map[string]interface{}{
			{"id": "usr_001", "email": "user1@example.com", "status": "active"},
			{"id": "usr_002", "email": "user2@example.com", "status": "active"},
		},
	})
}

func (h *HoneypotServer) handleFakeConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"version": "1.2.3",
		"env":     "production",
	})
}

func (h *HoneypotServer) handleCanaryFile(w http.ResponseWriter, r *http.Request) {
	tokenID := fmt.Sprintf("canary_%d", time.Now().UnixNano())
	h.mu.Lock()
	h.canaryTokens[tokenID] = &CanaryToken{ID: tokenID, Type: "json", Created: time.Now()}
	h.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", "attachment; filename=export.json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"classification": "CONFIDENTIAL",
		"_tracking":      tokenID,
	})
}

func (h *HoneypotServer) handleCanaryCallback(w http.ResponseWriter, r *http.Request) {
	tokenID := r.URL.Query().Get("t")
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
	w.Header().Set("Content-Type", "image/gif")
	w.Write([]byte{0x47, 0x49, 0x46, 0x38, 0x39, 0x61, 0x01, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x21, 0xf9, 0x04, 0x01, 0x00, 0x00, 0x00, 0x00, 0x2c, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0x02, 0x01, 0x00, 0x00})
}

func (h *HoneypotServer) handleDefault(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(`<!DOCTYPE html><html><head><title>Service Portal</title></head><body><h1>Welcome</h1></body></html>`))
}
