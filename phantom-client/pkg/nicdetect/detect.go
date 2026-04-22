// Package nicdetect provides physical NIC outbound detection without sending
// any probe packets to external addresses. It replaces the legacy
// net.Dial("udp4","8.8.8.8:53") approach with OS routing table queries.
package nicdetect

import (
	"fmt"
	"net"
	"strings"
)

// PhysicalNICDetector detects the physical outbound IP for reaching a target.
type PhysicalNICDetector interface {
	// DetectOutbound returns the physical outbound IP to reach targetIP.
	// It must NOT send any packets to external addresses.
	DetectOutbound(targetIP string) (net.IP, error)
}

// NewDetector creates a platform-specific PhysicalNICDetector.
// On Linux: parses `ip route get` output.
// On Windows: parses `route print` output.
// Falls back to interface enumeration if platform detection fails.
func NewDetector() PhysicalNICDetector {
	return newPlatformDetector()
}

// fallbackDetect enumerates non-loopback, non-TUN interfaces and returns
// the first valid IPv4 address found. Used when route table query fails.
func fallbackDetect() (net.IP, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("enumerate interfaces: %w", err)
	}

	for _, iface := range ifaces {
		if !isPhysicalInterface(iface) {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			ip := extractIPv4(addr)
			if ip != nil {
				return ip, nil
			}
		}
	}

	return nil, fmt.Errorf("no suitable physical interface with IPv4 found")
}

// isPhysicalInterface returns true if the interface is likely a physical NIC:
// - not loopback
// - not a TUN/TAP device
// - is up
// - has the multicast or broadcast flag (physical NICs typically do)
func isPhysicalInterface(iface net.Interface) bool {
	// Must be up
	if iface.Flags&net.FlagUp == 0 {
		return false
	}
	// Skip loopback
	if iface.Flags&net.FlagLoopback != 0 {
		return false
	}
	// Skip TUN/TAP devices by name heuristic
	name := strings.ToLower(iface.Name)
	tunPrefixes := []string{"tun", "tap", "wg", "wintun", "utun", "phantom"}
	for _, prefix := range tunPrefixes {
		if strings.HasPrefix(name, prefix) {
			return false
		}
	}
	return true
}

// extractIPv4 extracts a non-loopback IPv4 address from a net.Addr.
func extractIPv4(addr net.Addr) net.IP {
	var ip net.IP
	switch v := addr.(type) {
	case *net.IPNet:
		ip = v.IP
	case *net.IPAddr:
		ip = v.IP
	default:
		return nil
	}

	ip = ip.To4()
	if ip == nil {
		return nil
	}
	if ip.IsLoopback() {
		return nil
	}
	return ip
}

// FallbackDetect is exported for testing the fallback logic.
func FallbackDetect() (net.IP, error) {
	return fallbackDetect()
}

// IsPhysicalInterface is exported for testing.
func IsPhysicalInterface(iface net.Interface) bool {
	return isPhysicalInterface(iface)
}

// ExtractIPv4 is exported for testing.
func ExtractIPv4(addr net.Addr) net.IP {
	return extractIPv4(addr)
}
