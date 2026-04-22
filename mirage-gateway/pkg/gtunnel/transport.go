// Package gtunnel - 统一传输层抽象
// 支持 QUIC (UDP) 主通道 + WebSocket/mTLS (TCP) 降级通道
package gtunnel

import (
	"io"
	"net"
	"sync"
	"time"
)

// TransportType 传输类型
type TransportType uint8

const (
	TransportQUIC      TransportType = 0 // 主通道：QUIC over UDP
	TransportWebSocket TransportType = 1 // 降级通道：WebSocket over mTLS (TCP)
	TransportWebRTC    TransportType = 2 // WebRTC DataChannel (DTLS + SCTP)
	TransportICMP      TransportType = 3 // ICMP Tunnel (eBPF TC Hook)
	TransportDNS       TransportType = 4 // DNS Tunnel (Base32 子域名编码)
)

// TransportConn 统一传输连接接口
type TransportConn interface {
	// Send 发送原始 IP 包
	Send(data []byte) error
	// Recv 接收原始 IP 包
	Recv() ([]byte, error)
	// Close 关闭连接
	Close() error
	// Type 返回传输类型
	Type() TransportType
	// RTT 返回当前 RTT
	RTT() time.Duration
	// RemoteAddr 远端地址
	RemoteAddr() net.Addr
	// MaxDatagramSize 返回该传输协议单次 Send 可承载的最大字节数
	MaxDatagramSize() int
}

// TransportManager 传输管理器 — 管理主通道与降级通道的切换
//
// Deprecated: TransportManager 已被 Orchestrator 替代为唯一编排主链。
// 新代码不应使用 TransportManager，应使用 Orchestrator。
// 保留此类型仅为向后兼容，后续版本将移除。
// 参见 docs/外部零特征消除审计与整改清单.md S-01。
type TransportManager struct {
	mu sync.RWMutex

	// 当前活跃连接
	active TransportConn

	// 降级通道（备用）
	fallback TransportConn

	// 配置
	config TransportConfig

	// 状态
	state TransportState

	// 数据回调：收到 IP 包后交给上层处理
	onPacketRecv func(data []byte)

	// 状态变更回调
	onStateChange func(old, new TransportState)

	// 停止信号
	stopChan chan struct{}
}

// TransportConfig 传输配置
type TransportConfig struct {
	// QUIC 主通道配置
	QUICAddr    string        // QUIC 远端地址 (host:port)
	QUICTimeout time.Duration // QUIC 连接超时（超时后触发降级）

	// WebSocket 降级通道配置
	WSEndpoint string // WebSocket 端点 (wss://host:443/path)
	WSPath     string // HTTP 路径（伪装为正常页面）
	WSSNI      string // TLS SNI（伪装域名）

	// mTLS 证书
	CertFile string
	KeyFile  string
	CAFile   string

	// 降级策略
	FallbackTimeout  time.Duration // 降级超时阈值（默认 3s）
	ProbeInterval    time.Duration // 主通道探测间隔（降级后持续探测）
	PromoteThreshold int           // 连续成功探测次数后回升
}

// TransportState 传输状态
type TransportState uint8

const (
	StateDisconnected TransportState = 0 // 未连接
	StateQUIC         TransportState = 1 // QUIC 主通道活跃
	StateFallback     TransportState = 2 // TCP 降级通道活跃
	StatePromoting    TransportState = 3 // 正在回升到 QUIC
)

// DefaultTransportConfig 默认配置
func DefaultTransportConfig() TransportConfig {
	return TransportConfig{
		QUICTimeout:      3 * time.Second,
		FallbackTimeout:  3 * time.Second,
		ProbeInterval:    30 * time.Second,
		PromoteThreshold: 3,
		WSPath:           "/api/v2/stream",
	}
}

// NewTransportManager 创建传输管理器
func NewTransportManager(config TransportConfig) *TransportManager {
	return &TransportManager{
		config:   config,
		state:    StateDisconnected,
		stopChan: make(chan struct{}),
	}
}

// SetPacketCallback 设置收包回调
func (tm *TransportManager) SetPacketCallback(cb func(data []byte)) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.onPacketRecv = cb
}

// SetStateCallback 设置状态变更回调
func (tm *TransportManager) SetStateCallback(cb func(old, new TransportState)) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.onStateChange = cb
}

// GetState 获取当前状态
func (tm *TransportManager) GetState() TransportState {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.state
}

// GetActiveConn 获取当前活跃连接
func (tm *TransportManager) GetActiveConn() TransportConn {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.active
}

// Send 通过当前活跃通道发送数据
func (tm *TransportManager) Send(data []byte) error {
	tm.mu.RLock()
	conn := tm.active
	tm.mu.RUnlock()

	if conn == nil {
		return io.ErrClosedPipe
	}
	return conn.Send(data)
}

// Close 关闭所有连接
func (tm *TransportManager) Close() error {
	close(tm.stopChan)

	tm.mu.Lock()
	defer tm.mu.Unlock()

	if tm.active != nil {
		tm.active.Close()
	}
	if tm.fallback != nil {
		tm.fallback.Close()
	}
	tm.setState(StateDisconnected)
	return nil
}

// setState 内部状态切换（需持有锁）
func (tm *TransportManager) setState(newState TransportState) {
	old := tm.state
	tm.state = newState
	if tm.onStateChange != nil && old != newState {
		go tm.onStateChange(old, newState)
	}
}
