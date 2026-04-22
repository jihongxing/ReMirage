//go:build windows

package daemon

import (
	"fmt"
	"time"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

type windowsServiceBackend struct{}

func newServiceBackend() ServiceBackend {
	return &windowsServiceBackend{}
}

func (b *windowsServiceBackend) Install(name, execPath, configDir, logDir string) error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connect to service manager: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(name)
	if err == nil {
		s.Close()
		return fmt.Errorf("service %s already exists", name)
	}

	cfg := mgr.Config{
		DisplayName: "Phantom Client VPN Service",
		Description: "Phantom Client background VPN service with auto-recovery",
		StartType:   mgr.StartAutomatic,
	}

	s, err = m.CreateService(name, execPath, cfg,
		"--daemon",
		"--config-dir", configDir,
		"--log-dir", logDir,
	)
	if err != nil {
		return fmt.Errorf("create service: %w", err)
	}
	defer s.Close()

	// Configure recovery: restart on failure
	recoveryActions := []mgr.RecoveryAction{
		{Type: mgr.ServiceRestart, Delay: 5 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 10 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 30 * time.Second},
	}
	if err := s.SetRecoveryActions(recoveryActions, 86400); err != nil {
		return fmt.Errorf("set recovery actions: %w", err)
	}

	return nil
}

func (b *windowsServiceBackend) Uninstall(name string) error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connect to service manager: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(name)
	if err != nil {
		return fmt.Errorf("open service: %w", err)
	}
	defer s.Close()

	// Stop if running
	_, _ = s.Control(svc.Stop)

	if err := s.Delete(); err != nil {
		return fmt.Errorf("delete service: %w", err)
	}
	return nil
}

func (b *windowsServiceBackend) Start(name string) error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connect to service manager: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(name)
	if err != nil {
		return fmt.Errorf("open service: %w", err)
	}
	defer s.Close()

	return s.Start()
}

func (b *windowsServiceBackend) Stop(name string) error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connect to service manager: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(name)
	if err != nil {
		return fmt.Errorf("open service: %w", err)
	}
	defer s.Close()

	_, err = s.Control(svc.Stop)
	return err
}

func (b *windowsServiceBackend) Status(name string) (ServiceStatus, error) {
	m, err := mgr.Connect()
	if err != nil {
		return StatusUnknown, fmt.Errorf("connect to service manager: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(name)
	if err != nil {
		return StatusNotFound, nil
	}
	defer s.Close()

	status, err := s.Query()
	if err != nil {
		return StatusUnknown, fmt.Errorf("query service: %w", err)
	}

	switch status.State {
	case svc.Running:
		return StatusRunning, nil
	case svc.Stopped:
		return StatusStopped, nil
	default:
		return StatusUnknown, nil
	}
}
