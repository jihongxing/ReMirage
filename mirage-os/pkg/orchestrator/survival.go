// Package orchestrator - 生存编排器
// 将抗 DDoS 纳入统一状态机，与 Link/Session/Control State 统一编排
package orchestrator

import (
	"log"
	"sync"
	"time"
)

// SurvivalState 系统生存状态
type SurvivalState int

const (
	SurvivalNormal    SurvivalState = iota // 所有节点正常
	SurvivalDegraded                       // 部分节点受压
	SurvivalCritical                       // 多节点受攻击
	SurvivalEmergency                      // 系统级危机
)

// String 返回状态名称
func (s SurvivalState) String() string {
	switch s {
	case SurvivalNormal:
		return "NORMAL"
	case SurvivalDegraded:
		return "DEGRADED"
	case SurvivalCritical:
		return "CRITICAL"
	case SurvivalEmergency:
		return "EMERGENCY"
	default:
		return "UNKNOWN"
	}
}

// SurvivalEvent 生存状态变迁事件
type SurvivalEvent struct {
	FromState SurvivalState
	ToState   SurvivalState
	Timestamp time.Time
	Reason    string
}

// SurvivalStatus 系统生存状态快照
type SurvivalStatus struct {
	State        SurvivalState   `json:"state"`
	OnlineCount  int             `json:"online_count"`
	AttackCount  int             `json:"attack_count"`
	DeadCount    int             `json:"dead_count"`
	RecentEvents []SurvivalEvent `json:"recent_events"`
}

// RegistryReader 拓扑注册表只读接口
type RegistryReader interface {
	GetAllOnline() int
	GetByStatusCount(status string) int
}

// SurvivalOrchestrator 生存编排器
type SurvivalOrchestrator struct {
	registry RegistryReader
	state    SurvivalState
	events   []SurvivalEvent
	mu       sync.RWMutex
}

// NewSurvivalOrchestrator 创建生存编排器
func NewSurvivalOrchestrator(registry RegistryReader) *SurvivalOrchestrator {
	return &SurvivalOrchestrator{
		registry: registry,
		state:    SurvivalNormal,
		events:   make([]SurvivalEvent, 0),
	}
}

// Evaluate 评估系统生存状态
func (so *SurvivalOrchestrator) Evaluate() {
	so.mu.Lock()
	defer so.mu.Unlock()

	online := so.registry.GetAllOnline()
	underAttack := so.registry.GetByStatusCount("UNDER_ATTACK")
	dead := so.registry.GetByStatusCount("DEAD")

	totalActive := online + underAttack
	var newState SurvivalState

	if totalActive == 0 {
		newState = SurvivalEmergency
	} else if float64(underAttack+dead)/float64(totalActive) > 0.5 {
		newState = SurvivalCritical
	} else if underAttack > 0 || dead > 0 {
		newState = SurvivalDegraded
	} else {
		newState = SurvivalNormal
	}

	if newState != so.state {
		event := SurvivalEvent{
			FromState: so.state,
			ToState:   newState,
			Timestamp: time.Now(),
			Reason:    "auto_evaluate",
		}
		so.events = append(so.events, event)
		// 保留最近 100 条
		if len(so.events) > 100 {
			so.events = so.events[len(so.events)-50:]
		}
		log.Printf("[SurvivalOrchestrator] 状态变迁: %s → %s (online=%d, attack=%d, dead=%d)",
			so.state, newState, online, underAttack, dead)
		so.state = newState
	}
}

// GetStatus 返回当前系统生存状态
func (so *SurvivalOrchestrator) GetStatus() SurvivalStatus {
	so.mu.RLock()
	defer so.mu.RUnlock()

	online := so.registry.GetAllOnline()
	underAttack := so.registry.GetByStatusCount("UNDER_ATTACK")
	dead := so.registry.GetByStatusCount("DEAD")

	events := make([]SurvivalEvent, len(so.events))
	copy(events, so.events)

	return SurvivalStatus{
		State:        so.state,
		OnlineCount:  online,
		AttackCount:  underAttack,
		DeadCount:    dead,
		RecentEvents: events,
	}
}

// PrioritizeRecovery 按优先级决定恢复顺序
// Diamond(3) > Platinum(2) > Standard(1)
func (so *SurvivalOrchestrator) PrioritizeRecovery(affectedCells []CellPriority) []CellPriority {
	so.mu.RLock()
	defer so.mu.RUnlock()

	// 按优先级降序排序
	sorted := make([]CellPriority, len(affectedCells))
	copy(sorted, affectedCells)
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j].Level > sorted[i].Level {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}
	return sorted
}

// CellPriority 蜂窝优先级
type CellPriority struct {
	CellID string `json:"cell_id"`
	Level  int    `json:"level"` // 3=Diamond, 2=Platinum, 1=Standard
}

// State 返回当前状态
func (so *SurvivalOrchestrator) State() SurvivalState {
	so.mu.RLock()
	defer so.mu.RUnlock()
	return so.state
}
