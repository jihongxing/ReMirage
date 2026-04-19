//go:build windows

package tun

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

// WintunDLL is set by the main package via SetWintunDLL before CreateTUN is called.
// This avoids cross-package embed path issues.
var WintunDLL []byte

// SetWintunDLL sets the embedded wintun.dll bytes.
func SetWintunDLL(dll []byte) {
	WintunDLL = dll
}

// WintunDevice implements TUNDevice for Windows via embedded wintun.dll.
type WintunDevice struct {
	adapter uintptr
	session uintptr
	dll     *windows.DLL
	dllPath string
	dllDir  string
	name    string
	mtu     int
	closed  bool
}

func createPlatformTUN(name string, mtu int) (TUNDevice, error) {
	if len(WintunDLL) == 0 {
		return nil, fmt.Errorf("wintun.dll not loaded: call tun.SetWintunDLL() first")
	}

	// 1. Extract wintun.dll to temp dir
	tmpDir, err := os.MkdirTemp("", "mirage-wintun-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}

	dllPath := filepath.Join(tmpDir, "wintun.dll")
	if err := os.WriteFile(dllPath, WintunDLL, 0600); err != nil {
		os.RemoveAll(tmpDir)
		return nil, fmt.Errorf("write wintun.dll: %w", err)
	}

	// 2. Load DLL
	dll, err := windows.LoadDLL(dllPath)
	if err != nil {
		os.RemoveAll(tmpDir)
		return nil, fmt.Errorf("load wintun.dll: %w", err)
	}

	dev := &WintunDevice{
		dll:     dll,
		dllPath: dllPath,
		dllDir:  tmpDir,
		name:    name,
		mtu:     mtu,
	}

	// 3. Create adapter (WintunCreateAdapter)
	// In production, call actual Wintun API procs.
	// Placeholder: the real implementation would call:
	//   createAdapter, _ := dll.FindProc("WintunCreateAdapter")
	//   adapter, _, _ := createAdapter.Call(...)
	//   startSession, _ := dll.FindProc("WintunStartSession")
	//   session, _, _ := startSession.Call(adapter, 0x400000)

	return dev, nil
}

func (d *WintunDevice) Read(buf []byte) (int, error) {
	// Placeholder: real impl calls WintunReceivePacket
	return 0, fmt.Errorf("wintun read not implemented in placeholder")
}

func (d *WintunDevice) Write(buf []byte) (int, error) {
	// Placeholder: real impl calls WintunAllocateSendPacket + WintunSendPacket
	return 0, fmt.Errorf("wintun write not implemented in placeholder")
}

func (d *WintunDevice) Name() string { return d.name }
func (d *WintunDevice) MTU() int     { return d.mtu }

func (d *WintunDevice) Close() error {
	if d.closed {
		return nil
	}
	d.closed = true

	// Close session & adapter (real impl calls WintunEndSession, WintunCloseAdapter)
	if d.dll != nil {
		d.dll.Release()
	}
	// Remove temp files
	os.Remove(d.dllPath)
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
