// Package gtunnel - Chameleon TCP 降级通道
// 当 UDP/QUIC 被封锁时，自动降级为 WebSocket over mTLS
// 外部审查者看到的是：发往云厂商 IP 的普通 HTTPS 流量
package gtunnel

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"mirage-gateway/pkg/redact"
	"net"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

// ============================================================
// 网关侧：WebSocket 降级接收端 (Listener)
// ============================================================

// ChameleonListener 变色龙降级监听器
// 在网关侧监听 TCP 443，接受客户端的 WebSocket 降级连接
type ChameleonListener struct {
	mu sync.RWMutex

	// HTTP Server（伪装为正常 HTTPS 服务）
	httpServer *http.Server
	upgrader   websocket.Upgrader

	// mTLS 配置
	tlsConfig *tls.Config

	// 活跃连接
	connections map[string]*ChameleonServerConn
	connMu      sync.RWMutex

	// 收包回调：收到 IP 包后注入回 G-Tunnel 主流程
	onPacketRecv func(clientID string, data []byte)

	// 新连接回调
	onClientConnect func(clientID string, conn *ChameleonServerConn)

	// 配置
	config ChameleonListenerConfig

	// 统计
	stats ChameleonStats

	// 控制
	ctx    context.Context
	cancel context.CancelFunc
}

// ChameleonListenerConfig 监听器配置
type ChameleonListenerConfig struct {
	ListenAddr string // 监听地址 (默认 :443)
	WSPath     string // WebSocket 路径 (默认 /api/v2/stream)

	// TLS 证书
	CertFile string
	KeyFile  string
	CAFile   string // 客户端 CA（mTLS）

	// 伪装配置
	FakeServerName string // 伪装的 Server 头
	FakeBody       []byte // 非 WebSocket 请求返回的假页面

	// 限制
	MaxConnections int           // 最大连接数
	IdleTimeout    time.Duration // 空闲超时
	ReadLimit      int64         // 单消息最大字节
}

// ChameleonStats 统计
type ChameleonStats struct {
	TotalConnections  int64
	ActiveConnections int64
	BytesReceived     int64
	BytesSent         int64
	PacketsReceived   int64
	PacketsSent       int64
	FallbackEvents    int64
}

// DefaultChameleonListenerConfig 默认配置
func DefaultChameleonListenerConfig() ChameleonListenerConfig {
	return ChameleonListenerConfig{
		ListenAddr:     ":443",
		WSPath:         "/api/v2/stream",
		FakeServerName: "cloudflare",
		MaxConnections: 1000,
		IdleTimeout:    60 * time.Second,
		ReadLimit:      65536, // 64KB
	}
}

// NewChameleonListener 创建变色龙监听器
func NewChameleonListener(config ChameleonListenerConfig) (*ChameleonListener, error) {
	ctx, cancel := context.WithCancel(context.Background())

	cl := &ChameleonListener{
		connections: make(map[string]*ChameleonServerConn),
		config:      config,
		ctx:         ctx,
		cancel:      cancel,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  65536,
			WriteBufferSize: 65536,
			CheckOrigin:     func(r *http.Request) bool { return true },
		},
	}

	// 构建 mTLS 配置
	tlsConfig, err := cl.buildTLSConfig()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("TLS 配置失败: %w", err)
	}
	cl.tlsConfig = tlsConfig

	return cl, nil
}

// SetPacketCallback 设置收包回调
func (cl *ChameleonListener) SetPacketCallback(cb func(clientID string, data []byte)) {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	cl.onPacketRecv = cb
}

// SetClientConnectCallback 设置新连接回调
func (cl *ChameleonListener) SetClientConnectCallback(cb func(clientID string, conn *ChameleonServerConn)) {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	cl.onClientConnect = cb
}

