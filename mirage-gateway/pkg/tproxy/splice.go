// Package tproxy - Splice 零拷贝转发（兜底方案）
package tproxy

import (
	"io"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

// SpliceBridge Splice 桥接器
type SpliceBridge struct {
	clientConn net.Conn
	serverConn net.Conn

	clientFD int
	serverFD int

	pipeR *os.File
	pipeW *os.File

	bytesTx uint64
	bytesRx uint64

	done chan struct{}
}

// Stop 停止转发
func (sb *SpliceBridge) Stop() {
	sb.clientConn.Close()
	sb.serverConn.Close()
	if sb.pipeR != nil {
		sb.pipeR.Close()
	}
	if sb.pipeW != nil {
		sb.pipeW.Close()
	}
	<-sb.done
}

// GetStats 获取统计
func (sb *SpliceBridge) GetStats() (tx, rx uint64) {
	return atomic.LoadUint64(&sb.bytesTx), atomic.LoadUint64(&sb.bytesRx)
}

// BridgeWorker 桥接工作器（自动选择最优方案）
type BridgeWorker struct {
	sockMap   *interface{}
	useSplice bool
}

// NewBridgeWorker 创建桥接工作器
func NewBridgeWorker(sockMap interface{}) *BridgeWorker {
	return &BridgeWorker{
		sockMap:   &sockMap,
		useSplice: sockMap == nil,
	}
}

// Bridge 建立桥接
func (bw *BridgeWorker) Bridge(clientConn, serverConn net.Conn) error {
	if bw.sockMap != nil && *bw.sockMap != nil {
		return nil
	}

	if bw.useSplice {
		sb, err := NewSpliceBridge(clientConn, serverConn)
		if err == nil {
			sb.Start()
			return nil
		}
	}

	return bw.ioCopyRelay(clientConn, serverConn)
}

// ioCopyRelay io.Copy 转发（最后兜底）
func (bw *BridgeWorker) ioCopyRelay(clientConn, serverConn net.Conn) error {
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		io.Copy(serverConn, clientConn)
		if tc, ok := serverConn.(*net.TCPConn); ok {
			tc.CloseWrite()
		}
	}()

	go func() {
		defer wg.Done()
		io.Copy(clientConn, serverConn)
		if tc, ok := clientConn.(*net.TCPConn); ok {
			tc.CloseWrite()
		}
	}()

	wg.Wait()
	return nil
}

// UDPSpliceBridge UDP 桥接
type UDPSpliceBridge struct {
	clientConn *net.UDPConn
	serverConn *net.UDPConn
	serverAddr *net.UDPAddr

	bytesTx uint64
	bytesRx uint64

	done chan struct{}
}

// NewUDPSpliceBridge 创建 UDP 桥接
func NewUDPSpliceBridge(clientConn *net.UDPConn, serverAddr *net.UDPAddr) (*UDPSpliceBridge, error) {
	serverConn, err := net.DialUDP("udp", nil, serverAddr)
	if err != nil {
		return nil, err
	}

	return &UDPSpliceBridge{
		clientConn: clientConn,
		serverConn: serverConn,
		serverAddr: serverAddr,
		done:       make(chan struct{}),
	}, nil
}

// Start 启动 UDP 转发
func (ub *UDPSpliceBridge) Start() {
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		ub.udpRelay(ub.clientConn, ub.serverConn, &ub.bytesTx)
	}()

	go func() {
		defer wg.Done()
		ub.udpRelayBack(ub.serverConn, ub.clientConn, &ub.bytesRx)
	}()

	wg.Wait()
	close(ub.done)
}

func (ub *UDPSpliceBridge) udpRelay(src, dst *net.UDPConn, counter *uint64) {
	buf := make([]byte, 65535)
	for {
		select {
		case <-ub.done:
			return
		default:
		}

		src.SetReadDeadline(time.Now().Add(30 * time.Second))
		n, _, err := src.ReadFromUDP(buf)
		if err != nil {
			return
		}

		dst.SetWriteDeadline(time.Now().Add(30 * time.Second))
		_, err = dst.Write(buf[:n])
		if err != nil {
			return
		}

		atomic.AddUint64(counter, uint64(n))
	}
}

func (ub *UDPSpliceBridge) udpRelayBack(src, dst *net.UDPConn, counter *uint64) {
	buf := make([]byte, 65535)
	for {
		select {
		case <-ub.done:
			return
		default:
		}

		src.SetReadDeadline(time.Now().Add(30 * time.Second))
		n, err := src.Read(buf)
		if err != nil {
			return
		}

		dst.SetWriteDeadline(time.Now().Add(30 * time.Second))
		_, err = dst.Write(buf[:n])
		if err != nil {
			return
		}

		atomic.AddUint64(counter, uint64(n))
	}
}

// Stop 停止 UDP 转发
func (ub *UDPSpliceBridge) Stop() {
	ub.clientConn.Close()
	ub.serverConn.Close()
	<-ub.done
}
