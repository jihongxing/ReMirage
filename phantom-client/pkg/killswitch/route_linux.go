//go:build linux

package killswitch

import (
	"fmt"
	"os/exec"
	"strings"
)

type linuxPlatform struct{}

func newPlatform() Platform { return &linuxPlatform{} }

func (p *linuxPlatform) GetDefaultGateway() (string, string, error) {
	out, err := exec.Command("ip", "route", "show", "default").Output()
	if err != nil {
		return "", "", fmt.Errorf("ip route show: %w", err)
	}
	// Parse: "default via 192.168.1.1 dev eth0 ..."
	fields := strings.Fields(string(out))
	var gw, iface string
	for i, f := range fields {
		if f == "via" && i+1 < len(fields) {
			gw = fields[i+1]
		}
		if f == "dev" && i+1 < len(fields) {
			iface = fields[i+1]
		}
	}
	if gw == "" {
		return "", "", fmt.Errorf("no default gateway found")
	}
	return gw, iface, nil
}

func (p *linuxPlatform) DeleteDefaultRoute() error {
	return exec.Command("ip", "route", "del", "default").Run()
}

func (p *linuxPlatform) AddDefaultRoute(tunName string) error {
	return exec.Command("ip", "route", "add", "default", "dev", tunName).Run()
}

func (p *linuxPlatform) AddHostRoute(ip, gateway, iface string) error {
	return exec.Command("ip", "route", "add", ip+"/32", "via", gateway, "dev", iface).Run()
}

func (p *linuxPlatform) DeleteHostRoute(ip string) error {
	return exec.Command("ip", "route", "del", ip+"/32").Run()
}

func (p *linuxPlatform) RestoreDefaultRoute(gateway, iface string) error {
	return exec.Command("ip", "route", "add", "default", "via", gateway, "dev", iface).Run()
}
