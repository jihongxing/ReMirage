package tun

import "runtime"

// TUNDevice is the cross-platform TUN interface.
type TUNDevice interface {
	Read(buf []byte) (int, error)
	Write(buf []byte) (int, error)
	Name() string
	MTU() int
	Close() error
}

// CreateTUN creates a platform-specific TUN device.
func CreateTUN(name string, mtu int) (TUNDevice, error) {
	return createPlatformTUN(name, mtu)
}

// CleanupStale removes leftover resources from previous crashes.
func CleanupStale() {
	switch runtime.GOOS {
	case "windows":
		cleanupStaleWindows()
	default:
		// Linux/macOS: TUN interfaces are auto-cleaned on process exit
	}
}
