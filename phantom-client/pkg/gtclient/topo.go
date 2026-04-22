package gtclient

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// RouteTableResponse OS 控制面返回的路由表
type RouteTableResponse struct {
	Gateways    []GatewayNode `json:"gateways"`
	Version     uint64        `json:"version"`
	PublishedAt time.Time     `json:"published_at"`
	Signature   []byte        `json:"signature"`
}

// GatewayNode 带优先级和区域的网关节点
type GatewayNode struct {
	IP       string `json:"ip"`
	Port     int    `json:"port"`
	Priority uint8  `json:"priority"`
	Region   string `json:"region"`
	CellID   string `json:"cell_id"`
}

// hmacBody is the canonical form used for HMAC computation.
// It includes only gateways, version, and published_at (NOT signature).
type hmacBody struct {
	Gateways    []GatewayNode `json:"gateways"`
	Version     uint64        `json:"version"`
	PublishedAt time.Time     `json:"published_at"`
}

// ComputeHMAC computes HMAC-SHA256 over the canonical body of a RouteTableResponse.
func ComputeHMAC(resp *RouteTableResponse, key []byte) ([]byte, error) {
	body := hmacBody{
		Gateways:    resp.Gateways,
		Version:     resp.Version,
		PublishedAt: resp.PublishedAt,
	}
	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal hmac body: %w", err)
	}
	mac := hmac.New(sha256.New, key)
	mac.Write(data)
	return mac.Sum(nil), nil
}

// --- TopoVerifier ---

const (
	maxSigFailBeforePause = 3
	pauseDuration         = 30 * time.Minute
)

// TopoVerifier 路由表签名校验器
type TopoVerifier struct {
	hmacKey        []byte
	currentVersion uint64
	currentPubTime time.Time
	sigFailCount   atomic.Int32
	pauseUntil     atomic.Int64 // Unix timestamp
	mu             sync.Mutex   // protects currentVersion and currentPubTime
}

// NewTopoVerifier creates a TopoVerifier using the first 32 bytes of psk as HMAC key.
func NewTopoVerifier(psk []byte) *TopoVerifier {
	key := make([]byte, 32)
	copy(key, psk) // if psk < 32 bytes, remaining bytes are zero
	return &TopoVerifier{
		hmacKey: key,
	}
}

// Verify validates a RouteTableResponse:
// 1. HMAC-SHA256 signature
// 2. Version monotonically increasing
// 3. PublishedAt anti-rollback
// 4. Gateway list non-empty
func (tv *TopoVerifier) Verify(resp *RouteTableResponse) error {
	// Check pause state
	if tv.IsPaused() {
		return fmt.Errorf("topo verifier paused due to repeated signature failures")
	}

	// 1. Gateway list non-empty
	if len(resp.Gateways) == 0 {
		return fmt.Errorf("empty gateway list")
	}

	// 2. HMAC-SHA256 signature verification
	expected, err := ComputeHMAC(resp, tv.hmacKey)
	if err != nil {
		return fmt.Errorf("compute hmac: %w", err)
	}
	if !hmac.Equal(expected, resp.Signature) {
		count := tv.sigFailCount.Add(1)
		if count >= maxSigFailBeforePause {
			tv.pauseUntil.Store(time.Now().Add(pauseDuration).Unix())
		}
		return fmt.Errorf("signature verification failed")
	}

	// Signature OK — reset fail counter
	tv.sigFailCount.Store(0)

	tv.mu.Lock()
	defer tv.mu.Unlock()

	// 3. Version monotonically increasing
	if resp.Version <= tv.currentVersion {
		return fmt.Errorf("version %d not greater than current %d", resp.Version, tv.currentVersion)
	}

	// 4. PublishedAt anti-rollback
	if !resp.PublishedAt.After(tv.currentPubTime) {
		return fmt.Errorf("published_at not after current")
	}

	// All checks passed — update state
	tv.currentVersion = resp.Version
	tv.currentPubTime = resp.PublishedAt

	return nil
}

// IsPaused returns true if the verifier is paused due to consecutive signature failures.
func (tv *TopoVerifier) IsPaused() bool {
	pauseTS := tv.pauseUntil.Load()
	if pauseTS == 0 {
		return false
	}
	return time.Now().Unix() < pauseTS
}

// --- RuntimeTopology ---

// RuntimeTopology 运行时拓扑池（替代现有 RouteTable，线程安全）
type RuntimeTopology struct {
	nodes       []GatewayNode
	version     uint64
	publishedAt time.Time
	updatedAt   time.Time
	mu          sync.RWMutex
}

// Update writes a new set of nodes, version, and publish time into the topology.
func (rt *RuntimeTopology) Update(nodes []GatewayNode, version uint64, pubTime time.Time) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.nodes = make([]GatewayNode, len(nodes))
	copy(rt.nodes, nodes)
	rt.version = version
	rt.publishedAt = pubTime
	rt.updatedAt = time.Now()
}

// NextByPriority returns nodes sorted by priority ascending, excluding the given IP.
// Returns the first (highest priority) node not matching exclude.
func (rt *RuntimeTopology) NextByPriority(exclude string) (GatewayNode, error) {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	// Make a sorted copy
	sorted := make([]GatewayNode, 0, len(rt.nodes))
	for _, n := range rt.nodes {
		if n.IP != exclude {
			sorted = append(sorted, n)
		}
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Priority < sorted[j].Priority
	})

	if len(sorted) == 0 {
		return GatewayNode{}, fmt.Errorf("no available gateway")
	}
	return sorted[0], nil
}

