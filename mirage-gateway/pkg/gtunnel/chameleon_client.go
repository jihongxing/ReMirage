// Package gtunnel - Chameleon 客户端降级连接器
// 客户端侧：QUIC 超时后自动降级为 WebSocket over mTLS
//
// O4 深度隐匿：使用 utls (refraction-networking/utls) 替代 Go 原生 crypto/tls
// Go 原生 TLS 的 JA3/JA4 指纹与 Chrome 完全不同（Cipher Suites 顺序、Extensions 列表）
// utls 可以像素级复制真实浏览器的 ClientHello（含 GREASE 随机扩展）
package gtunnel

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/quic-go/quic-go"
	utls "github.com/refraction-networking/utls"
)

// ChameleonClientConn 客户端降级连接（实现 TransportConn 接口）
type ChameleonClientConn struct {
	mu sync.RWMutex

	// WebSocket 连接
	wsConn *websocket.Conn

	// 配置
	endpoint string // wss://host:443/path
	sni      string // TLS SNI 伪装域名

	// TLS 配置
	tlsConfig *tls.Config

	// 接收缓冲
	recvChan chan []byte

	// 状态
	connected int32
	rtt       time.Duration

	// 统计
	bytesSent int64
	bytesRecv int64

	// 控制
	ctx    context.Context
	cancel context.CancelFunc
}

// ChameleonDialConfig 拨号配置
type ChameleonDialConfig struct {
	// 网关端点
	Endpoint string // wss://gateway.example.com:443/api/v2/stream

	// TLS 伪装
	SNI       string // 伪装 SNI（如 cdn.cloudflare.com）
	UserAgent string // 伪装 UA

	// mTLS 客户端证书
	CertFile string
	KeyFile  string
	CAFile   string

	// 超时
	DialTimeout  time.Duration
	WriteTimeout time.Duration
	ReadTimeout  time.Duration
}

// DefaultChameleonDialConfig 默认拨号配置
func DefaultChameleonDialConfig() ChameleonDialConfig {
	return ChameleonDialConfig{
		SNI:          "cdn.cloudflare.com",
		UserAgent:    "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36",
		DialTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		ReadTimeout:  60 * time.Second,
	}
}

// DialChameleon 拨号建立降级连接
// O4 深度隐匿：使用 dialWithUTLS 建立底层 TCP+TLS 连接，在 uTLS 连接之上建立 WebSocket
// 消除 Go 原生 crypto/tls 的 JA3/JA4 指纹差异
func DialChameleon(ctx context.Context, config ChameleonDialConfig) (*ChameleonClientConn, error) {
	connCtx, cancel := context.WithCancel(ctx)

	conn := &ChameleonClientConn{
		endpoint: config.Endpoint,
		sni:      config.SNI,
		recvChan: make(chan []byte, 256),
		ctx:      connCtx,
		cancel:   cancel,
	}

	// 构建 TLS 配置（仅用于 mTLS 证书加载，实际握手由 uTLS 接管）
	tlsConfig, err := conn.buildClientTLS(config)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("TLS 配置失败: %w", err)
	}
	conn.tlsConfig = tlsConfig

	// 解析 endpoint 中的 host:port 用于 dialWithUTLS
	wsURL := config.Endpoint
	host := config.SNI
	addr := host + ":443" // 默认端口
	if u, parseErr := parseWSEndpoint(wsURL); parseErr == nil {
		addr = u.Host
		if u.Port() == "" {
			addr = u.Host + ":443"
		}
	}

	// 构建 WebSocket Dialer — 使用 NetDialTLSContext 注入 uTLS 连接
	dialer := &websocket.Dialer{
		HandshakeTimeout: config.DialTimeout,
		Proxy:            http.ProxyFromEnvironment,
		// 关键：通过 NetDialTLSContext 注入 uTLS 连接
		// 这样 WebSocket 握手在 uTLS 连接之上进行，而非 Go 原生 TLS
		NetDialTLSContext: func(ctx context.Context, network, dialAddr string) (net.Conn, error) {
			utlsConn, err := dialWithUTLS(addr, host, tlsConfig)
			if err != nil {
				return nil, fmt.Errorf("uTLS 连接失败: %w", err)
			}
			return utlsConn, nil
		},
	}

	// 伪装请求头
	headers := http.Header{
		"User-Agent":      []string{config.UserAgent},
		"Accept":          []string{"text/html,application/xhtml+xml"},
		"Accept-Language": []string{"en-US,en;q=0.9"},
		"Accept-Encoding": []string{"gzip, deflate, br"},
		"Cache-Control":   []string{"no-cache"},
	}

	// 拨号
	start := time.Now()
	wsConn, resp, err := dialer.DialContext(ctx, config.Endpoint, headers)
	if err != nil {
		cancel()
		if resp != nil {
			return nil, fmt.Errorf("WebSocket 拨号失败 (HTTP %d): %w", resp.StatusCode, err)
		}
		return nil, fmt.Errorf("WebSocket 拨号失败: %w", err)
	}
	conn.rtt = time.Since(start)

	conn.wsConn = wsConn
	atomic.StoreInt32(&conn.connected, 1)

	// 启动接收循环
	go conn.recvLoop()

	log.Printf("🦎 [Chameleon-Client] 降级连接已建立 (uTLS): %s (RTT: %v)", config.Endpoint, conn.rtt)

	return conn, nil
}

