package killswitch

import (
	"fmt"
	"sync"
)

// Platform abstracts OS-specific route operations.
type Platform interface {
	GetDefaultGateway() (ip, iface string, err error)
	DeleteDefaultRoute() error
	AddDefaultRoute(tunName string) error
	AddHostRoute(ip, gateway, iface string) error
	DeleteHostRoute(ip string) error
	RestoreDefaultRoute(gateway, iface string) error
}

// KillSwitch implements fail-closed routing hijack.
type KillSwitch struct {
	originalGW    string
	originalIface string
	tunName       string
	gatewayIP     string
	activated     bool
	mu            sync.Mutex
	platform      Platform
}

// NewKillSwitch creates a new kill switch for the given TUN device.
func NewKillSwitch(tunName string) *KillSwitch {
	return &KillSwitch{
		tunName:  tunName,
		platform: newPlatform(),
	}
}

// NewKillSwitchWithPlatform creates a kill switch with a custom platform (for testing).
func NewKillSwitchWithPlatform(tunName string, p Platform) *KillSwitch {
	return &KillSwitch{
		tunName:  tunName,
		platform: p,
	}
}

// Activate performs the 4-step route hijack sequence:
// 1. Backup default gateway
// 2. Delete default route
// 3. Add TUN as default route
// 4. Add /32 host route for gateway IP via original gateway
func (ks *KillSwitch) Activate(gatewayIP string) error {
	ks.mu.Lock()
	defer ks.mu.Unlock()

	if ks.activated {
		return fmt.Errorf("kill switch already activated")
	}

	// Step 1: Backup
	origGW, origIface, err := ks.platform.GetDefaultGateway()
	if err != nil {
		return fmt.Errorf("get default gateway: %w", err)
	}
	ks.originalGW = origGW
	ks.originalIface = origIface

	// Step 2: Delete default route
	if err := ks.platform.DeleteDefaultRoute(); err != nil {
		return fmt.Errorf("delete default route: %w", err)
	}

	// Step 3: Add TUN as default route
	if err := ks.platform.AddDefaultRoute(ks.tunName); err != nil {
		// Rollback: restore original
		_ = ks.platform.RestoreDefaultRoute(origGW, origIface)
		return fmt.Errorf("add TUN default route: %w", err)
	}

	// Step 4: Add /32 host route for gateway
	if err := ks.platform.AddHostRoute(gatewayIP, origGW, origIface); err != nil {
		// Rollback
		_ = ks.platform.DeleteDefaultRoute()
		_ = ks.platform.RestoreDefaultRoute(origGW, origIface)
		return fmt.Errorf("add host route: %w", err)
	}

	ks.gatewayIP = gatewayIP
	ks.activated = true
	return nil
}

// UpdateGatewayRoute atomically updates the /32 host route (add new, then delete old).
func (ks *KillSwitch) UpdateGatewayRoute(newGatewayIP string) error {
	ks.mu.Lock()
	defer ks.mu.Unlock()

	if !ks.activated {
		return fmt.Errorf("kill switch not activated")
	}

	oldIP := ks.gatewayIP

	// Add new route first (no gap)
	if err := ks.platform.AddHostRoute(newGatewayIP, ks.originalGW, ks.originalIface); err != nil {
		return fmt.Errorf("add new host route: %w", err)
	}

	// Then delete old route
	if err := ks.platform.DeleteHostRoute(oldIP); err != nil {
		// Non-fatal: old route remains, new route is active
		// Log warning in production
	}

	ks.gatewayIP = newGatewayIP
	return nil
}

// Deactivate restores original routing.
func (ks *KillSwitch) Deactivate() error {
	ks.mu.Lock()
	defer ks.mu.Unlock()

	if !ks.activated {
		return nil
	}

	var errs []error

	// Remove /32 host route
	if err := ks.platform.DeleteHostRoute(ks.gatewayIP); err != nil {
		errs = append(errs, fmt.Errorf("delete host route: %w", err))
	}

	// Remove TUN default route
	if err := ks.platform.DeleteDefaultRoute(); err != nil {
		errs = append(errs, fmt.Errorf("delete TUN default: %w", err))
	}

	// Restore original default route
	if err := ks.platform.RestoreDefaultRoute(ks.originalGW, ks.originalIface); err != nil {
		errs = append(errs, fmt.Errorf("restore default: %w", err))
	}

	ks.activated = false

	if len(errs) > 0 {
		return fmt.Errorf("deactivate errors (manual recovery may be needed): %v", errs)
	}
	return nil
}

// PreAddHostRoute adds a /32 host route for the new gateway without removing the old one.
// Used as step 1 of transactional gateway switch.
func (ks *KillSwitch) PreAddHostRoute(newGatewayIP string) error {
	ks.mu.Lock()
	defer ks.mu.Unlock()
	if !ks.activated {
		return fmt.Errorf("kill switch not activated")
	}
	return ks.platform.AddHostRoute(newGatewayIP, ks.originalGW, ks.originalIface)
}

// CommitSwitch deletes the old gateway route and updates internal state.
// Used as step 3 of transactional gateway switch.
func (ks *KillSwitch) CommitSwitch(oldGatewayIP, newGatewayIP string) error {
	ks.mu.Lock()
	defer ks.mu.Unlock()
	_ = ks.platform.DeleteHostRoute(oldGatewayIP)
	ks.gatewayIP = newGatewayIP
	return nil
}

// RollbackPreAdd removes a pre-added route on transaction failure.
func (ks *KillSwitch) RollbackPreAdd(newGatewayIP string) error {
	ks.mu.Lock()
	defer ks.mu.Unlock()
	return ks.platform.DeleteHostRoute(newGatewayIP)
}

// IsActivated returns whether the kill switch is active.
func (ks *KillSwitch) IsActivated() bool {
	ks.mu.Lock()
	defer ks.mu.Unlock()
	return ks.activated
}
