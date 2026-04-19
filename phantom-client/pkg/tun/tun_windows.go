//go:build windows

package tun

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"golang.zx2c4.com/wintun"
)

// WintunDLL is set by the main package via SetWintunDLL before CreateTUN is called.
var WintunDLL []byte

// SetWintunDLL extracts and loads the embedded wintun.dll.
// Must be called before CreateTUN on Windows.
func SetWintunDLL(dll []byte) {
	WintunDLL = dll
}

// WintunDevice implements TUNDevice using the official wintun library.
type WintunDevice struct {
	adapter *wintun.Adapter
	session wintun.Session
	name    string
	mtu     int
	closed  bool
	mu      sync.Mutex
	dllDir  string // temp dir for cleanup
}

func createPlatformTUN(name string, mtu int) (TUNDevice, error) {
	if len(WintunDLL) == 0 {
		return nil, fmt.Errorf("wintun.dll not loaded: call tun.SetWintunDLL() first")
	}

	// 1. Extract wintun.dll to exe directory (Windows DLL search checks exe dir first)
	exePath, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("get executable path: %w", err)
	}
	exeDir := filepath.Dir(exePath)
	dllPath := filepath.Join(exeDir, "wintun.dll")

	// Write DLL if not already present
	if _, err := os.Stat(dllPath); os.IsNotExist(err) {
		if err := os.WriteFile(dllPath, WintunDLL, 0644); err != nil {
			// Fallback to temp dir
			tmpDir, _ := os.MkdirTemp("", "mirage-wintun-*")
			dllPath = filepath.Join(tmpDir, "wintun.dll")
			if err := os.WriteFile(dllPath, WintunDLL, 0644); err != nil {
				return nil, fmt.Errorf("write wintun.dll: %w", err)
			}
			exeDir = tmpDir
		}
	}

	// 2. Ensure DLL is findable via PATH
	os.Setenv("PATH", exeDir+";"+os.Getenv("PATH"))

	// 3. Create adapter
	adapter, err := wintun.CreateAdapter(name, "Mirage", nil)
	if err != nil {
		return nil, fmt.Errorf("WintunCreateAdapter: %w", err)
	}

	// 4. Start session with 8MB ring buffer (must be power of 2, range 128KB-64MB)
	session, err := adapter.StartSession(0x800000)
	if err != nil {
		adapter.Close()
		return nil, fmt.Errorf("WintunStartSession: %w", err)
	}

	dev := &WintunDevice{
		adapter: adapter,
		session: session,
		name:    name,
		mtu:     mtu,
		dllDir:  exeDir,
	}

	return dev, nil
}

// Read blocks until a packet arrives from the Windows network stack.
// The packet is copied into buf. This is the egress path (host -> tunnel).
func (d *WintunDevice) Read(buf []byte) (int, error) {
	if d.closed {
		return 0, fmt.Errorf("device closed")
	}

	// ReceivePacket blocks until a packet is available.
	// Returns a slice backed by the ring buffer — must be released after copy.
	packet, err := d.session.ReceivePacket()
	if err != nil {
		return 0, fmt.Errorf("ReceivePacket: %w", err)
	}

	n := copy(buf, packet)

	// Critical: release ring buffer memory immediately
	d.session.ReleaseReceivePacket(packet)

	return n, nil
}

// Write injects a packet into the Windows network stack.
// This is the ingress path (tunnel -> host).
func (d *WintunDevice) Write(buf []byte) (int, error) {
	if d.closed {
		return 0, fmt.Errorf("device closed")
	}

	n := len(buf)
	if n == 0 {
		return 0, nil
	}

	// Allocate exact-size packet from ring buffer
	packet, err := d.session.AllocateSendPacket(n)
	if err != nil {
		// Ring buffer full — drop packet to maintain stability
		return 0, nil
	}

	// Copy data and notify Windows kernel
	copy(packet, buf[:n])
	d.session.SendPacket(packet)

	return n, nil
}

func (d *WintunDevice) Name() string { return d.name }
func (d *WintunDevice) MTU() int     { return d.mtu }

func (d *WintunDevice) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.closed {
		return nil
	}
	d.closed = true

	d.session.End()
	d.adapter.Close()

	// Cleanup temp DLL
	os.RemoveAll(d.dllDir)
	return nil
}

func cleanupStaleWindows() {
	tmpDir := os.TempDir()
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), "mirage-wintun-") {
			path := filepath.Join(tmpDir, e.Name())
			_ = os.RemoveAll(path)
		}
	}
}