// AllByPriority returns all nodes sorted by priority ascending, excluding the given IP.
func (rt *RuntimeTopology) AllByPriority(exclude string) []GatewayNode {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	sorted := make([]GatewayNode, 0, len(rt.nodes))
	for _, n := range rt.nodes {
		if n.IP != exclude {
			sorted = append(sorted, n)
		}
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Priority < sorted[j].Priority
	})
	return sorted
}

// Count returns the number of nodes.
func (rt *RuntimeTopology) Count() int {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	return len(rt.nodes)
}

// Version returns the current topology version.
func (rt *RuntimeTopology) Version() uint64 {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	return rt.version
}

// IsEmpty returns true if the topology has no nodes.
func (rt *RuntimeTopology) IsEmpty() bool {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	return len(rt.nodes) == 0
}

// Snapshot returns a copy of the current topology state (for testing/inspection).
func (rt *RuntimeTopology) Snapshot() (nodes []GatewayNode, version uint64, updatedAt time.Time) {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	cp := make([]GatewayNode, len(rt.nodes))
	copy(cp, rt.nodes)
	return cp, rt.version, rt.updatedAt
}

// --- TopoRefresher ---

const (
	defaultRefreshInterval = 5 * time.Minute
	defaultBackoffBase     = 30 * time.Second
	defaultBackoffMax      = 30 * time.Minute
	alertAfterFailures     = 3
)

// TopoFetcher is a function that fetches a RouteTableResponse from the OS control plane.
// Abstracted for testability (production uses HTTP, tests use a mock).
type TopoFetcher func(ctx context.Context) (*RouteTableResponse, error)

// TopoRefresherConfig holds configuration for TopoRefresher.
type TopoRefresherConfig struct {
	Fetcher  TopoFetcher      // required: function to fetch route table
	Verifier *TopoVerifier    // required: signature verifier
	Topo     *RuntimeTopology // required: target topology to update
	Interval time.Duration    // refresh interval (default 5min)
	OnAlert  func(msg string) // optional: alert callback on consecutive failures
	Cache    *TopoCache       // optional: local persistence cache
}

// TopoRefresher 拓扑周期刷新器
type TopoRefresher struct {
	fetcher  TopoFetcher
	verifier *TopoVerifier
	topo     *RuntimeTopology
	interval time.Duration
	onAlert  func(msg string)
	cache    *TopoCache

	backoff   *ExponentialBackoff
	failCount atomic.Int32
	stopCh    chan struct{}
	stopMu    sync.Mutex
}

// NewTopoRefresher creates a new TopoRefresher.
func NewTopoRefresher(cfg TopoRefresherConfig) *TopoRefresher {
	interval := cfg.Interval
	if interval <= 0 {
		interval = defaultRefreshInterval
	}
	return &TopoRefresher{
		fetcher:  cfg.Fetcher,
		verifier: cfg.Verifier,
		topo:     cfg.Topo,
		interval: interval,
		onAlert:  cfg.OnAlert,
		cache:    cfg.Cache,
		backoff:  NewExponentialBackoff(defaultBackoffBase, defaultBackoffMax),
		stopCh:   make(chan struct{}),
	}
}

// Start launches the background refresh goroutine. Blocks until ctx is cancelled or Stop is called.
// Safe to call again after Stop() — creates a fresh stop channel for the new run.
func (tr *TopoRefresher) Start(ctx context.Context) {
	tr.stopMu.Lock()
	tr.stopCh = make(chan struct{})
	stopCh := tr.stopCh
	tr.stopMu.Unlock()

	timer := time.NewTimer(tr.interval)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-stopCh:
			return
		case <-timer.C:
			tr.PullOnce(ctx)

			// Determine next interval: normal or backoff
			if tr.failCount.Load() > 0 {
				timer.Reset(tr.backoff.Next())
			} else {
				timer.Reset(tr.interval)
			}
		}
	}
}

// PullOnce performs a single fetch → verify → update cycle.
// On failure, existing topology is preserved, backoff incremented, and alert emitted after 3 consecutive failures.
func (tr *TopoRefresher) PullOnce(ctx context.Context) error {
	resp, err := tr.fetcher(ctx)
	if err != nil {
		return tr.handleFailure(fmt.Sprintf("fetch failed: %v", err))
	}

	if err := tr.verifier.Verify(resp); err != nil {
		return tr.handleFailure(fmt.Sprintf("verify failed: %v", err))
	}

	// Success: update topology, reset backoff
	tr.topo.Update(resp.Gateways, resp.Version, resp.PublishedAt)
	tr.failCount.Store(0)
	tr.backoff.Reset()

	// 持久化到本地缓存
	if tr.cache != nil {
		if err := tr.cache.Save(resp); err != nil {
			log.Printf("[TopoRefresher] ⚠️ 缓存写入失败: %v", err)
		}
	}

	return nil
}

// handleFailure increments fail count, records backoff, and emits alert after threshold.
func (tr *TopoRefresher) handleFailure(reason string) error {
	count := tr.failCount.Add(1)
	tr.backoff.FailCount = int(count)

	if count >= alertAfterFailures && tr.onAlert != nil {
		tr.onAlert(fmt.Sprintf("topo refresh: %d consecutive failures, latest: %s", count, reason))
	}

	return fmt.Errorf("%s", reason)
}

// Stop signals the refresh goroutine to stop. Safe to call multiple times.
func (tr *TopoRefresher) Stop() {
	tr.stopMu.Lock()
	defer tr.stopMu.Unlock()
	if tr.stopCh != nil {
		select {
		case <-tr.stopCh:
			// already closed
		default:
			close(tr.stopCh)
		}
	}
}

// ConsecutiveFailures returns the current consecutive failure count.
func (tr *TopoRefresher) ConsecutiveFailures() int32 {
	return tr.failCount.Load()
}
