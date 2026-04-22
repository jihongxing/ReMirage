package entitlement

import (
	"context"
	"math"
	"sync"
	"sync/atomic"
	"time"
)

// ServiceClass 服务等级
type ServiceClass string

const (
	ClassStandard ServiceClass = "standard"
	ClassPlatinum ServiceClass = "platinum"
	ClassDiamond  ServiceClass = "diamond"
)

// Entitlement 订阅权益
type Entitlement struct {
	ExpiresAt      time.Time    `json:"expires_at"`
	QuotaRemaining int64        `json:"quota_remaining_bytes"`
	ServiceClass   ServiceClass `json:"service_class"`
	Banned         bool         `json:"banned"`
	FetchedAt      time.Time    `json:"fetched_at"`
}

// Equal returns true if two Entitlements are semantically equal.
func (e *Entitlement) Equal(other *Entitlement) bool {
	if e == nil && other == nil {
		return true
	}
	if e == nil || other == nil {
		return false
	}
	return e.ExpiresAt.Equal(other.ExpiresAt) &&
		e.QuotaRemaining == other.QuotaRemaining &&
		e.ServiceClass == other.ServiceClass &&
		e.Banned == other.Banned &&
		e.FetchedAt.Equal(other.FetchedAt)
}

// ExponentialBackoff 指数退避计算器（本地副本，避免循环导入）
type ExponentialBackoff struct {
	Base      time.Duration
	Max       time.Duration
	FailCount int
}

// NewExponentialBackoff creates a new ExponentialBackoff.
func NewExponentialBackoff(base, max time.Duration) *ExponentialBackoff {
	return &ExponentialBackoff{Base: base, Max: max}
}

// Next returns the current backoff delay: min(base × 2^FailCount, max).
func (eb *ExponentialBackoff) Next() time.Duration {
	if eb.FailCount <= 0 {
		return eb.Base
	}
	delay := time.Duration(float64(eb.Base) * math.Pow(2, float64(eb.FailCount)))
	if delay > eb.Max || delay <= 0 {
		return eb.Max
	}
	return delay
}

// Record increments the fail count and returns the new backoff delay.
func (eb *ExponentialBackoff) Record() time.Duration {
	eb.FailCount++
	return eb.Next()
}

// Reset resets the fail count to zero.
func (eb *ExponentialBackoff) Reset() {
	eb.FailCount = 0
}

// EntitlementFetcher is a function that fetches Entitlement from the OS control plane.
// Abstracted for testability.
type EntitlementFetcher func(ctx context.Context) (*Entitlement, error)

const (
	defaultEntitlementInterval = 10 * time.Minute
	defaultEntBackoffBase      = 30 * time.Second
	defaultEntBackoffMax       = 10 * time.Minute
)

// EntitlementConfig holds configuration for EntitlementManager.
type EntitlementConfig struct {
	Fetcher     EntitlementFetcher           // required: function to fetch entitlement
	Interval    time.Duration                // refresh interval (default 10min)
	OnChange    func(old, new_ *Entitlement) // optional: state change callback
	OnBanned    func()                       // optional: banned callback
	OnReadOnly  func()                       // optional: called when grace window expires → read-only mode
	OnRecovered func()                       // optional: called when control plane recovers from read-only
	Grace       *GraceWindow                 // optional: grace window for offline tolerance
}

// EntitlementManager 订阅托管管理器
type EntitlementManager struct {
	fetcher     EntitlementFetcher
	interval    time.Duration
	onChange    func(old, new_ *Entitlement)
	onBanned    func()
	onReadOnly  func()
	onRecovered func()
	grace       *GraceWindow

	current  atomic.Pointer[Entitlement]
	readOnly atomic.Bool // true when grace window expired and control plane unreachable
	backoff  *ExponentialBackoff
	stopCh   chan struct{}
	once     sync.Once
}

// NewEntitlementManager creates a new EntitlementManager.
func NewEntitlementManager(cfg EntitlementConfig) *EntitlementManager {
	interval := cfg.Interval
	if interval <= 0 {
		interval = defaultEntitlementInterval
	}
	return &EntitlementManager{
		fetcher:     cfg.Fetcher,
		interval:    interval,
		onChange:    cfg.OnChange,
		onBanned:    cfg.OnBanned,
		onReadOnly:  cfg.OnReadOnly,
		onRecovered: cfg.OnRecovered,
		grace:       cfg.Grace,
		backoff:     NewExponentialBackoff(defaultEntBackoffBase, defaultEntBackoffMax),
		stopCh:      make(chan struct{}),
	}
}

// Start launches the background refresh goroutine. Blocks until ctx is cancelled or Stop is called.
func (em *EntitlementManager) Start(ctx context.Context) {
	timer := time.NewTimer(0) // fire immediately on start
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-em.stopCh:
			return
		case <-timer.C:
			em.FetchOnce(ctx)

			if em.backoff.FailCount > 0 {
				timer.Reset(em.backoff.Next())
			} else {
				timer.Reset(em.interval)
			}
		}
	}
}

// FetchOnce performs a single fetch and update cycle.
// Integrates grace window checking: on failure, checks if grace expired → read-only mode.
// On success after read-only, recovers to normal mode.
func (em *EntitlementManager) FetchOnce(ctx context.Context) {
	ent, err := em.fetcher(ctx)
	if err != nil {
		em.backoff.Record()
		// Check grace window: if expired, enter read-only mode
		if em.grace != nil && !em.grace.IsWithinGrace() && !em.readOnly.Load() {
			em.readOnly.Store(true)
			if em.onReadOnly != nil {
				em.onReadOnly()
			}
		}
		return
	}

	// Success: record in grace window
	if em.grace != nil {
		em.grace.RecordSuccess(ent)
	}

	// If we were in read-only mode, recover
	wasReadOnly := em.readOnly.Swap(false)
	if wasReadOnly && em.onRecovered != nil {
		em.onRecovered()
	}

	old := em.current.Load()
	em.current.Store(ent)
	em.backoff.Reset()

	// Fire onChange if state changed (skip if in read-only — but we just recovered)
	if em.onChange != nil && !ent.Equal(old) {
		em.onChange(old, ent)
	}

	// Fire onBanned if banned
	if ent.Banned && em.onBanned != nil {
		em.onBanned()
	}
}

// IsReadOnly returns true if the manager is in read-only mode (grace window expired).
func (em *EntitlementManager) IsReadOnly() bool {
	return em.readOnly.Load()
}

// Current returns the most recently fetched Entitlement.
func (em *EntitlementManager) Current() *Entitlement {
	return em.current.Load()
}

// Stop signals the refresh goroutine to stop.
func (em *EntitlementManager) Stop() {
	em.once.Do(func() {
		close(em.stopCh)
	})
}
