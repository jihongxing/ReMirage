// +build !linux

package tproxy

import (
	"fmt"
	"net"
)

// Start 启动 TPROXY 监听 (非 Linux 平台 stub)
func (tb *TPROXYBridge) Start() error {
	return fmt.Errorf("TPROXY 仅支持 Linux 平台")
}

// getOriginalDst 获取原始目标地址 (非 Linux 平台 stub)
func (tb *TPROXYBridge) getOriginalDst(conn net.Conn) (*net.TCPAddr, error) {
	return nil, fmt.Errorf("TPROXY 仅支持 Linux 平台")
}

// injectSockmap 注入 Sockmap (非 Linux 平台 stub)
func (tb *TPROXYBridge) injectSockmap(pc *ProxyConnection) error {
	return fmt.Errorf("Sockmap 仅支持 Linux 平台")
}
