//go:build !windows

package tun

// SetWintunDLL is a no-op on non-Windows platforms.
func SetWintunDLL(dll []byte) {}