// Start 启动监听器
func (cl *ChameleonListener) Start() error {
	mux := http.NewServeMux()

	// WebSocket 降级端点
	mux.HandleFunc(cl.config.WSPath, cl.handleWebSocket)

	// 伪装页面（非 WS 请求返回假内容）
	mux.HandleFunc("/", cl.handleFakePage)

	cl.httpServer = &http.Server{
		Addr:      cl.config.ListenAddr,
		Handler:   mux,
		TLSConfig: cl.tlsConfig,
		// 伪装 HTTP 头
		ErrorLog: log.New(io.Discard, "", 0),
	}

	log.Printf("🦎 [Chameleon] TCP 降级通道已启动: %s%s", cl.config.ListenAddr, cl.config.WSPath)

	go func() {
		var err error
		if cl.config.CertFile != "" {
			err = cl.httpServer.ListenAndServeTLS(cl.config.CertFile, cl.config.KeyFile)
		} else {
			err = cl.httpServer.ListenAndServe()
		}
		if err != nil && err != http.ErrServerClosed {
			log.Printf("⚠️ [Chameleon] 监听器错误: %v", err)
		}
	}()

	return nil
}

// Stop 停止监听器
func (cl *ChameleonListener) Stop() error {
	cl.cancel()

	// 关闭所有连接
	cl.connMu.Lock()
	for _, conn := range cl.connections {
		conn.Close()
	}
	cl.connections = make(map[string]*ChameleonServerConn)
	cl.connMu.Unlock()

	// 关闭 HTTP 服务器
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return cl.httpServer.Shutdown(ctx)
}

// handleWebSocket 处理 WebSocket 升级请求
func (cl *ChameleonListener) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	// 检查连接数限制
	cl.connMu.RLock()
	if len(cl.connections) >= cl.config.MaxConnections {
		cl.connMu.RUnlock()
		http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
		return
	}
	cl.connMu.RUnlock()

	// 从 mTLS 证书提取客户端 ID
	clientID := cl.extractClientID(r)
	if clientID == "" {
		// 非法客户端，返回假页面
		cl.handleFakePage(w, r)
		return
	}

	// 升级为 WebSocket
	wsConn, err := cl.upgrader.Upgrade(w, r, http.Header{
		"Server": []string{cl.config.FakeServerName},
	})
	if err != nil {
		log.Printf("⚠️ [Chameleon] WebSocket 升级失败: %v", err)
		return
	}

	// 设置读取限制
	wsConn.SetReadLimit(cl.config.ReadLimit)

	// 创建服务端连接
	conn := &ChameleonServerConn{
		clientID:  clientID,
		wsConn:    wsConn,
		createdAt: time.Now(),
		sendChan:  make(chan []byte, 256),
	}

	// 注册连接
	cl.connMu.Lock()
	cl.connections[clientID] = conn
	cl.connMu.Unlock()
	atomic.AddInt64(&cl.stats.TotalConnections, 1)
	atomic.AddInt64(&cl.stats.ActiveConnections, 1)
	atomic.AddInt64(&cl.stats.FallbackEvents, 1)

	log.Printf("🦎 [Chameleon] 客户端降级连接: %s (来自 %s)", clientID, redact.RedactIP(r.RemoteAddr))

	// 通知上层
	cl.mu.RLock()
	onConnect := cl.onClientConnect
	cl.mu.RUnlock()
	if onConnect != nil {
		onConnect(clientID, conn)
	}

	// 启动读写循环
	go cl.readLoop(conn)
	go cl.writeLoop(conn)
}

// handleFakePage 返回伪装页面（对非法请求/爬虫/DPI 探测）
func (cl *ChameleonListener) handleFakePage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Server", cl.config.FakeServerName)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Strict-Transport-Security", "max-age=31536000")

	if cl.config.FakeBody != nil {
		w.Write(cl.config.FakeBody)
	} else {
		// 默认返回一个看起来像 CDN 错误页的内容
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`<!DOCTYPE html><html><head><title>403 Forbidden</title></head><body><center><h1>403 Forbidden</h1></center><hr><center>cloudflare</center></body></html>`))
	}
}

