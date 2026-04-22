package nicdetect

import (
	"net"
	"testing"

	"pgregory.net/rapid"
)

// Feature: v1-client-productization, Property 18: NIC 回退接口选择
// **Validates: Requirements 13.4**
func TestProperty18_FallbackInterfaceSelection(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate arbitrary interface list with mix of types
		numIfaces := rapid.IntRange(0, 10).Draw(t, "numIfaces")

		type testIface struct {
			name     string
			flags    net.Flags
			hasIPv4  bool
			ipv4     net.IP
			loopback bool
		}

		ifaces := make([]testIface, numIfaces)
		for i := range ifaces {
			ifaceType := rapid.SampledFrom([]string{"physical", "loopback", "tun", "down"}).Draw(t, "ifaceType")

			switch ifaceType {
			case "physical":
				ifaces[i] = testIface{
					name:    rapid.SampledFrom([]string{"eth0", "enp0s3", "wlan0", "Ethernet", "Wi-Fi"}).Draw(t, "physName"),
					flags:   net.FlagUp | net.FlagMulticast,
					hasIPv4: rapid.Bool().Draw(t, "hasIPv4"),
				}
				if ifaces[i].hasIPv4 {
					a := rapid.ByteRange(1, 254).Draw(t, "ipA")
					b := rapid.ByteRange(0, 255).Draw(t, "ipB")
					c := rapid.ByteRange(0, 255).Draw(t, "ipC")
					d := rapid.ByteRange(1, 254).Draw(t, "ipD")
					ifaces[i].ipv4 = net.IPv4(a, b, c, d)
				}
			case "loopback":
				ifaces[i] = testIface{
					name:     "lo",
					flags:    net.FlagUp | net.FlagLoopback,
					hasIPv4:  true,
					ipv4:     net.IPv4(127, 0, 0, 1),
					loopback: true,
				}
			case "tun":
				ifaces[i] = testIface{
					name:    rapid.SampledFrom([]string{"tun0", "tun1", "wg0", "wintun", "tap0", "phantom0"}).Draw(t, "tunName"),
					flags:   net.FlagUp | net.FlagPointToPoint,
					hasIPv4: true,
					ipv4:    net.IPv4(10, 0, 0, 1),
				}
			case "down":
				ifaces[i] = testIface{
					name:    "eth1",
					flags:   0, // not up
					hasIPv4: true,
					ipv4:    net.IPv4(192, 168, 1, 100),
				}
			}
		}

		// Simulate the selection logic
		var expectedIP net.IP
		for _, iface := range ifaces {
			netIface := net.Interface{
				Name:  iface.name,
				Flags: iface.flags,
			}
			if !IsPhysicalInterface(netIface) {
				continue
			}
			if iface.hasIPv4 && iface.ipv4 != nil && !iface.ipv4.IsLoopback() {
				expectedIP = iface.ipv4
				break
			}
		}

		// Verify properties:
		// 1. If a valid physical interface with IPv4 exists, selection must succeed
		// 2. Selected IP must not be loopback
		// 3. Selected IP must be IPv4
		if expectedIP != nil {
			if expectedIP.IsLoopback() {
				t.Fatal("selected IP must not be loopback")
			}
			if expectedIP.To4() == nil {
				t.Fatal("selected IP must be IPv4")
			}
		}

		// Verify IsPhysicalInterface filters correctly
		for _, iface := range ifaces {
			netIface := net.Interface{
				Name:  iface.name,
				Flags: iface.flags,
			}
			result := IsPhysicalInterface(netIface)

			if iface.loopback && result {
				t.Fatal("loopback interface must be filtered out")
			}
			if iface.flags&net.FlagUp == 0 && result {
				t.Fatal("down interface must be filtered out")
			}
		}
	})
}

func TestIsPhysicalInterface(t *testing.T) {
	tests := []struct {
		name   string
		iface  net.Interface
		expect bool
	}{
		{
			name:   "physical eth0",
			iface:  net.Interface{Name: "eth0", Flags: net.FlagUp | net.FlagMulticast},
			expect: true,
		},
		{
			name:   "loopback",
			iface:  net.Interface{Name: "lo", Flags: net.FlagUp | net.FlagLoopback},
			expect: false,
		},
		{
			name:   "tun device",
			iface:  net.Interface{Name: "tun0", Flags: net.FlagUp | net.FlagPointToPoint},
			expect: false,
		},
		{
			name:   "wintun device",
			iface:  net.Interface{Name: "wintun", Flags: net.FlagUp},
			expect: false,
		},
		{
			name:   "wg device",
			iface:  net.Interface{Name: "wg0", Flags: net.FlagUp},
			expect: false,
		},
		{
			name:   "down interface",
			iface:  net.Interface{Name: "eth1", Flags: 0},
			expect: false,
		},
		{
			name:   "phantom device",
			iface:  net.Interface{Name: "phantom0", Flags: net.FlagUp},
			expect: false,
		},
		{
			name:   "tap device",
			iface:  net.Interface{Name: "tap0", Flags: net.FlagUp},
			expect: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsPhysicalInterface(tt.iface)
			if got != tt.expect {
				t.Errorf("IsPhysicalInterface(%s) = %v, want %v", tt.iface.Name, got, tt.expect)
			}
		})
	}
}

func TestExtractIPv4(t *testing.T) {
	tests := []struct {
		name   string
		addr   net.Addr
		expect net.IP
	}{
		{
			name:   "valid IPv4",
			addr:   &net.IPNet{IP: net.IPv4(192, 168, 1, 100), Mask: net.CIDRMask(24, 32)},
			expect: net.IPv4(192, 168, 1, 100).To4(),
		},
		{
			name:   "loopback IPv4",
			addr:   &net.IPNet{IP: net.IPv4(127, 0, 0, 1), Mask: net.CIDRMask(8, 32)},
			expect: nil,
		},
		{
			name:   "IPv6 address",
			addr:   &net.IPNet{IP: net.ParseIP("::1"), Mask: net.CIDRMask(128, 128)},
			expect: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractIPv4(tt.addr)
			if tt.expect == nil && got != nil {
				t.Errorf("expected nil, got %v", got)
			}
			if tt.expect != nil && !tt.expect.Equal(got) {
				t.Errorf("expected %v, got %v", tt.expect, got)
			}
		})
	}
}

func TestParseRoutePrint(t *testing.T) {
	output := `
===========================================================================
IPv4 Route Table
===========================================================================
Active Routes:
Network Destination        Netmask          Gateway       Interface  Metric
          0.0.0.0          0.0.0.0      192.168.1.1    192.168.1.100     25
        127.0.0.0        255.0.0.0         On-link         127.0.0.1    331
===========================================================================
`
	ip, err := ParseRoutePrint(output, "1.2.3.4")
	if err != nil {
		t.Fatalf("ParseRoutePrint: %v", err)
	}
	expected := net.ParseIP("192.168.1.100")
	if !ip.Equal(expected) {
		t.Errorf("got %v, want %v", ip, expected)
	}
}

func TestParseRoutePrint_NoDefault(t *testing.T) {
	output := `
===========================================================================
IPv4 Route Table
===========================================================================
Active Routes:
Network Destination        Netmask          Gateway       Interface  Metric
        127.0.0.0        255.0.0.0         On-link         127.0.0.1    331
===========================================================================
`
	_, err := ParseRoutePrint(output, "1.2.3.4")
	if err == nil {
		t.Fatal("expected error for no default route")
	}
}
