package killswitch

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
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
	activatedAt   time.Time
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
		// Rollback step 2: restore original default route
		_ = ks.platform.RestoreDefaultRoute(origGW, origIface)
		return fmt.Errorf("add TUN default route: %w", err)
	}

	// Step 4: Add /32 host route for gateway
	if err := ks.platform.AddHostRoute(gatewayIP, origGW, origIface); err != nil {
		// Rollback step 3: delete TUN default route
		_ = ks.platform.DeleteDefaultRoute()
		// Rollback step 2: restore original default route
		_ = ks.platform.RestoreDefaultRoute(origGW, origIface)
		return fmt.Errorf("add host route: %w", err)
	}

	ks.gatewayIP = gatewayIP
	ks.activated = true
	ks.activatedAt = time.Now()
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

// RouteState captures the routing snapshot for crash recovery.
type RouteState struct {
	OriginalGW    string    `json:"original_gw"`
	OriginalIface string    `json:"original_iface"`
	CurrentGWIP   string    `json:"current_gw_ip"`
	TUNName       string    `json:"tun_name"`
	ActivatedAt   time.Time `json:"activated_at"`
}

// GetRouteState returns the current route state snapshot (caller must hold lock or call externally).
func (ks *KillSwitch) GetRouteState() *RouteState {
	ks.mu.Lock()
	defer ks.mu.Unlock()
	if !ks.activated {
		return nil
	}
	return &RouteState{
		OriginalGW:    ks.originalGW,
		OriginalIface: ks.originalIface,
		CurrentGWIP:   ks.gatewayIP,
		TUNName:       ks.tunName,
		ActivatedAt:   ks.activatedAt,
	}
}

// PersistState saves the current route state to a JSON file using atomic write.
func (ks *KillSwitch) PersistState(path string) error {
	state := ks.GetRouteState()
	if state == nil {
		return fmt.Errorf("kill switch not activated, nothing to persist")
	}
	return SaveRouteState(path, state)
}

// SaveRouteState writes a RouteState to the given path using atomic write (temp + rename).
func SaveRouteState(path string, state *RouteState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("route state marshal: %w", err)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("route state mkdir: %w", err)
	}

	tmp, err := os.CreateTemp(dir, ".route-state-*.tmp")
	if err != nil {
		return fmt.Errorf("route state temp: %w", err)
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("route state write: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("route state close: %w", err)
	}

	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("route state rename: %w", err)
	}
	return nil
}

// LoadRouteState reads a RouteState from the given JSON file path.
func LoadRouteState(path string) (*RouteState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("route state read: %w", err)
	}
	var state RouteState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("route state unmarshal: %w", err)
	}
	return &state, nil
}

// NewPlatform returns the platform-specific route operations implementation.
// Exported for use by daemon mode stale route cleanup.
func NewPlatform() Platform {
	return newPlatform()
}

// CleanupStaleRoutes detects and cleans up residual routes from a previous crash.
// It reads the persisted route state, checks for stale TUN device and /32 host routes,
// and restores the original routing configuration.
func CleanupStaleRoutes(path string, p Platform) error {
	state, err := LoadRouteState(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No stale state, nothing to clean
		}
		return fmt.Errorf("cleanup load state: %w", err)
	}

	// Attempt to clean up residual routes
	var cleanupErrs []error

	// Remove /32 host route for the gateway
	if state.CurrentGWIP != "" {
		if err := p.DeleteHostRoute(state.CurrentGWIP); err != nil {
			cleanupErrs = append(cleanupErrs, fmt.Errorf("delete stale host route %s: %w", state.CurrentGWIP, err))
		}
	}

	// Remove TUN default route
	if err := p.DeleteDefaultRoute(); err != nil {
		// Non-fatal: TUN default may already be gone
		cleanupErrs = append(cleanupErrs, fmt.Errorf("delete stale TUN default: %w", err))
	}

	// Restore original default route
	if state.OriginalGW != "" {
		if err := p.RestoreDefaultRoute(state.OriginalGW, state.OriginalIface); err != nil {
			cleanupErrs = append(cleanupErrs, fmt.Errorf("restore original route: %w", err))
		}
	}

	// Remove the stale state file
	os.Remove(path)

	if len(cleanupErrs) > 0 {
		return fmt.Errorf("cleanup stale routes (partial): %v", cleanupErrs)
	}
	return nil
}