// readLoop 读取循环：从 WebSocket 读取客户端发来的 IP 包
func (cl *ChameleonListener) readLoop(conn *ChameleonServerConn) {
	defer cl.removeConnection(conn)

	// 为每个连接创建控制帧路由器（处理 WebRTC 信令偷渡）
	router := NewCtrlFrameRouter(conn, WebRTCTransportConfig{})
	router.SetWebRTCPacketCallback(func(clientID string, data []byte) {
		cl.mu.RLock()
		onRecv := cl.onPacketRecv
		cl.mu.RUnlock()
		if onRecv != nil {
			onRecv(clientID, data)
		}
	})
	defer router.Close()

	for {
		select {
		case <-cl.ctx.Done():
			return
		default:
		}

		// 设置读取超时
		conn.wsConn.SetReadDeadline(time.Now().Add(cl.config.IdleTimeout))

		_, message, err := conn.wsConn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.Printf("⚠️ [Chameleon] 读取错误 (%s): %v", conn.clientID, err)
			}
			return
		}

		// 解帧：提取原始 IP 包
		packets, err := cl.deframe(message)
		if err != nil {
			log.Printf("⚠️ [Chameleon] 解帧错误 (%s): %v", conn.clientID, err)
			continue
		}

		// 注入回 G-Tunnel 主流程
		cl.mu.RLock()
		onRecv := cl.onPacketRecv
		cl.mu.RUnlock()

		for _, pkt := range packets {
			// 拦截控制帧（WebRTC 信令偷渡）
			if router.HandleControlFrame(pkt) {
				continue
			}

			atomic.AddInt64(&cl.stats.PacketsReceived, 1)
			atomic.AddInt64(&cl.stats.BytesReceived, int64(len(pkt)))
			if onRecv != nil {
				onRecv(conn.clientID, pkt)
			}
		}
	}
}

// writeLoop 写入循环：向客户端发送 IP 包
func (cl *ChameleonListener) writeLoop(conn *ChameleonServerConn) {
	for {
		select {
		case <-cl.ctx.Done():
			return
		case data, ok := <-conn.sendChan:
			if !ok {
				return
			}

			// 封帧
			frame := cl.enframe(data)

			conn.wsConn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			err := conn.wsConn.WriteMessage(websocket.BinaryMessage, frame)
			if err != nil {
				log.Printf("⚠️ [Chameleon] 写入错误 (%s): %v", conn.clientID, err)
				return
			}

			atomic.AddInt64(&cl.stats.PacketsSent, 1)
			atomic.AddInt64(&cl.stats.BytesSent, int64(len(data)))
		}
	}
}

// SendToClient 向指定客户端发送数据
func (cl *ChameleonListener) SendToClient(clientID string, data []byte) error {
	cl.connMu.RLock()
	conn, exists := cl.connections[clientID]
	cl.connMu.RUnlock()

	if !exists {
		return fmt.Errorf("客户端不存在: %s", clientID)
	}

	select {
	case conn.sendChan <- data:
		return nil
	default:
		return fmt.Errorf("发送缓冲区已满: %s", clientID)
	}
}

// removeConnection 移除连接
func (cl *ChameleonListener) removeConnection(conn *ChameleonServerConn) {
	cl.connMu.Lock()
	delete(cl.connections, conn.clientID)
	cl.connMu.Unlock()
	atomic.AddInt64(&cl.stats.ActiveConnections, -1)

	conn.Close()
	log.Printf("🦎 [Chameleon] 客户端断开: %s", conn.clientID)
}

// extractClientID 从 mTLS 证书提取客户端 ID
func (cl *ChameleonListener) extractClientID(r *http.Request) string {
	if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
		// 无客户端证书 → 非法连接
		return ""
	}

	// 使用客户端证书的 CN 作为 ID
	return r.TLS.PeerCertificates[0].Subject.CommonName
}

// buildTLSConfig 构建 mTLS 配置
func (cl *ChameleonListener) buildTLSConfig() (*tls.Config, error) {
	config := &tls.Config{
		MinVersion: tls.VersionTLS13,
		// 使用与 Chrome/Firefox 一致的密码套件顺序
		CipherSuites: []uint16{
			tls.TLS_AES_128_GCM_SHA256,
			tls.TLS_AES_256_GCM_SHA384,
			tls.TLS_CHACHA20_POLY1305_SHA256,
		},
		// ALPN: 伪装为 HTTP/2
		NextProtos: []string{"h2", "http/1.1"},
	}

	// 加载客户端 CA（mTLS 验证）
	if cl.config.CAFile != "" {
		caCert, err := os.ReadFile(cl.config.CAFile)
		if err != nil {
			return nil, fmt.Errorf("读取 CA 证书失败: %w", err)
		}

		caPool := x509.NewCertPool()
		if !caPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("解析 CA 证书失败")
		}

		config.ClientCAs = caPool
		config.ClientAuth = tls.RequireAndVerifyClientCert
	}

	return config, nil
}

