//go:build linux

package tun

import (
	"fmt"
	"os"
	"os/exec"
	"unsafe"

	"golang.org/x/sys/unix"
)

const (
	tunDevice  = "/dev/net/tun"
	ifnameSize = 16
)

// ioctl flags
const (
	iffTUN    = 0x0001
	iffNOPI   = 0x1000
	tunSetIFF = 0x400454ca
)

type ifReq struct {
	Name  [ifnameSize]byte
	Flags uint16
	_     [22]byte // padding
}

// LinuxTUNDevice implements TUNDevice for Linux.
type LinuxTUNDevice struct {
	fd   int
	file *os.File
	name string
	mtu  int
}

func createPlatformTUN(name string, mtu int) (TUNDevice, error) {
	fd, err := unix.Open(tunDevice, unix.O_RDWR|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w (need CAP_NET_ADMIN)", tunDevice, err)
	}

	var req ifReq
	copy(req.Name[:], name)
	req.Flags = iffTUN | iffNOPI

	_, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), uintptr(tunSetIFF), uintptr(unsafe.Pointer(&req)))
	if errno != 0 {
		unix.Close(fd)
		return nil, fmt.Errorf("ioctl TUNSETIFF: %w", errno)
	}

	actualName := string(req.Name[:clen(req.Name[:])])

	dev := &LinuxTUNDevice{
		fd:   fd,
		file: os.NewFile(uintptr(fd), tunDevice),
		name: actualName,
		mtu:  mtu,
	}

	// Configure interface
	if err := exec.Command("ip", "link", "set", actualName, "up").Run(); err != nil {
		dev.Close()
		return nil, fmt.Errorf("ip link set up: %w", err)
	}
	if err := exec.Command("ip", "addr", "add", "10.7.0.2/24", "dev", actualName).Run(); err != nil {
		dev.Close()
		return nil, fmt.Errorf("ip addr add: %w", err)
	}
	if err := exec.Command("ip", "link", "set", actualName, "mtu", fmt.Sprintf("%d", mtu)).Run(); err != nil {
		dev.Close()
		return nil, fmt.Errorf("ip link set mtu: %w", err)
	}

	return dev, nil
}

func (d *LinuxTUNDevice) Read(buf []byte) (int, error)  { return d.file.Read(buf) }
func (d *LinuxTUNDevice) Write(buf []byte) (int, error) { return d.file.Write(buf) }
func (d *LinuxTUNDevice) Name() string                  { return d.name }
func (d *LinuxTUNDevice) MTU() int                      { return d.mtu }

func (d *LinuxTUNDevice) Close() error {
	_ = exec.Command("ip", "link", "delete", d.name).Run()
	return d.file.Close()
}

func clen(b []byte) int {
	for i, c := range b {
		if c == 0 {
			return i
		}
	}
	return len(b)
}

func cleanupStaleWindows() {} // no-op on Linux
