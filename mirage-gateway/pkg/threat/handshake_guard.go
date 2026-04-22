package threat

import (
	"log"
	"net"
	"sync"
	"time"
)

// hsCounter 单个 IP 的握手超时计数
type hsCounter struct {
	Count       int
	WindowStart time.Time
}

// HandshakeGuard 半开连接熔断器
// 为每个入站连接注入握手超时，超时后静默 RST 并累加风险评分
type HandshakeGuard struct {
	timeout    time.Duration
	mu         sync.Mutex
	ipCounters map[string]*hsCounter
	blacklist  *BlacklistManager
	riskScorer RiskScoreAdder
}

// NewHandshakeGuard 创建握手熔断器（默认 timeout=300ms）
func NewHandshakeGuard(timeout time.Duration, bl *BlacklistManager, rs RiskScoreAdder) *HandshakeGuard {
	return &HandshakeGuard{
		timeout:    timeout,
		ipCounters: make(map[string]*hsCounter),
		blacklist:  bl,
		riskScorer: rs,
	}
}

// WrapListener 包装 net.Listener，为每个连接注入握手超时
func (hg *HandshakeGuard) WrapListener(ln net.Listener) net.Listener {
	return &guardedListener{
		Listener: ln,
		guard:    hg,
	}
}

// onTimeout 握手超时回调
// 递增 IP 超时计数，同一 IP 1 分钟内 > 5 次 → 黑名单 1h + RiskScorer +20
func (hg *HandshakeGuard) onTimeout(sourceIP string) {
	HandshakeTimeoutTotal.WithLabelValues(GetGatewayID()).Inc()

	hg.mu.Lock()
	defer hg.mu.Unlock()

	now := time.Now()
	c, ok := hg.ipCounters[sourceIP]
	if !ok {
		c = &hsCounter{WindowStart: now}
		hg.ipCounters[sourceIP] = c
	}

	// 窗口过期（>1min）则重置
	if now.Sub(c.WindowStart) > 1*time.Minute {
		c.Count = 0
		c.WindowStart = now
	}

	c.Count++

	if c.Count > 5 {
		if hg.blacklist != nil {
			_ = hg.blacklist.Add(sourceIP+"/32", now.Add(1*time.Hour), SourceLocal)
		}
		if hg.riskScorer != nil {
			hg.riskScorer.AddScore(sourceIP, 20, "handshake_timeout")
		}
		log.Printf("[HandshakeGuard] IP %s 超时 %d 次，已加入黑名单", sourceIP, c.Count)
	}
}

// ---------------------------------------------------------------------------
// guardedListener 实现 net.Listener 接口
// ---------------------------------------------------------------------------

type guardedListener struct {
	net.Listener
	guard *HandshakeGuard
}

func (gl *guardedListener) Accept() (net.Conn, error) {
	conn, err := gl.Listener.Accept()
	if err != nil {
		return nil, err
	}

	// 提取远端 IP
	host, _, err := net.SplitHostPort(conn.RemoteAddr().String())
	if err != nil {
		host = conn.RemoteAddr().String()
	}

	// 注入握手超时
	_ = conn.SetDeadline(time.Now().Add(gl.guard.timeout))

	return &guardedConn{
		Conn:     conn,
		guard:    gl.guard,
		sourceIP: host,
	}, nil
}

// ---------------------------------------------------------------------------
// guardedConn 包装 net.Conn，增加超时检测与 ClearDeadline
// ---------------------------------------------------------------------------

type guardedConn struct {
	net.Conn
	guard    *HandshakeGuard
	sourceIP string
	timedOut bool
}

// ClearDeadline 握手成功后调用，清除 deadline
func (gc *guardedConn) ClearDeadline() {
	_ = gc.Conn.SetDeadline(time.Time{})
}

// Read 覆写 Read，检测超时错误并设置 timedOut 标志
func (gc *guardedConn) Read(b []byte) (int, error) {
	n, err := gc.Conn.Read(b)
	if err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			gc.timedOut = true
		}
	}
	return n, err
}

// Close 覆写 Close，超时时触发 onTimeout 回调
func (gc *guardedConn) Close() error {
	if gc.timedOut {
		gc.guard.onTimeout(gc.sourceIP)
	}
	return gc.Conn.Close()
}