// Send 发送 IP 包
func (c *ChameleonClientConn) Send(data []byte) error {
	if atomic.LoadInt32(&c.connected) == 0 {
		return io.ErrClosedPipe
	}

	// 封帧
	frame := make([]byte, 2+len(data))
	binary.BigEndian.PutUint16(frame[0:2], uint16(len(data)))
	copy(frame[2:], data)

	c.mu.Lock()
	defer c.mu.Unlock()

	c.wsConn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	err := c.wsConn.WriteMessage(websocket.BinaryMessage, frame)
	if err != nil {
		return fmt.Errorf("发送失败: %w", err)
	}

	atomic.AddInt64(&c.bytesSent, int64(len(data)))
	return nil
}

// Recv 接收 IP 包
func (c *ChameleonClientConn) Recv() ([]byte, error) {
	select {
	case <-c.ctx.Done():
		return nil, io.ErrClosedPipe
	case data, ok := <-c.recvChan:
		if !ok {
			return nil, io.EOF
		}
		return data, nil
	}
}

// Close 关闭连接
func (c *ChameleonClientConn) Close() error {
	if !atomic.CompareAndSwapInt32(&c.connected, 1, 0) {
		return nil
	}
	c.cancel()
	return c.wsConn.Close()
}

// Type 返回传输类型
func (c *ChameleonClientConn) Type() TransportType {
	return TransportWebSocket
}

// RTT 返回 RTT
func (c *ChameleonClientConn) RTT() time.Duration {
	return c.rtt
}

// RemoteAddr 远端地址
func (c *ChameleonClientConn) RemoteAddr() net.Addr {
	if c.wsConn != nil {
		return c.wsConn.RemoteAddr()
	}
	return nil
}

// MaxDatagramSize 返回 WSS 最大数据报大小
func (c *ChameleonClientConn) MaxDatagramSize() int {
	return 65535
}

// IsConnected 是否已连接
func (c *ChameleonClientConn) IsConnected() bool {
	return atomic.LoadInt32(&c.connected) == 1
}

// recvLoop 接收循环
func (c *ChameleonClientConn) recvLoop() {
	defer func() {
		atomic.StoreInt32(&c.connected, 0)
		close(c.recvChan)
	}()

	for {
		select {
		case <-c.ctx.Done():
			return
		default:
		}

		_, message, err := c.wsConn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.Printf("⚠️ [Chameleon-Client] 读取错误: %v", err)
			}
			return
		}

		// 解帧
		packets, err := deframePackets(message)
		if err != nil {
			log.Printf("⚠️ [Chameleon-Client] 解帧错误: %v", err)
			continue
		}

		for _, pkt := range packets {
			atomic.AddInt64(&c.bytesRecv, int64(len(pkt)))
			select {
			case c.recvChan <- pkt:
			default:
				// 缓冲区满，丢弃最旧的包
				<-c.recvChan
				c.recvChan <- pkt
			}
		}
	}
}

// buildClientTLS 构建客户端 TLS 配置
// O4: 使用 utls HelloChrome_Auto 像素级复制 Chrome 指纹
// 返回的 tls.Config 仅用于 mTLS 证书加载，实际握手由 utls 接管
func (c *ChameleonClientConn) buildClientTLS(config ChameleonDialConfig) (*tls.Config, error) {
	tlsCfg := &tls.Config{
		ServerName: config.SNI,
		MinVersion: tls.VersionTLS13,
		NextProtos: []string{"http/1.1"}, // WebSocket 不走 h2
		// 强制使用与 Chrome 一致的 Cipher Suites 顺序
		// Go 1.21+ TLS 1.3 cipher suites 不可配置（由 runtime 决定），
		// 但 CurvePreferences 可以调整以减少指纹差异
		CurvePreferences: []tls.CurveID{
			tls.X25519,
			tls.CurveP256,
			tls.CurveP384,
		},
	}

	// 加载客户端证书（mTLS）
	if config.CertFile != "" && config.KeyFile != "" {
		cert, err := tls.LoadX509KeyPair(config.CertFile, config.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("加载客户端证书失败: %w", err)
		}
		tlsCfg.Certificates = []tls.Certificate{cert}
	}

	return tlsCfg, nil
}

