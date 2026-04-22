//go:build linux

package daemon

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const systemdUnitPath = "/etc/systemd/system"

type linuxServiceBackend struct{}

func newServiceBackend() ServiceBackend {
	return &linuxServiceBackend{}
}

func (b *linuxServiceBackend) Install(name, execPath, configDir, logDir string) error {
	unit := generateSystemdUnit(name, execPath, configDir, logDir)
	unitFile := filepath.Join(systemdUnitPath, name+".service")

	if err := os.WriteFile(unitFile, []byte(unit), 0644); err != nil {
		return fmt.Errorf("write systemd unit: %w", err)
	}

	// Reload systemd daemon
	if err := exec.Command("systemctl", "daemon-reload").Run(); err != nil {
		return fmt.Errorf("systemctl daemon-reload: %w", err)
	}

	// Enable service for auto-start
	if err := exec.Command("systemctl", "enable", name).Run(); err != nil {
		return fmt.Errorf("systemctl enable: %w", err)
	}

	return nil
}

func (b *linuxServiceBackend) Uninstall(name string) error {
	_ = exec.Command("systemctl", "stop", name).Run()
	_ = exec.Command("systemctl", "disable", name).Run()

	unitFile := filepath.Join(systemdUnitPath, name+".service")
	if err := os.Remove(unitFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove systemd unit: %w", err)
	}

	return exec.Command("systemctl", "daemon-reload").Run()
}

func (b *linuxServiceBackend) Start(name string) error {
	return exec.Command("systemctl", "start", name).Run()
}

func (b *linuxServiceBackend) Stop(name string) error {
	return exec.Command("systemctl", "stop", name).Run()
}

func (b *linuxServiceBackend) Status(name string) (ServiceStatus, error) {
	out, err := exec.Command("systemctl", "is-active", name).Output()
	if err != nil {
		// is-active returns non-zero for inactive/not-found
		status := strings.TrimSpace(string(out))
		if status == "inactive" || status == "failed" {
			return StatusStopped, nil
		}
		return StatusNotFound, nil
	}
	status := strings.TrimSpace(string(out))
	if status == "active" {
		return StatusRunning, nil
	}
	return StatusStopped, nil
}

func generateSystemdUnit(name, execPath, configDir, logDir string) string {
	return fmt.Sprintf(`[Unit]
Description=Phantom Client VPN Service
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=%s --daemon --config-dir %s --log-dir %s
Restart=on-failure
RestartSec=5s
LimitNOFILE=65536
Environment=PHANTOM_CONFIG_DIR=%s
Environment=PHANTOM_LOG_DIR=%s
StandardOutput=journal
StandardError=journal
SyslogIdentifier=%s

[Install]
WantedBy=multi-user.target
`, execPath, configDir, logDir, configDir, logDir, name)
}
