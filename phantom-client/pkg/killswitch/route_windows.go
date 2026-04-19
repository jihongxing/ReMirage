//go:build windows

package killswitch

import (
	"fmt"
	"os/exec"
	"strings"
)

type windowsPlatform struct{}

func newPlatform() Platform { return &windowsPlatform{} }

func (p *windowsPlatform) GetDefaultGateway() (string, string, error) {
	out, err := exec.Command("route", "print", "0.0.0.0", "mask", "0.0.0.0").Output()
	if err != nil {
		return "", "", fmt.Errorf("route print: %w", err)
	}
	// Parse output for default gateway line
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) >= 4 && fields[0] == "0.0.0.0" && fields[1] == "0.0.0.0" {
			return fields[2], fields[3], nil // gateway, interface
		}
	}
	return "", "", fmt.Errorf("no default gateway found")
}

func (p *windowsPlatform) DeleteDefaultRoute() error {
	return exec.Command("route", "delete", "0.0.0.0", "mask", "0.0.0.0").Run()
}

func (p *windowsPlatform) AddDefaultRoute(tunName string) error {
	// On Windows, we add via the TUN interface metric
	return exec.Command("route", "add", "0.0.0.0", "mask", "0.0.0.0", "10.7.0.1", "metric", "1").Run()
}

func (p *windowsPlatform) AddHostRoute(ip, gateway, iface string) error {
	return exec.Command("route", "add", ip, "mask", "255.255.255.255", gateway).Run()
}

func (p *windowsPlatform) DeleteHostRoute(ip string) error {
	return exec.Command("route", "delete", ip, "mask", "255.255.255.255").Run()
}

func (p *windowsPlatform) RestoreDefaultRoute(gateway, iface string) error {
	return exec.Command("route", "add", "0.0.0.0", "mask", "0.0.0.0", gateway).Run()
}
