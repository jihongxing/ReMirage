// Package phantom 运维克制策略执行器
// 实现资源熔断、数据衰减、心理战退出
package phantom

import (
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// GovernancePolicy 克制策略
type GovernancePolicy struct {
	ResourceLimit    ResourceLimitPolicy    `yaml:"resource_limit"`
	CortexRetention  CortexRetentionPolicy  `yaml:"cortex_retention"`
	PhantomWarfare   PhantomWarfarePolicy   `yaml:"phantom_warfare"`
	CthulhuMode      CthulhuModePolicy      `yaml:"cthulhu_mode"`
	Reincarnation    ReincarnationPolicy    `yaml:"reincarnation"`
}

// ResourceLimitPolicy 资源限制策略
type ResourceLimitPolicy struct {
	MaxCPUPercent   int    `yaml:"max_cpu_percent"`
	MaxMemMB        int    `yaml:"max_mem_mb"`
	IOPriority      string `yaml:"io_priority"`
	MaxGoroutines   int    `yaml:"max_goroutines"`
	CircuitBreaker  CircuitBreakerConfig `yaml:"circuit_breaker"`
}

// CircuitBreakerConfig 熔断配置
type CircuitBreakerConfig struct {
	Enabled      bool   `yaml:"enabled"`
	ThresholdCPU int    `yaml:"threshold_cpu"`
	ThresholdMem int    `yaml:"threshold_mem"`
	Action       string `yaml:"action"`
}

// CortexRetentionPolicy 指纹库保留策略
type CortexRetentionPolicy struct {
	HighRiskTTL      time.Duration `yaml:"high_risk_ttl"`
	MediumRiskTTL    time.Duration `yaml:"medium_risk_ttl"`
	LowRiskTTL       time.Duration `yaml:"low_risk_ttl"`
	InactiveTTL      time.Duration `yaml:"inactive_ttl"`
	MaxDBRecords     int           `yaml:"max_db_records"`
	CleanupInterval  time.Duration `yaml:"cleanup_interval"`
	CleanupBatchSize int           `yaml:"cleanup_batch_size"`
}

// PhantomWarfarePolicy 心理战策略
type PhantomWarfarePolicy struct {
	MaxSessionDuration  time.Duration `yaml:"max_session_duration"`
	MaxTotalDuration    time.Duration `yaml:"max_total_duration"`
	AutoRefreshSeed     bool          `yaml:"auto_refresh_seed"`
	SeedRefreshInterval time.Duration `yaml:"seed_refresh_interval"`
	CanaryTraceDepth    int           `yaml:"canary_trace_depth"`
	CeasefireAction     string        `yaml:"ceasefire_action"`
}

// CthulhuModePolicy 克苏鲁模式策略
type CthulhuModePolicy struct {
	MaxActivationsPerUID int           `yaml:"max_activations_per_uid"`
	CooldownPeriod       time.Duration `yaml:"cooldown_period"`
	MaxResponseSizeKB    int           `yaml:"max_response_size_kb"`
	PayloadMirrorLimit   int           `yaml:"payload_mirror_limit"`
}

// ReincarnationPolicy 重生策略
type ReincarnationPolicy struct {
	MaxGenerations   int `yaml:"max_generations"`
	MinIntervalMs    int `yaml:"min_interval_ms"`
	MaxActiveShadows int `yaml:"max_active_shadows"`
}

// GovernanceEnforcer 策略执行器
type GovernanceEnforcer struct {
	mu sync.RWMutex

	policy GovernancePolicy

	// 会话追踪
	sessions map[string]*SessionTracker

	// 克苏鲁激活计数
	cthulhuActivations map[string]*CthulhuTracker

	// 重生计数
	reincarnationCount map[string]int

	// 熔断状态
	circuitOpen int32

	// 统计
	stats GovernanceStats

	// 停止信号
	stopChan chan struct{}
}

// SessionTracker 会话追踪
type SessionTracker struct {
	UID           string
	StartTime     time.Time
	TotalDuration time.Duration
	LastActivity  time.Time
	Ceased        bool
}

// CthulhuTracker 克苏鲁追踪
type CthulhuTracker struct {
	UID          string
	Activations  int
	LastActivation time.Time
	MirrorCount  int
}

// GovernanceStats 统计
type GovernanceStats struct {
	CircuitBreaks     int64
	SessionsCeased    int64
	FingerprintsPurged int64
	CthulhuThrottled  int64
}

// DefaultGovernancePolicy 默认策略
func DefaultGovernancePolicy() GovernancePolicy {
	return GovernancePolicy{
		ResourceLimit: ResourceLimitPolicy{
			MaxCPUPercent: 15,
			MaxMemMB:      2048,
			IOPriority:    "low",
			MaxGoroutines: 500,
			CircuitBreaker: CircuitBreakerConfig{
				Enabled:      true,
				ThresholdCPU: 80,
				ThresholdMem: 85,
				Action:       "drop_phantom",
			},
		},
		CortexRetention: CortexRetentionPolicy{
			HighRiskTTL:      90 * 24 * time.Hour,
			MediumRiskTTL:    30 * 24 * time.Hour,
			LowRiskTTL:       15 * 24 * time.Hour,
			InactiveTTL:      7 * 24 * time.Hour,
			MaxDBRecords:     1000000,
			CleanupInterval:  time.Hour,
			CleanupBatchSize: 10000,
		},
		PhantomWarfare: PhantomWarfarePolicy{
			MaxSessionDuration:  6 * time.Hour,
			MaxTotalDuration:    24 * time.Hour,
			AutoRefreshSeed:     true,
			SeedRefreshInterval: 4 * time.Hour,
			CanaryTraceDepth:    3,
			CeasefireAction:     "deadend",
		},
		CthulhuMode: CthulhuModePolicy{
			MaxActivationsPerUID: 10,
			CooldownPeriod:       30 * time.Minute,
			MaxResponseSizeKB:    64,
			PayloadMirrorLimit:   3,
		},
		Reincarnation: ReincarnationPolicy{
			MaxGenerations:   50,
			MinIntervalMs:    100,
			MaxActiveShadows: 100,
		},
	}
}

// NewGovernanceEnforcer 创建执行器
func NewGovernanceEnforcer(policy GovernancePolicy) *GovernanceEnforcer {
	return &GovernanceEnforcer{
		policy:             policy,
		sessions:           make(map[string]*SessionTracker),
		cthulhuActivations: make(map[string]*CthulhuTracker),
		reincarnationCount: make(map[string]int),
		stopChan:           make(chan struct{}),
	}
}

// Start 启动策略执行
func (ge *GovernanceEnforcer) Start() {
	go ge.monitorResources()
	go ge.cleanupLoop()
}

// Stop 停止
func (ge *GovernanceEnforcer) Stop() {
	close(ge.stopChan)
}

// monitorResources 资源监控
func (ge *GovernanceEnforcer) monitorResources() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ge.stopChan:
			return
		case <-ticker.C:
			ge.checkResourceLimits()
		}
	}
}

