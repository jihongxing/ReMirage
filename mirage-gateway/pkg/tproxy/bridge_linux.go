// +build linux

package tproxy

import (
	"fmt"
	"log"
	"net"
	"syscall"

	"golang.org/x/sys/unix"
)

const (
	IP_TRANSPARENT     = 19
	IP_RECVORIGDSTADDR = 20
	SO_ORIGINAL_DST    = 80
)

// Start 启动 TPROXY 监听 (Linux)
func (tb *TPROXYBridge) Start() error {
	lc := net.ListenConfig{
		Control: func(network, address string, c syscall.RawConn) error {
			var opErr error
			err := c.Control(func(fd uintptr) {
				opErr = unix.SetsockoptInt(int(fd), unix.SOL_IP, IP_TRANSPARENT, 1)
				if opErr != nil {
					return
				}
				opErr = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEADDR, 1)
				if opErr != nil {
					return
				}
				opErr = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEPORT, 1)
			})
			if err != nil {
				return err
			}
			return opErr
		},
	}

	listener, err := lc.Listen(tb.ctx, "tcp", tb.listenAddr)
	if err != nil {
		return fmt.Errorf("创建 TPROXY 监听器失败: %w", err)
	}
	tb.listener = listener

	log.Printf("✅ TPROXY 桥接器已启动: %s", tb.listenAddr)

	tb.wg.Add(1)
	go tb.acceptLoop()

	tb.wg.Add(1)
	go tb.healthMonitor()

	return nil
}

// getOriginalDst 获取原始目标地址 (Linux)
func (tb *TPROXYBridge) getOriginalDst(conn net.Conn) (*net.TCPAddr, error) {
	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		return nil, fmt.Errorf("非 TCP 连接")
	}

	file, err := tcpConn.File()
	if err != nil {
		return nil, fmt.Errorf("获取文件描述符失败: %w", err)
	}
	defer file.Close()

	fd := int(file.Fd())

	addr, err := syscall.GetsockoptIPv6Mreq(fd, syscall.IPPROTO_IP, SO_ORIGINAL_DST)
	if err == nil {
		ip := net.IPv4(addr.Multiaddr[4], addr.Multiaddr[5], addr.Multiaddr[6], addr.Multiaddr[7])
		port := int(addr.Multiaddr[2])<<8 + int(addr.Multiaddr[3])
		return &net.TCPAddr{IP: ip, Port: port}, nil
	}

	localAddr := conn.LocalAddr().(*net.TCPAddr)
	return localAddr, nil
}

// injectSockmap 注入 Sockmap (Linux)
func (tb *TPROXYBridge) injectSockmap(pc *ProxyConnection) error {
	if tb.sockMap == nil {
		return fmt.Errorf("Sockmap 未初始化")
	}

	clientFile, err := pc.ClientConn.(*net.TCPConn).File()
	if err != nil {
		return fmt.Errorf("获取客户端 FD 失败: %w", err)
	}
	clientFD := uint64(clientFile.Fd())
	clientFile.Close()

	serverFile, err := pc.ServerConn.(*net.TCPConn).File()
	if err != nil {
		return fmt.Errorf("获取服务端 FD 失败: %w", err)
	}
	serverFD := uint64(serverFile.Fd())
	serverFile.Close()

	if err := tb.sockMap.Put(pc.ClientIdx, clientFD); err != nil {
		return fmt.Errorf("注入客户端 Socket 失败: %w", err)
	}
	if err := tb.sockMap.Put(pc.ServerIdx, serverFD); err != nil {
		return fmt.Errorf("注入服务端 Socket 失败: %w", err)
	}

	if tb.proxyMap != nil {
		clientPair := ProxyPair{PeerIdx: pc.ServerIdx, Flags: 0}
		serverPair := ProxyPair{PeerIdx: pc.ClientIdx, Flags: 0}

		if err := tb.proxyMap.Put(pc.ClientIdx, clientPair); err != nil {
			return fmt.Errorf("注入客户端代理对失败: %w", err)
		}
		if err := tb.proxyMap.Put(pc.ServerIdx, serverPair); err != nil {
			return fmt.Errorf("注入服务端代理对失败: %w", err)
		}
	}

	return nil
}
