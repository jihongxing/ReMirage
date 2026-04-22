//go:build linux

package dataplane

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"sync"
	"unsafe"

	"golang.org/x/sys/unix"
)

const (
	tunDevPath = "/dev/net/tun"
	ifnameSize = 16
	iffTUN     = 0x0001
	iffNOPI    = 0x1000
	tunSetIFF  = 0x400454ca
)

type ifReq struct {
	Name  [ifnameSize]byte
	Flags uint16
	_     [22]byte // padding
}

// TUNInjector 通过 Linux TUN 设备将 IP 包注入本机网络栈。
type TUNInjector struct {
	mu     sync.Mutex
	fd     *os.File
	name   string
	closed bool
}

// NewTUNInjector 创建并初始化 TUN 设备。
// 需要 CAP_NET_ADMIN 权限。
func NewTUNInjector(cfg TUNConfig) (*TUNInjector, error) {
	fd, err := unix.Open(tunDevPath, unix.O_RDWR|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("dataplane: open %s: %w (need CAP_NET_ADMIN)", tunDevPath, err)
	}

	var req ifReq
	copy(req.Name[:], cfg.DeviceName)
	req.Flags = iffTUN | iffNOPI

	_, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), uintptr(tunSetIFF), uintptr(unsafe.Pointer(&req)))
	if errno != 0 {
		unix.Close(fd)
		return nil, fmt.Errorf("dataplane: ioctl TUNSETIFF: %w", errno)
	}

	actualName := string(req.Name[:clen(req.Name[:])])
	file := os.NewFile(uintptr(fd), tunDevPath)

	// 配置接口
	if err := exec.Command("ip", "link", "set", actualName, "up").Run(); err != nil {
		file.Close()
		return nil, fmt.Errorf("dataplane: ip link set up: %w", err)
	}
	if err := exec.Command("ip", "addr", "add", cfg.Subnet, "dev", actualName).Run(); err != nil {
		file.Close()
		return nil, fmt.Errorf("dataplane: ip addr add: %w", err)
	}
	if err := exec.Command("ip", "link", "set", actualName, "mtu", fmt.Sprintf("%d", cfg.MTU)).Run(); err != nil {
		file.Close()
		return nil, fmt.Errorf("dataplane: ip link set mtu: %w", err)
	}

	log.Printf("[DataPlane] TUN 设备已创建: %s (MTU %d, subnet %s)", actualName, cfg.MTU, cfg.Subnet)

	return &TUNInjector{
		fd:   file,
		name: actualName,
	}, nil
}

// InjectIPPacket 将完整 IP 包写入 TUN fd，注入本机网络栈。
func (t *TUNInjector) InjectIPPacket(pkt []byte) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed || t.fd == nil {
		return fmt.Errorf("dataplane: TUN device %s not open", t.name)
	}
	_, err := t.fd.Write(pkt)
	return err
}

// Close 关闭 TUN 设备并清理接口。
func (t *TUNInjector) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return nil
	}
	t.closed = true
	log.Printf("[DataPlane] 关闭 TUN 设备: %s", t.name)
	_ = exec.Command("ip", "link", "delete", t.name).Run()
	return t.fd.Close()
}

func clen(b []byte) int {
	for i, c := range b {
		if c == 0 {
			return i
		}
	}
	return len(b)
}
