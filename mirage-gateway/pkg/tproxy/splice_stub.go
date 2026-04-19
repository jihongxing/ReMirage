// +build !linux

package tproxy

import (
	"fmt"
	"net"
)

// NewSpliceBridge 创建 Splice 桥接器 (非 Linux stub)
func NewSpliceBridge(clientConn, serverConn net.Conn) (*SpliceBridge, error) {
	return nil, fmt.Errorf("Splice 仅支持 Linux 平台")
}

// Start 启动 Splice 转发 (非 Linux stub)
func (sb *SpliceBridge) Start() {
	// no-op
}

// spliceRelay Splice 转发 (非 Linux stub)
func (sb *SpliceBridge) spliceRelay(srcFD, dstFD int, counter *uint64) {
	// no-op
}
