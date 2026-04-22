package daemon

import "fmt"

// ServiceStatus represents the current state of the daemon service.
type ServiceStatus int

const (
	StatusUnknown ServiceStatus = iota
	StatusRunning
	StatusStopped
	StatusNotFound
)

func (s ServiceStatus) String() string {
	switch s {
	case StatusRunning:
		return "running"
	case StatusStopped:
		return "stopped"
	case StatusNotFound:
		return "not_found"
	default:
		return "unknown"
	}
}

// ServiceBackend abstracts platform-specific service management operations.
type ServiceBackend interface {
	Install(name, execPath, configDir, logDir string) error
	Uninstall(name string) error
	Start(name string) error
	Stop(name string) error
	Status(name string) (ServiceStatus, error)
}

// DaemonManager manages the phantom-client system service lifecycle.
type DaemonManager struct {
	serviceName string
	execPath    string
	configDir   string
	logDir      string
	backend     ServiceBackend
}

// NewDaemonManager creates a DaemonManager with the platform-specific backend.
func NewDaemonManager(name, execPath, configDir, logDir string) *DaemonManager {
	return &DaemonManager{
		serviceName: name,
		execPath:    execPath,
		configDir:   configDir,
		logDir:      logDir,
		backend:     newServiceBackend(),
	}
}

// NewDaemonManagerWithBackend creates a DaemonManager with a custom backend (for testing).
func NewDaemonManagerWithBackend(name, execPath, configDir, logDir string, backend ServiceBackend) *DaemonManager {
	return &DaemonManager{
		serviceName: name,
		execPath:    execPath,
		configDir:   configDir,
		logDir:      logDir,
		backend:     backend,
	}
}

// Install registers the service with the operating system.
func (dm *DaemonManager) Install() error {
	if dm.backend == nil {
		return fmt.Errorf("no service backend available")
	}
	return dm.backend.Install(dm.serviceName, dm.execPath, dm.configDir, dm.logDir)
}

// Uninstall removes the service registration from the operating system.
func (dm *DaemonManager) Uninstall() error {
	if dm.backend == nil {
		return fmt.Errorf("no service backend available")
	}
	return dm.backend.Uninstall(dm.serviceName)
}

// Start starts the registered service.
func (dm *DaemonManager) Start() error {
	if dm.backend == nil {
		return fmt.Errorf("no service backend available")
	}
	return dm.backend.Start(dm.serviceName)
}

// Stop stops the running service.
func (dm *DaemonManager) Stop() error {
	if dm.backend == nil {
		return fmt.Errorf("no service backend available")
	}
	return dm.backend.Stop(dm.serviceName)
}

// Status returns the current service status.
func (dm *DaemonManager) Status() (ServiceStatus, error) {
	if dm.backend == nil {
		return StatusUnknown, fmt.Errorf("no service backend available")
	}
	return dm.backend.Status(dm.serviceName)
}
