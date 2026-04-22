package gtunnel

import (
	"net"
	"sync"
	"time"
)

// ChameleonServerConnAdapter 将 ChameleonServerConn 适配为 TransportConn 接口。
// 用于 Gateway 服务端模式：ChameleonListener 接受入站 WSS 连接后，
// 通过此适配器注入 Orchestrator 作为受管路径。
type ChameleonServerConnAdapter struct {
	inner     *ChameleonServerConn
	clientID  string
	recvCh    chan []byte
	closed    chan struct{}
	closeOnce sync.Once
}

// NewChameleonServerConnAdapter 创建适配器。
func NewChameleonServerConnAdapter(conn *ChameleonServerConn, clientID string) *ChameleonServerConnAdapter {
	return &ChameleonServerConnAdapter{
		inner:    conn,
		clientID: clientID,
		recvCh:   make(chan []byte, 256),
		closed:   make(chan struct{}),
	}
}

// Send 发送数据到客户端。
func (a *ChameleonServerConnAdapter) Send(data []byte) error {
	return a.inner.Send(data)
}

// Recv 从客户端接收数据。
// 当适配器被 Close 后，立即返回 net.ErrClosed（不会永久阻塞）。
func (a *ChameleonServerConnAdapter) Recv() ([]byte, error) {
	select {
	case data, ok := <-a.recvCh:
		if !ok {
			return nil, net.ErrClosed
		}
		return data, nil
	case <-a.closed:
		return nil, net.ErrClosed
	}
}

// FeedPacket 由 ChameleonListener 的 readLoop 调用，将收到的包喂入适配器。
// 如果适配器已关闭，静默丢弃。
func (a *ChameleonServerConnAdapter) FeedPacket(data []byte) {
	select {
	case <-a.closed:
		return // 已关闭，丢弃
	default:
	}
	select {
	case a.recvCh <- data:
	default:
		// channel 满，丢弃最旧的包
		select {
		case <-a.recvCh:
		default:
		}
		a.recvCh <- data
	}
}

// Close 关闭连接和 recvCh，确保 Recv() 不会永久阻塞。
func (a *ChameleonServerConnAdapter) Close() error {
	a.closeOnce.Do(func() {
		close(a.closed)
	})
	return a.inner.Close()
}

// Type 返回传输类型。
func (a *ChameleonServerConnAdapter) Type() TransportType {
	return TransportWebSocket
}

// RTT 返回 RTT（WSS 连接无精确 RTT，返回 0）。
func (a *ChameleonServerConnAdapter) RTT() time.Duration {
	return 0
}

// RemoteAddr 返回远端地址。
func (a *ChameleonServerConnAdapter) RemoteAddr() net.Addr {
	return a.inner.RemoteAddr()
}

// MaxDatagramSize 返回最大数据报大小。
func (a *ChameleonServerConnAdapter) MaxDatagramSize() int {
	return 65535
}

// ClientID 返回客户端标识。
func (a *ChameleonServerConnAdapter) ClientID() string {
	return a.clientID
}