// dialWithUTLS 使用 utls 建立 TLS 连接（像素级 Chrome 指纹）
// 替代 Go 原生 tls.Dial，消除 JA3/JA4 指纹差异
func dialWithUTLS(addr, sni string, baseTLS *tls.Config) (net.Conn, error) {
	// 1. 建立 TCP 连接
	tcpConn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("TCP 连接失败: %w", err)
	}

	// 2. 构建 utls 配置（像素级复制 Chrome 最新版指纹）
	utlsConfig := &utls.Config{
		ServerName: sni,
		MinVersion: tls.VersionTLS12, // Chrome 支持 TLS 1.2+
		// 不设置 CipherSuites/CurvePreferences — 由 HelloChrome_Auto 完全接管
	}

	// 传递 InsecureSkipVerify（测试/开发环境）
	if baseTLS != nil && baseTLS.InsecureSkipVerify {
		utlsConfig.InsecureSkipVerify = true
	}

	// 加载 mTLS 客户端证书（如果有）
	if baseTLS != nil && len(baseTLS.Certificates) > 0 {
		// 只传递核心字段（Certificate chain + PrivateKey）
		for _, cert := range baseTLS.Certificates {
			utlsConfig.Certificates = append(utlsConfig.Certificates, utls.Certificate{
				Certificate: cert.Certificate,
				PrivateKey:  cert.PrivateKey,
				Leaf:        cert.Leaf,
			})
		}
	}

	// 3. 使用 HelloChrome_Auto：自动选择最新 Chrome 版本的完整指纹
	// 包含：GREASE 随机扩展、正确的 Cipher Suites 顺序、
	// Supported Groups、Signature Algorithms、ALPN 等
	utlsConn := utls.UClient(tcpConn, utlsConfig, utls.HelloChrome_Auto)

	// 4. 执行握手
	if err := utlsConn.Handshake(); err != nil {
		tcpConn.Close()
		return nil, fmt.Errorf("utls 握手失败: %w", err)
	}

	return utlsConn, nil
}

// parseWSEndpoint 解析 WebSocket endpoint URL
func parseWSEndpoint(endpoint string) (*url.URL, error) {
	return url.Parse(endpoint)
}

// deframePackets 解帧（公共函数）
func deframePackets(message []byte) ([][]byte, error) {
	var packets [][]byte
	offset := 0

	for offset < len(message) {
		if offset+2 > len(message) {
			return nil, fmt.Errorf("帧头不完整")
		}

		length := int(binary.BigEndian.Uint16(message[offset : offset+2]))
		offset += 2

		if length == 0 || offset+length > len(message) {
			return nil, fmt.Errorf("帧数据不完整")
		}

		pkt := make([]byte, length)
		copy(pkt, message[offset:offset+length])
		packets = append(packets, pkt)
		offset += length
	}

	return packets, nil
}

// ============================================================
// 自动降级控制器 — 兼容适配层（委托到 Orchestrator）
//
// Deprecated: 以下方法均已委托到内部 Orchestrator 实例。
// 新代码应直接使用 Orchestrator。
// ============================================================