// ============================================================
// 帧协议：将 IP 包封装为 WebSocket 消息
// 格式：[2 bytes length][payload][2 bytes length][payload]...
// ============================================================

// enframe 封帧：将单个 IP 包封装
func (cl *ChameleonListener) enframe(data []byte) []byte {
	frame := make([]byte, 2+len(data))
	binary.BigEndian.PutUint16(frame[0:2], uint16(len(data)))
	copy(frame[2:], data)
	return frame
}

// deframe 解帧：从 WebSocket 消息中提取 IP 包（支持多包合并）
func (cl *ChameleonListener) deframe(message []byte) ([][]byte, error) {
	var packets [][]byte
	offset := 0

	for offset < len(message) {
		if offset+2 > len(message) {
			return nil, fmt.Errorf("帧头不完整")
		}

		length := int(binary.BigEndian.Uint16(message[offset : offset+2]))
		offset += 2

		if offset+length > len(message) {
			return nil, fmt.Errorf("帧数据不完整: need %d, have %d", length, len(message)-offset)
		}

		pkt := make([]byte, length)
		copy(pkt, message[offset:offset+length])
		packets = append(packets, pkt)
		offset += length
	}

	return packets, nil
}

// GetStats 获取统计
func (cl *ChameleonListener) GetStats() ChameleonStats {
	return ChameleonStats{
		TotalConnections:  atomic.LoadInt64(&cl.stats.TotalConnections),
		ActiveConnections: atomic.LoadInt64(&cl.stats.ActiveConnections),
		BytesReceived:     atomic.LoadInt64(&cl.stats.BytesReceived),
		BytesSent:         atomic.LoadInt64(&cl.stats.BytesSent),
		PacketsReceived:   atomic.LoadInt64(&cl.stats.PacketsReceived),
		PacketsSent:       atomic.LoadInt64(&cl.stats.PacketsSent),
		FallbackEvents:    atomic.LoadInt64(&cl.stats.FallbackEvents),
	}
}

// GetActiveConnections 获取活跃连接列表
func (cl *ChameleonListener) GetActiveConnections() []string {
	cl.connMu.RLock()
	defer cl.connMu.RUnlock()

	ids := make([]string, 0, len(cl.connections))
	for id := range cl.connections {
		ids = append(ids, id)
	}
	return ids
}

// ============================================================
// ChameleonServerConn - 服务端单个降级连接
// ============================================================

// ChameleonServerConn 服务端降级连接
type ChameleonServerConn struct {
	clientID  string
	wsConn    *websocket.Conn
	createdAt time.Time
	sendChan  chan []byte
	closed    int32
}

// Send 发送数据到客户端
func (c *ChameleonServerConn) Send(data []byte) error {
	if atomic.LoadInt32(&c.closed) == 1 {
		return io.ErrClosedPipe
	}
	select {
	case c.sendChan <- data:
		return nil
	default:
		return fmt.Errorf("send buffer full")
	}
}

// Close 关闭连接
func (c *ChameleonServerConn) Close() error {
	if !atomic.CompareAndSwapInt32(&c.closed, 0, 1) {
		return nil
	}
	close(c.sendChan)
	return c.wsConn.Close()
}

// ClientID 获取客户端 ID
func (c *ChameleonServerConn) ClientID() string {
	return c.clientID
}

// RemoteAddr 远端地址
func (c *ChameleonServerConn) RemoteAddr() net.Addr {
	return c.wsConn.RemoteAddr()
}

// CreatedAt 创建时间
func (c *ChameleonServerConn) CreatedAt() time.Time {
	return c.createdAt
}
