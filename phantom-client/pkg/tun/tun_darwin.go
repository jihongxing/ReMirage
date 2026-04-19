//go:build darwin

package tun

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"unsafe"

	"golang.org/x/sys/unix"
)

const (
	pfSystem      = 2 // PF_SYSTEM
	sockDgram     = unix.SOCK_DGRAM
	sysprotoCtl   = 2 // SYSPROTO_CONTROL
	utunCtlName   = "com.apple.net.utun_control"
	utunOptIfname = 2
	ctlIOCGInfo   = 0xc0644e03
)

type ctlInfo struct {
	ID   uint32
	Name [96]byte
}

type sockaddrCtl struct {
	Len       uint8
	Family    uint8
	SSSysaddr uint16
	ID        uint32
	Unit      uint32
	Reserved  [5]uint32
}

// UtunDevice implements TUNDevice for macOS.
type UtunDevice struct {
	fd   int
	file *os.File
	name string
	mtu  int
}

func createPlatformTUN(name string, mtu int) (TUNDevice, error) {
	fd, err := unix.Socket(pfSystem, sockDgram, sysprotoCtl)
	if err != nil {
		return nil, fmt.Errorf("socket PF_SYSTEM: %w (need root/sudo)", err)
	}

	var info ctlInfo
	copy(info.Name[:], utunCtlName)
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), uintptr(ctlIOCGInfo), uintptr(unsafe.Pointer(&info)))
	if errno != 0 {
		unix.Close(fd)
		return nil, fmt.Errorf("ioctl CTLIOCGINFO: %w", errno)
	}

	addr := sockaddrCtl{
		Len:       uint8(unsafe.Sizeof(sockaddrCtl{})),
		Family:    unix.AF_SYSTEM,
		SSSysaddr: 2, // AF_SYS_CONTROL
		ID:        info.ID,
		Unit:      0, // let kernel assign utunN
	}

	_, _, errno = unix.Syscall(unix.SYS_CONNECT, uintptr(fd), uintptr(unsafe.Pointer(&addr)), unsafe.Sizeof(addr))
	if errno != 0 {
		unix.Close(fd)
		return nil, fmt.Errorf("connect utun: %w", errno)
	}

	// Get assigned interface name
	ifname := make([]byte, 32)
	ifnameLen := uint32(len(ifname))
	_, _, errno = unix.Syscall6(unix.SYS_GETSOCKOPT, uintptr(fd), sysprotoCtl, utunOptIfname,
		uintptr(unsafe.Pointer(&ifname[0])), uintptr(unsafe.Pointer(&ifnameLen)), 0)
	if errno != 0 {
		unix.Close(fd)
		return nil, fmt.Errorf("getsockopt utun name: %w", errno)
	}
	actualName := string(ifname[:ifnameLen-1]) // trim null

	dev := &UtunDevice{
		fd:   fd,
		file: os.NewFile(uintptr(fd), "/dev/"+actualName),
		name: actualName,
		mtu:  mtu,
	}

	// Configure IP and MTU
	ip := net.IPv4(10, 7, 0, 2)
	peer := net.IPv4(10, 7, 0, 1)
	if err := exec.Command("ifconfig", actualName, ip.String(), peer.String(), "mtu", fmt.Sprintf("%d", mtu), "up").Run(); err != nil {
		dev.Close()
		return nil, fmt.Errorf("ifconfig: %w", err)
	}

	return dev, nil
}

func (d *UtunDevice) Read(buf []byte) (int, error) {
	// macOS utun prepends 4-byte protocol header
	tmp := make([]byte, len(buf)+4)
	n, err := d.file.Read(tmp)
	if err != nil {
		return 0, err
	}
	if n <= 4 {
		return 0, nil
	}
	copy(buf, tmp[4:n])
	return n - 4, nil
}

func (d *UtunDevice) Write(buf []byte) (int, error) {
	// Prepend 4-byte AF_INET header
	tmp := make([]byte, len(buf)+4)
	tmp[3] = 2 // AF_INET
	copy(tmp[4:], buf)
	n, err := d.file.Write(tmp)
	if err != nil {
		return 0, err
	}
	return n - 4, nil
}

func (d *UtunDevice) Name() string { return d.name }
func (d *UtunDevice) MTU() int     { return d.mtu }

func (d *UtunDevice) Close() error {
	return d.file.Close()
}

func cleanupStaleWindows() {} // no-op on macOS
