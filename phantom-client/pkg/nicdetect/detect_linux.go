//go:build linux

package nicdetect

import (
	"fmt"
	"net"
	"os/exec"
	"strings"
)

type linuxDetector struct{}

func newPlatformDetector() PhysicalNICDetector {
	return &linuxDetector{}
}

// DetectOutbound uses `ip route get <targetIP>` to find the source IP
// without sending any packets. Parses the "src" field from output like:
//
//	1.2.3.4 via 192.168.1.1 dev eth0 src 192.168.1.100 uid 1000
func (d *linuxDetector) DetectOutbound(targetIP string) (net.IP, error) {
	ip, err := detectViaIPRoute(targetIP)
	if err != nil {
		// Fallback to interface enumeration
		return fallbackDetect()
	}
	return ip, nil
}

func detectViaIPRoute(targetIP string) (net.IP, error) {
	out, err := exec.Command("ip", "route", "get", targetIP).Output()
	if err != nil {
		return nil, fmt.Errorf("ip route get: %w", err)
	}
	return parseIPRouteSrc(string(out))
}

// parseIPRouteSrc extracts the src IP from `ip route get` output.
func parseIPRouteSrc(output string) (net.IP, error) {
	fields := strings.Fields(output)
	for i, f := range fields {
		if f == "src" && i+1 < len(fields) {
			ip := net.ParseIP(fields[i+1])
			if ip == nil {
				return nil, fmt.Errorf("invalid src IP: %s", fields[i+1])
			}
			return ip, nil
		}
	}
	return nil, fmt.Errorf("src field not found in ip route output")
}

// ParseIPRouteSrc is exported for testing.
func ParseIPRouteSrc(output string) (net.IP, error) {
	return parseIPRouteSrc(output)
}