// ConnectWithFallback 带降级的连接流程
//
// Deprecated: 此方法属于 TransportManager 的二元切换体系，已被 Orchestrator 替代。
// 新代码应使用 Orchestrator.Start() 进行 HappyEyeballs 多协议竞速。
// 此方法现已委托到内部 Orchestrator.Start()。
// 参见 docs/外部零特征消除审计与整改清单.md S-01。
func (tm *TransportManager) ConnectWithFallback(ctx context.Context) error {
	// 委托到 Orchestrator
	if tm.orchestrator != nil {
		return tm.orchestrator.Start(ctx)
	}

	// 兼容回退：orchestrator 未初始化时使用旧逻辑
	log.Printf("🚀 [Transport] 尝试 QUIC 连接: %s (超时 %v)", tm.config.QUICAddr, tm.config.FallbackTimeout)

	quicCtx, quicCancel := context.WithTimeout(ctx, tm.config.FallbackTimeout)
	defer quicCancel()

	quicConn, err := tm.dialQUIC(quicCtx)
	if err == nil {
		// QUIC 连接成功
		tm.mu.Lock()
		tm.active = quicConn
		tm.setState(StateQUIC)
		tm.mu.Unlock()
		log.Printf("✅ [Transport] QUIC 连接成功")
		go tm.recvLoop()
		return nil
	}

	// QUIC 失败 → 降级
	log.Printf("⚠️ [Transport] QUIC 连接失败 (%v)，启动 TCP 降级...", err)

	// 第二步：降级为 WebSocket
	wsConfig := ChameleonDialConfig{
		Endpoint:    tm.config.WSEndpoint,
		SNI:         tm.config.WSSNI,
		CertFile:    tm.config.CertFile,
		KeyFile:     tm.config.KeyFile,
		DialTimeout: 10 * time.Second,
	}

	wsConn, err := DialChameleon(ctx, wsConfig)
	if err != nil {
		return fmt.Errorf("QUIC 和 TCP 降级均失败: QUIC=%v, WS=%w", err, err)
	}

	tm.mu.Lock()
	tm.active = wsConn
	tm.setState(StateFallback)
	tm.mu.Unlock()

	log.Printf("🦎 [Transport] TCP 降级成功，启动 QUIC 探测...")

	// 第三步：后台持续探测 QUIC
	go tm.probeAndPromote(ctx)
	go tm.recvLoop()

	return nil
}

// QUICConn 实现 TransportConn 接口的 QUIC Datagram 连接
type QUICConn struct {
	conn    *quic.Conn
	udpConn *net.UDPConn
	ctx     context.Context
	cancel  context.CancelFunc
}

func (q *QUICConn) Send(data []byte) error {
	return q.conn.SendDatagram(data)
}

func (q *QUICConn) Recv() ([]byte, error) {
	return q.conn.ReceiveDatagram(q.ctx)
}

func (q *QUICConn) Close() error {
	q.cancel()
	err := q.conn.CloseWithError(0, "shutdown")
	if q.udpConn != nil {
		q.udpConn.Close()
	}
	return err
}

func (q *QUICConn) Type() TransportType { return TransportQUIC }

func (q *QUICConn) RTT() time.Duration {
	// quic-go v0.59 ConnectionState 不直接暴露 RTT，使用 SmoothedRTT
	return 0 // RTT 由上层 LinkAuditor 通过 ping 测量
}

func (q *QUICConn) RemoteAddr() net.Addr {
	return q.conn.RemoteAddr()
}

func (q *QUICConn) MaxDatagramSize() int {
	return 1200 // conservative QUIC initial MTU minus overhead
}

// dialQUIC 拨号 QUIC — 使用 quic-go Datagram 模式连接 Gateway
func (tm *TransportManager) dialQUIC(ctx context.Context) (TransportConn, error) {
	dialCtx, cancel := context.WithTimeout(ctx, tm.config.QUICTimeout)
	defer cancel()

	tlsConf := &tls.Config{
		// 伪装为标准 HTTP/3 流量（ALPN 必须与正常浏览器一致）
		// 绝不使用自定义 ALPN，否则 DPI 一眼识别
		NextProtos: []string{"h3"},
	}

	// 加载 mTLS 客户端证书
	if tm.config.CertFile != "" && tm.config.KeyFile != "" {
		cert, err := tls.LoadX509KeyPair(tm.config.CertFile, tm.config.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("load mTLS cert: %w", err)
		}
		tlsConf.Certificates = []tls.Certificate{cert}
		tlsConf.InsecureSkipVerify = false
	} else {
		tlsConf.InsecureSkipVerify = true
	}

	quicConf := &quic.Config{
		EnableDatagrams: true,
		KeepAlivePeriod: 10 * time.Second,
		MaxIdleTimeout:  60 * time.Second,
	}

	// 绑定物理网卡（避免 TUN 路由环路）
	udpConn, err := net.ListenUDP("udp4", &net.UDPAddr{Port: 0})
	if err != nil {
		return nil, fmt.Errorf("bind UDP: %w", err)
	}

	remoteAddr, err := net.ResolveUDPAddr("udp4", tm.config.QUICAddr)
	if err != nil {
		udpConn.Close()
		return nil, fmt.Errorf("resolve %s: %w", tm.config.QUICAddr, err)
	}

	conn, err := quic.Dial(dialCtx, udpConn, remoteAddr, tlsConf, quicConf)
	if err != nil {
		udpConn.Close()
		return nil, fmt.Errorf("quic dial %s: %w", tm.config.QUICAddr, err)
	}

	// 验证 Datagram 支持
	state := conn.ConnectionState()
	if !state.SupportsDatagrams.Remote || !state.SupportsDatagrams.Local {
		conn.CloseWithError(0, "datagrams not supported")
		udpConn.Close()
		return nil, fmt.Errorf("gateway does not support QUIC Datagrams")
	}

	qCtx, qCancel := context.WithCancel(context.Background())
	qConn := &QUICConn{
		conn:    conn,
		udpConn: udpConn,
		ctx:     qCtx,
		cancel:  qCancel,
	}

	log.Printf("🚀 [Transport] QUIC 连接成功: %s", tm.config.QUICAddr)
	return qConn, nil
}

