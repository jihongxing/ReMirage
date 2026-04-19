// Package tproxy - TPROXY 透明代理桥接器
// 实现零侵入流量拦截与 Sockmap 注入
package tproxy

import (
	"context"
	"log"
	"net"
	"sync"
	"time"

	"github.com/cilium/ebpf"
)

// ProxyPair 代理对（C 结构体对应）
type ProxyPair struct {
	PeerIdx uint32
	Flags   uint32
	BytesTx uint64
	BytesRx uint64
}

// ConnState 连接状态
type ConnState struct {
	State       uint32
	Established uint64
	LastActive  uint64
}

// TPROXYBridge TPROXY 桥接器
type TPROXYBridge struct {
	listenAddr string
	listener   net.Listener
	sockMap    *ebpf.Map
	proxyMap   *ebpf.Map
	connMap    *ebpf.Map

	mu      sync.RWMutex
	conns   map[uint32]*ProxyConnection
	nextIdx uint32

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// ProxyConnection 代理连接
type ProxyConnection struct {
	ClientConn net.Conn
	ServerConn net.Conn
	ClientIdx  uint32
	ServerIdx  uint32
	OrigDst    *net.TCPAddr
	StartTime  time.Time
	BytesTx    uint64
	BytesRx    uint64
}

// NewTPROXYBridge 创建 TPROXY 桥接器
func NewTPROXYBridge(listenAddr string, sockMap, proxyMap, connMap *ebpf.Map) *TPROXYBridge {
	ctx, cancel := context.WithCancel(context.Background())
	return &TPROXYBridge{
		listenAddr: listenAddr,
		sockMap:    sockMap,
		proxyMap:   proxyMap,
		connMap:    connMap,
		conns:      make(map[uint32]*ProxyConnection),
		nextIdx:    1,
		ctx:        ctx,
		cancel:     cancel,
	}
}

// Stop 停止桥接器
func (tb *TPROXYBridge) Stop() error {
	tb.cancel()

	if tb.listener != nil {
		tb.listener.Close()
	}

	tb.mu.Lock()
	for _, conn := range tb.conns {
		tb.cleanupConnection(conn)
	}
	tb.mu.Unlock()

	tb.wg.Wait()
	log.Println("🛑 TPROXY 桥接器已停止")
	return nil
}

// acceptLoop 接受连接循环
func (tb *TPROXYBridge) acceptLoop() {
	defer tb.wg.Done()

	for {
		select {
		case <-tb.ctx.Done():
			return
		default:
		}

		conn, err := tb.listener.Accept()
		if err != nil {
			if tb.ctx.Err() != nil {
				return
			}
			log.Printf("⚠️  接受连接失败: %v", err)
			continue
		}

		go tb.handleConnection(conn)
	}
}

// handleConnection 处理新连接
func (tb *TPROXYBridge) handleConnection(clientConn net.Conn) {
	origDst, err := tb.getOriginalDst(clientConn)
	if err != nil {
		log.Printf("⚠️  获取原始目标失败: %v", err)
		clientConn.Close()
		return
	}

	log.Printf("🔗 新连接: %s → %s", clientConn.RemoteAddr(), origDst)

	serverConn, err := net.DialTimeout("tcp", origDst.String(), 10*time.Second)
	if err != nil {
		log.Printf("❌ 连接目标失败: %s → %v", origDst, err)
		clientConn.Close()
		return
	}

	tb.mu.Lock()
	clientIdx := tb.nextIdx
	tb.nextIdx++
	serverIdx := tb.nextIdx
	tb.nextIdx++
	tb.mu.Unlock()

	proxyConn := &ProxyConnection{
		ClientConn: clientConn,
		ServerConn: serverConn,
		ClientIdx:  clientIdx,
		ServerIdx:  serverIdx,
		OrigDst:    origDst,
		StartTime:  time.Now(),
	}

	if err := tb.injectSockmap(proxyConn); err != nil {
		log.Printf("⚠️  Sockmap 注入失败，降级到用户态: %v", err)
		go tb.userSpaceRelay(proxyConn)
		return
	}

	tb.mu.Lock()
	tb.conns[clientIdx] = proxyConn
	tb.mu.Unlock()

	log.Printf("✅ 零拷贝路径已激活: idx=%d↔%d", clientIdx, serverIdx)

	go tb.monitorConnection(proxyConn)
}

// monitorConnection 监控连接（零拷贝模式）
func (tb *TPROXYBridge) monitorConnection(pc *ProxyConnection) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-tb.ctx.Done():
			tb.cleanupConnection(pc)
			return
		case <-ticker.C:
			if !tb.isConnectionAlive(pc) {
				log.Printf("🔌 连接已断开: idx=%d↔%d", pc.ClientIdx, pc.ServerIdx)
				tb.cleanupConnection(pc)
				return
			}

			if tb.proxyMap != nil {
				var pair ProxyPair
				if err := tb.proxyMap.Lookup(pc.ClientIdx, &pair); err == nil {
					pc.BytesTx = pair.BytesTx
					pc.BytesRx = pair.BytesRx
				}
			}
		}
	}
}

