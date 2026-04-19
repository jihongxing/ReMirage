// +build linux

package tproxy

import (
	"fmt"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"syscall"
)

const (
	SPLICE_F_MOVE     = 1
	SPLICE_F_NONBLOCK = 2
	SPLICE_F_MORE     = 4
	PIPE_BUF_SIZE     = 64 * 1024
	F_SETPIPE_SZ      = 1031
)

// NewSpliceBridge 创建 Splice 桥接器 (Linux)
func NewSpliceBridge(clientConn, serverConn net.Conn) (*SpliceBridge, error) {
	clientFile, err := clientConn.(*net.TCPConn).File()
	if err != nil {
		return nil, fmt.Errorf("获取客户端 FD 失败: %w", err)
	}

	serverFile, err := serverConn.(*net.TCPConn).File()
	if err != nil {
		clientFile.Close()
		return nil, fmt.Errorf("获取服务端 FD 失败: %w", err)
	}

	pipeR, pipeW, err := os.Pipe()
	if err != nil {
		clientFile.Close()
		serverFile.Close()
		return nil, fmt.Errorf("创建管道失败: %w", err)
	}

	// 设置管道大小
	syscall.Syscall(syscall.SYS_FCNTL, pipeW.Fd(), F_SETPIPE_SZ, PIPE_BUF_SIZE)

	return &SpliceBridge{
		clientConn: clientConn,
		serverConn: serverConn,
		clientFD:   int(clientFile.Fd()),
		serverFD:   int(serverFile.Fd()),
		pipeR:      pipeR,
		pipeW:      pipeW,
		done:       make(chan struct{}),
	}, nil
}

// Start 启动 Splice 转发 (Linux)
func (sb *SpliceBridge) Start() {
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		sb.spliceRelay(sb.clientFD, sb.serverFD, &sb.bytesTx)
	}()

	go func() {
		defer wg.Done()
		sb.spliceRelay(sb.serverFD, sb.clientFD, &sb.bytesRx)
	}()

	wg.Wait()
	close(sb.done)
}

// spliceRelay Splice 转发 (Linux)
func (sb *SpliceBridge) spliceRelay(srcFD, dstFD int, counter *uint64) {
	pipeFDs := [2]int{int(sb.pipeR.Fd()), int(sb.pipeW.Fd())}

	for {
		select {
		case <-sb.done:
			return
		default:
		}

		n1, err := syscall.Splice(srcFD, nil, pipeFDs[1], nil, PIPE_BUF_SIZE, SPLICE_F_MOVE|SPLICE_F_MORE)
		if err != nil || n1 == 0 {
			return
		}

		n2, err := syscall.Splice(pipeFDs[0], nil, dstFD, nil, int(n1), SPLICE_F_MOVE|SPLICE_F_MORE)
		if err != nil || n2 == 0 {
			return
		}

		atomic.AddUint64(counter, uint64(n2))
	}
}
