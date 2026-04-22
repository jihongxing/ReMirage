package gtclient

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"fmt"
	"net"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

// WSSTransportConfig WSS 降级传输配置。
type WSSTransportConfig struct {
	Addr     string // Gateway WSS 地址 (host:port)
	SNI      string // TLS SNI
	WSPath   string // WebSocket 路径（必须与 Gateway ChameleonListener 一致）
	CertFile string // mTLS 客户端证书
	KeyFile  string // mTLS 客户端私钥
	CAFile   string // Gateway CA 证书（验证服务端）
}

// WSSTransport 通过 WebSocket over mTLS 与 Gateway ChameleonListener 通信。
// 帧协议：[2 bytes BigEndian length][payload]，与 ChameleonListener.enframe/deframe 一致。
type WSSTransport struct {
	mu        sync.Mutex
	ws        *websocket.Conn
	closed    int32
	recvCh    chan []byte // 接收缓冲
	done      chan struct{}
	closeOnce sync.Once
}

// NewWSSTransport 拨号建立 WSS 降级连接。
func NewWSSTransport(ctx context.Context, cfg WSSTransportConfig) (Transport, error) {
	tlsCfg := &tls.Config{
		ServerName: cfg.SNI,
		MinVersion: tls.VersionTLS13,
		NextProtos: []string{"http/1.1"},
	}

	// mTLS 客户端证书
	if cfg.CertFile != "" && cfg.KeyFile != "" {
		cert, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("wss: load client cert: %w", err)
		}
		tlsCfg.Certificates = []tls.Certificate{cert}
	}

	// 服务端 CA
	if cfg.CAFile != "" {
		caPEM, err := os.ReadFile(cfg.CAFile)
		if err != nil {
			return nil, fmt.Errorf("wss: read CA: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(caPEM) {
			return nil, fmt.Errorf("wss: invalid CA cert")
		}
		tlsCfg.RootCAs = pool
	}

	dialer := websocket.Dialer{
		TLSClientConfig:  tlsCfg,
		HandshakeTimeout: 10 * time.Second,
		NetDialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, addr)
		},
	}

	wsURL := fmt.Sprintf("wss://%s%s", cfg.Addr, cfg.WSPath)
	header := http.Header{}
	header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	ws, _, err := dialer.DialContext(ctx, wsURL, header)
	if err != nil {
		return nil, fmt.Errorf("wss: dial %s: %w", wsURL, err)
	}

	t := &WSSTransport{
		ws:     ws,
		recvCh: make(chan []byte, 256),
		done:   make(chan struct{}),
	}
	go t.readLoop()
	return t, nil
}

// SendDatagram 封帧并发送。
func (t *WSSTransport) SendDatagram(data []byte) error {
	if atomic.LoadInt32(&t.closed) == 1 {
		return net.ErrClosed
	}
	frame := make([]byte, 2+len(data))
	binary.BigEndian.PutUint16(frame[0:2], uint16(len(data)))
	copy(frame[2:], data)

	t.mu.Lock()
	defer t.mu.Unlock()
	t.ws.SetWriteDeadline(time.Now().Add(10 * time.Second))
	return t.ws.WriteMessage(websocket.BinaryMessage, frame)
}

// ReceiveDatagram 从接收缓冲读取一个解帧后的包。
func (t *WSSTransport) ReceiveDatagram(ctx context.Context) ([]byte, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case pkt, ok := <-t.recvCh:
		if !ok {
			return nil, net.ErrClosed
		}
		return pkt, nil
	case <-t.done:
		return nil, net.ErrClosed
	}
}

// IsConnected 返回连接是否存活。
func (t *WSSTransport) IsConnected() bool {
	return atomic.LoadInt32(&t.closed) == 0
}

// Close 关闭 WebSocket 连接。
// 通过 done channel 通知 readLoop 退出，readLoop 负责关闭 recvCh。
func (t *WSSTransport) Close() error {
	var wsErr error
	t.closeOnce.Do(func() {
		atomic.StoreInt32(&t.closed, 1)
		close(t.done)
		wsErr = t.ws.Close()
	})
	return wsErr
}

// readLoop 从 WebSocket 读取消息并解帧。
// readLoop 是 recvCh 的唯一写入方，退出时关闭 recvCh。
func (t *WSSTransport) readLoop() {
	defer close(t.recvCh)

	for {
		select {
		case <-t.done:
			return
		default:
		}

		t.ws.SetReadDeadline(time.Now().Add(60 * time.Second))
		_, message, err := t.ws.ReadMessage()
		if err != nil {
			return
		}

		// 解帧：2 字节长度头 + payload，支持多包合并
		offset := 0
		for offset < len(message) {
			if offset+2 > len(message) {
				return // 帧头不完整
			}
			length := int(binary.BigEndian.Uint16(message[offset : offset+2]))
			offset += 2
			if offset+length > len(message) {
				return // 帧数据不完整
			}
			pkt := make([]byte, length)
			copy(pkt, message[offset:offset+length])
			offset += length

			select {
			case t.recvCh <- pkt:
			case <-t.done:
				return
			}
		}
	}
}
