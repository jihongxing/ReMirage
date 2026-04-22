//go:build darwin

package daemon

import "fmt"

// darwinServiceBackend is a stub for macOS (launchd).
type darwinServiceBackend struct{}

func newServiceBackend() ServiceBackend {
	return &darwinServiceBackend{}
}

func (b *darwinServiceBackend) Install(name, execPath, configDir, logDir string) error {
	return fmt.Errorf("darwin service management not yet implemented")
}

func (b *darwinServiceBackend) Uninstall(name string) error {
	return fmt.Errorf("darwin service management not yet implemented")
}

func (b *darwinServiceBackend) Start(name string) error {
	return fmt.Errorf("darwin service management not yet implemented")
}

func (b *darwinServiceBackend) Stop(name string) error {
	return fmt.Errorf("darwin service management not yet implemented")
}

func (b *darwinServiceBackend) Status(name string) (ServiceStatus, error) {
	return StatusUnknown, fmt.Errorf("darwin service management not yet implemented")
}
