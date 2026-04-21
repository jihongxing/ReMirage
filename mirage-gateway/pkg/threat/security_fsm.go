package threat

import (
	"log"
	"sync"
	"time"
)

// SecurityMetrics 安全指标（驱动状态机迁移）
type SecurityMetrics struct {
	ThreatLevel      ThreatLevel
	RejectRate       float64 // 最近 1 分钟入口拒绝率
	BlacklistHitRate float64
	ControlPlaneDown bool
}

// SecurityFSM Gateway 本地安全状态机
type SecurityFSM struct {
	current       SecurityState
	cooldownUntil time.Time
	policy        *IngressPolicy
	mu            sync.Mutex
	onStateChange func(SecurityState)
	nowFunc       func() time.Time // 可注入的时间源（测试用）
}

// NewSecurityFSM 创建安全状态机
func NewSecurityFSM(policy *IngressPolicy, onStateChange func(SecurityState)) *SecurityFSM {
	return &SecurityFSM{
		current:       StateNormal,
		policy:        policy,
		onStateChange: onStateChange,
		nowFunc:       time.Now,
	}
}

// Evaluate 根据安全指标评估并执行状态迁移
func (fsm *SecurityFSM) Evaluate(metrics *SecurityMetrics) {
	fsm.mu.Lock()
	defer fsm.mu.Unlock()

	target := fsm.computeTarget(metrics)

	// 升级立即执行
	if target > fsm.current {
		fsm.transition(target)
		return
	}
	// 降级需冷却 300s
	if target < fsm.current && fsm.now().After(fsm.cooldownUntil) {
		fsm.transition(target)
	}
}

// computeTarget 根据指标计算目标状态
func (fsm *SecurityFSM) computeTarget(m *SecurityMetrics) SecurityState {
	switch {
	case m.ControlPlaneDown:
		return StateSilent
	case m.ThreatLevel >= LevelExtreme || m.RejectRate > 0.8:
		return StateIsolated
	case m.ThreatLevel >= LevelCritical || m.RejectRate > 0.5:
		return StateHighPressure
	case m.ThreatLevel >= LevelHigh || m.RejectRate > 0.2:
		return StateAlert
	default:
		return StateNormal
	}
}

// transition 执行状态迁移
func (fsm *SecurityFSM) transition(target SecurityState) {
	old := fsm.current
	fsm.current = target
	fsm.cooldownUntil = fsm.now().Add(300 * time.Second)

	// 9.5: 更新安全状态 Prometheus gauge
	SecurityStateGauge.WithLabelValues(GetGatewayID()).Set(float64(target))

	// 8.3: 通过 IngressPolicy.ApplyStateOverride 覆盖入口策略
	if fsm.policy != nil {
		fsm.policy.ApplyStateOverride(target)
	}

	if fsm.onStateChange != nil {
		go fsm.onStateChange(target)
	}

	log.Printf("[SecurityFSM] 状态迁移: %d → %d", old, target)
}

// ForceState OS 强制切换状态（绕过冷却期）
func (fsm *SecurityFSM) ForceState(state SecurityState) {
	fsm.mu.Lock()
	defer fsm.mu.Unlock()
	fsm.transition(state)
}

// CurrentState 获取当前安全状态
func (fsm *SecurityFSM) CurrentState() SecurityState {
	fsm.mu.Lock()
	defer fsm.mu.Unlock()
	return fsm.current
}

// now 返回当前时间（支持测试注入）
func (fsm *SecurityFSM) now() time.Time {
	if fsm.nowFunc != nil {
		return fsm.nowFunc()
	}
	return time.Now()
}