// isConnectionAlive 检查连接是否存活
func (tb *TPROXYBridge) isConnectionAlive(pc *ProxyConnection) bool {
	one := make([]byte, 1)
	pc.ClientConn.SetReadDeadline(time.Now().Add(1 * time.Millisecond))
	_, err := pc.ClientConn.Read(one)
	pc.ClientConn.SetReadDeadline(time.Time{})

	if err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			return true
		}
		return false
	}
	return true
}

// cleanupConnection 清理连接
func (tb *TPROXYBridge) cleanupConnection(pc *ProxyConnection) {
	if tb.sockMap != nil {
		tb.sockMap.Delete(pc.ClientIdx)
		tb.sockMap.Delete(pc.ServerIdx)
	}
	if tb.proxyMap != nil {
		tb.proxyMap.Delete(pc.ClientIdx)
		tb.proxyMap.Delete(pc.ServerIdx)
	}

	if pc.ClientConn != nil {
		pc.ClientConn.Close()
	}
	if pc.ServerConn != nil {
		pc.ServerConn.Close()
	}

	tb.mu.Lock()
	delete(tb.conns, pc.ClientIdx)
	tb.mu.Unlock()

	log.Printf("🧹 连接已清理: idx=%d↔%d, 传输=%d/%d bytes",
		pc.ClientIdx, pc.ServerIdx, pc.BytesTx, pc.BytesRx)
}

// userSpaceRelay 用户态转发（降级方案）
func (tb *TPROXYBridge) userSpaceRelay(pc *ProxyConnection) {
	defer tb.cleanupConnection(pc)

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		tb.relay(pc.ClientConn, pc.ServerConn, &pc.BytesTx)
	}()

	go func() {
		defer wg.Done()
		tb.relay(pc.ServerConn, pc.ClientConn, &pc.BytesRx)
	}()

	wg.Wait()
}

// relay 数据转发
func (tb *TPROXYBridge) relay(src, dst net.Conn, counter *uint64) {
	buf := make([]byte, 32*1024)
	for {
		select {
		case <-tb.ctx.Done():
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

		*counter += uint64(n)
	}
}

// healthMonitor 健康监控
func (tb *TPROXYBridge) healthMonitor() {
	defer tb.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-tb.ctx.Done():
			return
		case <-ticker.C:
			tb.mu.RLock()
			activeConns := len(tb.conns)
			tb.mu.RUnlock()

			log.Printf("📊 TPROXY 状态: 活跃连接=%d", activeConns)
		}
	}
}

// GetStats 获取统计信息
func (tb *TPROXYBridge) GetStats() map[string]uint64 {
	tb.mu.RLock()
	defer tb.mu.RUnlock()

	var totalTx, totalRx uint64
	for _, conn := range tb.conns {
		totalTx += conn.BytesTx
		totalRx += conn.BytesRx
	}

	return map[string]uint64{
		"active_connections": uint64(len(tb.conns)),
		"total_tx_bytes":     totalTx,
		"total_rx_bytes":     totalRx,
	}
}
