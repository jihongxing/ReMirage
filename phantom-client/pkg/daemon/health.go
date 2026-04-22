// Package daemon provides background service management and health monitoring.
package daemon

import (
	"context"
	"sync"
	"time"
)

// HealthCheck represents the result of a single health check.
type HealthCheck struct {
	Name    string
	Healthy bool
	Detail  string
}

// CheckFunc is a function that performs a health check.
type CheckFunc func(ctx context.Context) HealthCheck

// RepairFunc is called when a repair action is attempted.
type RepairFunc func(check string, err error)

// HealthGuardian performs periodic health checks and attempts self-repair.
type HealthGuardian struct {
	interval time.Duration
	checks   []namedCheck
	onRepair RepairFunc
	mu       sync.RWMutex
	stopCh   chan struct{}
}

type namedCheck struct {
	name string
	fn   CheckFunc
}

// NewHealthGuardian creates a HealthGuardian with the given check interval.
// Default interval is 30 seconds if zero is passed.
func NewHealthGuardian(interval time.Duration) *HealthGuardian {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	return &HealthGuardian{
		interval: interval,
		stopCh:   make(chan struct{}),
	}
}

// SetOnRepair sets the callback invoked when a repair action is attempted.
func (hg *HealthGuardian) SetOnRepair(fn RepairFunc) {
	hg.mu.Lock()
	defer hg.mu.Unlock()
	hg.onRepair = fn
}

// Register adds a named health check function.
func (hg *HealthGuardian) Register(name string, fn CheckFunc) {
	hg.mu.Lock()
	defer hg.mu.Unlock()
	hg.checks = append(hg.checks, namedCheck{name: name, fn: fn})
}

// Start begins periodic health checking. Blocks until ctx is cancelled.
func (hg *HealthGuardian) Start(ctx context.Context) {
	ticker := time.NewTicker(hg.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-hg.stopCh:
			return
		case <-ticker.C:
			hg.runChecks(ctx)
		}
	}
}

// Stop signals the health guardian to stop.
func (hg *HealthGuardian) Stop() {
	select {
	case hg.stopCh <- struct{}{}:
	default:
	}
}

// RunOnce executes all health checks once and returns results.
func (hg *HealthGuardian) RunOnce(ctx context.Context) []HealthCheck {
	return hg.runChecks(ctx)
}

func (hg *HealthGuardian) runChecks(ctx context.Context) []HealthCheck {
	hg.mu.RLock()
	checks := make([]namedCheck, len(hg.checks))
	copy(checks, hg.checks)
	repairFn := hg.onRepair
	hg.mu.RUnlock()

	var results []HealthCheck
	for _, c := range checks {
		result := c.fn(ctx)
		results = append(results, result)
		if !result.Healthy && repairFn != nil {
			repairFn(result.Name, nil)
		}
	}
	return results
}
