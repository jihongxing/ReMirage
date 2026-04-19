//go:build darwin

package killswitch

import (
	"fmt"
	"os/exec"
	"strings"
)

type darwinPlatform struct{}

func newPlatform() Platform { return &darwinPlatform{} }

func (p *darwinPlatform) GetDefaultGateway() (string, string, error) {
	out, err := exec.Command("route", "-n", "get", "default").Output()
	if err != nil {
		return "", "", fmt.Errorf("route get default: %w", err)
	}
	var gw, iface string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "gateway:") {
			gw = strings.TrimSpace(strings.TrimPrefix(line, "gateway:"))
		}
		if strings.HasPrefix(line, "interface:") {
			iface = strings.TrimSpace(strings.TrimPrefix(line, "interface:"))
		}
	}
	if gw == "" {
		return "", "", fmt.Errorf("no default gateway found")
	}
	return gw, iface, nil
}

func (p *darwinPlatform) DeleteDefaultRoute() error {
	return exec.Command("route", "delete", "default").Run()
}

func (p *darwinPlatform) AddDefaultRoute(tunName string) error {
	return exec.Command("route", "add", "default", "-interface", tunName).Run()
}

func (p *darwinPlatform) AddHostRoute(ip, gateway, iface string) error {
	return exec.Command("route", "add", "-host", ip, gateway).Run()
}

func (p *darwinPlatform) DeleteHostRoute(ip string) error {
	return exec.Command("route", "delete", "-host", ip).Run()
}

func (p *darwinPlatform) RestoreDefaultRoute(gateway, iface string) error {
	return exec.Command("route", "add", "default", gateway).Run()
}