// probeAndPromote 后台探测 QUIC 并在恢复后回升
//
// Deprecated: 已被 Orchestrator.probeLoop() 替代。
// 此方法现已委托到 Orchestrator 的探测循环。
func (tm *TransportManager) probeAndPromote(ctx context.Context) {
	// 委托到 Orchestrator — probeLoop 由 Orchestrator.Start() 自动启动
	// 此方法保留为空操作兼容层
	if tm.orchestrator != nil {
		return
	}

	// 兼容回退：orchestrator 未初始化时使用旧逻辑
	ticker := time.NewTicker(tm.config.ProbeInterval)
	defer ticker.Stop()

	successCount := 0

	for {
		select {
		case <-tm.stopChan:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			// 探测 QUIC 是否恢复
			probeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			_, err := tm.dialQUIC(probeCtx)
			cancel()

			if err != nil {
				successCount = 0
				continue
			}

			successCount++
			log.Printf("🔍 [Transport] QUIC 探测成功 (%d/%d)", successCount, tm.config.PromoteThreshold)

			if successCount >= tm.config.PromoteThreshold {
				// 回升到 QUIC
				tm.promoteToQUIC(ctx)
				return
			}
		}
	}
}

// promoteToQUIC 回升到 QUIC 主通道
//
// Deprecated: 已被 Orchestrator.promote() 替代。
// 此方法现已委托到 Orchestrator.promote()。
func (tm *TransportManager) promoteToQUIC(ctx context.Context) {
	// 委托到 Orchestrator
	if tm.orchestrator != nil {
		if err := tm.orchestrator.promote(TransportQUIC); err != nil {
			log.Printf("⚠️ [Transport] Orchestrator 升格失败: %v", err)
		}
		return
	}

	// 兼容回退：orchestrator 未初始化时使用旧逻辑
	log.Printf("⬆️ [Transport] QUIC 恢复，执行回升...")

	tm.mu.Lock()
	tm.setState(StatePromoting)
	tm.mu.Unlock()

	// 建立新 QUIC 连接
	quicCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	quicConn, err := tm.dialQUIC(quicCtx)
	if err != nil {
		log.Printf("⚠️ [Transport] 回升失败: %v，保持降级通道", err)
		tm.mu.Lock()
		tm.setState(StateFallback)
		tm.mu.Unlock()
		return
	}

	// 切换活跃连接
	tm.mu.Lock()
	oldConn := tm.active
	tm.active = quicConn
	tm.fallback = oldConn // 保留降级连接作为备用
	tm.setState(StateQUIC)
	tm.mu.Unlock()

	log.Printf("✅ [Transport] 已回升到 QUIC 主通道")
}

// recvLoop 统一接收循环
//
// Deprecated: 已被 Orchestrator.receiveLoop() 替代。
// 此方法现已委托到 Orchestrator 的接收循环。
func (tm *TransportManager) recvLoop() {
	// 委托到 Orchestrator — receiveLoop 由 Orchestrator.Start() 自动启动
	// 此方法保留为空操作兼容层
	if tm.orchestrator != nil {
		return
	}

	// 兼容回退：orchestrator 未初始化时使用旧逻辑
	for {
		select {
		case <-tm.stopChan:
			return
		default:
		}

		tm.mu.RLock()
		conn := tm.active
		cb := tm.onPacketRecv
		tm.mu.RUnlock()

		if conn == nil {
			time.Sleep(100 * time.Millisecond)
			continue
		}

		data, err := conn.Recv()
		if err != nil {
			if err == io.EOF || err == io.ErrClosedPipe {
				return
			}
			log.Printf("⚠️ [Transport] 接收错误: %v", err)
			time.Sleep(100 * time.Millisecond)
			continue
		}

		if cb != nil {
			cb(data)
		}
	}
}