// checkResourceLimits 检查资源限制
func (ge *GovernanceEnforcer) checkResourceLimits() {
	if !ge.policy.ResourceLimit.CircuitBreaker.Enabled {
		return
	}

	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	memUsedMB := int(m.Alloc / 1024 / 1024)
	goroutines := runtime.NumGoroutine()

	// 检查内存
	if memUsedMB > ge.policy.ResourceLimit.MaxMemMB {
		ge.tripCircuitBreaker("memory_exceeded")
		return
	}

	// 检查 Goroutine 数量
	if goroutines > ge.policy.ResourceLimit.MaxGoroutines {
		ge.tripCircuitBreaker("goroutines_exceeded")
		return
	}

	// 恢复熔断
	atomic.StoreInt32(&ge.circuitOpen, 0)
}

// tripCircuitBreaker 触发熔断
func (ge *GovernanceEnforcer) tripCircuitBreaker(reason string) {
	if atomic.CompareAndSwapInt32(&ge.circuitOpen, 0, 1) {
		ge.mu.Lock()
		ge.stats.CircuitBreaks++
		ge.mu.Unlock()
	}
}

// IsCircuitOpen 熔断是否开启
func (ge *GovernanceEnforcer) IsCircuitOpen() bool {
	return atomic.LoadInt32(&ge.circuitOpen) == 1
}

// ShouldAllowPhantom 是否允许影子操作
func (ge *GovernanceEnforcer) ShouldAllowPhantom() bool {
	return !ge.IsCircuitOpen()
}

// cleanupLoop 清理循环
func (ge *GovernanceEnforcer) cleanupLoop() {
	ticker := time.NewTicker(ge.policy.CortexRetention.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ge.stopChan:
			return
		case <-ticker.C:
			ge.cleanupSessions()
		}
	}
}

