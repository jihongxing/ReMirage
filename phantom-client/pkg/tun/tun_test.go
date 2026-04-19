package tun

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// Mock TUN device for testing
type mockTUN struct {
	name   string
	mtu    int
	closed bool
	buf    []byte
}

func (m *mockTUN) Read(buf []byte) (int, error) {
	n := copy(buf, m.buf)
	return n, nil
}

func (m *mockTUN) Write(buf []byte) (int, error) {
	m.buf = make([]byte, len(buf))
	copy(m.buf, buf)
	return len(buf), nil
}

func (m *mockTUN) Name() string { return m.name }
func (m *mockTUN) MTU() int     { return m.mtu }
func (m *mockTUN) Close() error { m.closed = true; return nil }

func TestMockTUNReadWrite(t *testing.T) {
	dev := &mockTUN{name: "mirage0", mtu: 1400}
	data := []byte("test IP packet data")

	n, err := dev.Write(data)
	if err != nil || n != len(data) {
		t.Fatalf("write: n=%d err=%v", n, err)
	}

	buf := make([]byte, 1500)
	n, err = dev.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	if string(buf[:n]) != string(data) {
		t.Fatalf("expected %q, got %q", data, buf[:n])
	}
}

func TestMockTUNClose(t *testing.T) {
	dev := &mockTUN{name: "mirage0", mtu: 1400}
	if err := dev.Close(); err != nil {
		t.Fatal(err)
	}
	if !dev.closed {
		t.Fatal("expected closed")
	}
}

func TestCleanupStaleWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-only test")
	}

	// Create fake stale directory
	tmpDir := os.TempDir()
	staleDir := filepath.Join(tmpDir, "mirage-wintun-test-stale")
	if err := os.MkdirAll(staleDir, 0755); err != nil {
		t.Fatal(err)
	}
	fakeDLL := filepath.Join(staleDir, "wintun.dll")
	if err := os.WriteFile(fakeDLL, []byte("fake"), 0600); err != nil {
		t.Fatal(err)
	}

	cleanupStaleWindows()

	if _, err := os.Stat(staleDir); !os.IsNotExist(err) {
		t.Fatal("stale directory should have been cleaned up")
	}
}

func TestTUNDeviceInterface(t *testing.T) {
	// Verify mock satisfies interface
	var _ TUNDevice = (*mockTUN)(nil)
}
