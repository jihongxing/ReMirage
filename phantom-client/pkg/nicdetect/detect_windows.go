//go:build windows

package nicdetect

import (
	"fmt"
	"net"
	"os/exec"
	"strings"
)

type windowsDetector struct{}

func newPlatformDetector() PhysicalNICDetector {
	return &windowsDetector{}
}

// DetectOutbound uses `route print` to find the outbound interface for targetIP.
// Falls back to interface enumeration if parsing fails.
func (d *windowsDetector) DetectOutbound(targetIP string) (net.IP, error) {
	ip, err := detectViaRoutePrint(targetIP)
	if err != nil {
		return fallbackDetect()
	}
	return ip, nil
}

func detectViaRoutePrint(targetIP string) (net.IP, error) {
	out, err := exec.Command("route", "print").Output()
	if err != nil {
		return nil, fmt.Errorf("route print: %w", err)
	}
	return parseRoutePrint(string(out), targetIP)
}

// parseRoutePrint parses Windows `route print` output to find the gateway
// interface for the given target IP. It looks for the default route (0.0.0.0)
// in the IPv4 Route Table section and returns the interface IP.
//
// Typical route print output format:
// ===========================================================================
// IPv4 Route Table
// ===========================================================================
// Active Routes:
// Network Destination    Netmask          Gateway       Interface  Metric
//
//	0.0.0.0          0.0.0.0      192.168.1.1    192.168.1.100     25
func parseRoutePrint(output, targetIP string) (net.IP, error) {
	target := net.ParseIP(targetIP)
	if target == nil {
		return nil, fmt.Errorf("invalid target IP: %s", targetIP)
	}

	lines := strings.Split(output, "\n")
	inIPv4Section := false
	var defaultIfaceIP net.IP
	var bestMetric int = -1

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.Contains(trimmed, "IPv4 Route Table") {
			inIPv4Section = true
			continue
		}
		if strings.Contains(trimmed, "IPv6 Route Table") {
			inIPv4Section = false
			continue
		}

		if !inIPv4Section {
			continue
		}

		fields := strings.Fields(trimmed)
		if len(fields) < 5 {
			continue
		}

		// Check for default route: 0.0.0.0
		dest := fields[0]
		if dest != "0.0.0.0" {
			continue
		}

		ifaceIP := net.ParseIP(fields[3])
		if ifaceIP == nil || ifaceIP.To4() == nil {
			continue
		}

		// Parse metric for best route selection
		metric := 0
		fmt.Sscanf(fields[4], "%d", &metric)

		if bestMetric < 0 || metric < bestMetric {
			bestMetric = metric
			defaultIfaceIP = ifaceIP
		}
	}

	if defaultIfaceIP != nil {
		return defaultIfaceIP, nil
	}

	return nil, fmt.Errorf("no default route found in route print output")
}

// ParseRoutePrint is exported for testing.
func ParseRoutePrint(output, targetIP string) (net.IP, error) {
	return parseRoutePrint(output, targetIP)
}
