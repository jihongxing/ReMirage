package gtunnel

import (
	"context"
	"net"
	"time"

	"github.com/quic-go/quic-go"
)

// QUICServerConn 服务端 QUIC Datagram 连接适配器
// 将 *quic.Conn 包装为 TransportConn 接口，用于 Gateway 侧接受入站 QUIC 连接
type QUICServerConn struct {
	conn   *quic.Conn
	ctx    context.Context
	cancel context.CancelFunc
}

// NewQUICServerConn 创建服务端 QUIC 连接适配器
func NewQUICServerConn(conn *quic.Conn) *QUICServerConn {
	ctx, cancel := context.WithCancel(context.Background())
	return &QUICServerConn{
		conn:   conn,
		ctx:    ctx,
		cancel: cancel,
	}
}

func (q *QUICServerConn) Send(data []byte) error {
	return q.conn.SendDatagram(data)
}

func (q *QUICServerConn) Recv() ([]byte, error) {
	return q.conn.ReceiveDatagram(q.ctx)
}

func (q *QUICServerConn) Close() error {
	q.cancel()
	return q.conn.CloseWithError(0, "shutdown")
}

func (q *QUICServerConn) Type() TransportType { return TransportQUIC }

func (q *QUICServerConn) RTT() time.Duration {
	return 0 // RTT 由上层 LinkAuditor 通过 ping 测量
}

func (q *QUICServerConn) RemoteAddr() net.Addr {
	return q.conn.RemoteAddr()
}

func (q *QUICServerConn) MaxDatagramSize() int {
	return 1200 // conservative QUIC initial MTU minus overhead
}