// cleanupSessions 清理过期会话
func (ge *GovernanceEnforcer) cleanupSessions() {
	ge.mu.Lock()
	defer ge.mu.Unlock()

	now := time.Now()

	for uid, session := range ge.sessions {
		// 检查不活跃
		if now.Sub(session.LastActivity) > ge.policy.CortexRetention.InactiveTTL {
			delete(ge.sessions, uid)
			continue
		}

		// 检查总时长
		if session.TotalDuration > ge.policy.PhantomWarfare.MaxTotalDuration {
			session.Ceased = true
			ge.stats.SessionsCeased++
		}
	}

	// 清理克苏鲁追踪
	for uid, tracker := range ge.cthulhuActivations {
		if now.Sub(tracker.LastActivation) > 24*time.Hour {
			delete(ge.cthulhuActivations, uid)
		}
	}
}

// TrackSession 追踪会话
func (ge *GovernanceEnforcer) TrackSession(uid string) *SessionTracker {
	ge.mu.Lock()
	defer ge.mu.Unlock()

	tracker, exists := ge.sessions[uid]
	if !exists {
		tracker = &SessionTracker{
			UID:          uid,
			StartTime:    time.Now(),
			LastActivity: time.Now(),
		}
		ge.sessions[uid] = tracker
	}

	tracker.LastActivity = time.Now()
	tracker.TotalDuration = time.Since(tracker.StartTime)

	return tracker
}

// ShouldCeasefire 是否应停火
func (ge *GovernanceEnforcer) ShouldCeasefire(uid string) (bool, string) {
	ge.mu.RLock()
	defer ge.mu.RUnlock()

	tracker, exists := ge.sessions[uid]
	if !exists {
		return false, ""
	}

	if tracker.Ceased {
		return true, "already_ceased"
	}

	// 检查单次会话时长
	sessionDuration := time.Since(tracker.StartTime)
	if sessionDuration > ge.policy.PhantomWarfare.MaxSessionDuration {
		return true, "session_timeout"
	}

	// 检查累计时长
	if tracker.TotalDuration > ge.policy.PhantomWarfare.MaxTotalDuration {
		return true, "total_timeout"
	}

	return false, ""
}

// CanActivateCthulhu 是否可激活克苏鲁
func (ge *GovernanceEnforcer) CanActivateCthulhu(uid string) bool {
	ge.mu.Lock()
	defer ge.mu.Unlock()

	tracker, exists := ge.cthulhuActivations[uid]
	if !exists {
		ge.cthulhuActivations[uid] = &CthulhuTracker{
			UID:            uid,
			Activations:    1,
			LastActivation: time.Now(),
		}
		return true
	}

	// 检查冷却期
	if time.Since(tracker.LastActivation) < ge.policy.CthulhuMode.CooldownPeriod {
		ge.stats.CthulhuThrottled++
		return false
	}

	// 检查激活次数
	if tracker.Activations >= ge.policy.CthulhuMode.MaxActivationsPerUID {
		ge.stats.CthulhuThrottled++
		return false
	}

	tracker.Activations++
	tracker.LastActivation = time.Now()
	return true
}

// CanMirrorPayload 是否可回显载荷
func (ge *GovernanceEnforcer) CanMirrorPayload(uid string) bool {
	ge.mu.Lock()
	defer ge.mu.Unlock()

	tracker, exists := ge.cthulhuActivations[uid]
	if !exists {
		return true
	}

	if tracker.MirrorCount >= ge.policy.CthulhuMode.PayloadMirrorLimit {
		return false
	}

	tracker.MirrorCount++
	return true
}

// CanReincarnate 是否可重生
func (ge *GovernanceEnforcer) CanReincarnate(uid string) bool {
	ge.mu.Lock()
	defer ge.mu.Unlock()

	count := ge.reincarnationCount[uid]
	if count >= ge.policy.Reincarnation.MaxGenerations {
		return false
	}

	ge.reincarnationCount[uid] = count + 1
	return true
}

// GetCeasefireAction 获取停火动作
func (ge *GovernanceEnforcer) GetCeasefireAction() string {
	return ge.policy.PhantomWarfare.CeasefireAction
}

// GetMaxResponseSize 获取最大响应大小
func (ge *GovernanceEnforcer) GetMaxResponseSize() int {
	return ge.policy.CthulhuMode.MaxResponseSizeKB * 1024
}

// GetStats 获取统计
func (ge *GovernanceEnforcer) GetStats() GovernanceStats {
	ge.mu.RLock()
	defer ge.mu.RUnlock()
	return ge.stats
}

// GetPolicy 获取策略
func (ge *GovernanceEnforcer) GetPolicy() GovernancePolicy {
	ge.mu.RLock()
	defer ge.mu.RUnlock()
	return ge.policy
}

// UpdatePolicy 更新策略
func (ge *GovernanceEnforcer) UpdatePolicy(policy GovernancePolicy) {
	ge.mu.Lock()
	defer ge.mu.Unlock()
	ge.policy = policy
}
