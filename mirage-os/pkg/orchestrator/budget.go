// Package orchestrator - 抗 DDoS 预算模型
// 不同服务等级绑定不同生存预算
package orchestrator

import (
	"fmt"
	"log"
	"sync"
)

// TierBudget 等级预算
type TierBudget struct {
	MaxHotStandby    int     `json:"max_hot_standby"`     // 最大热备节点数
	MaxSwitchPerHour int     `json:"max_switch_per_hour"` // 每小时最大切换次数
	GuardIntensity   int     `json:"guard_intensity"`     // 入口守卫强度（1-10）
	RecoveryPriority int     `json:"recovery_priority"`   // 恢复优先级
	CurrentUsage     float64 `json:"current_usage"`       // 当前预算使用率（0-1）
	SwitchCount      int     `json:"switch_count"`        // 当前小时切换次数
}

// DDoSBudget 抗 DDoS 预算模型
type DDoSBudget struct {
	tierBudgets map[int]*TierBudget // cellLevel → budget
	mu          sync.RWMutex
}

// 默认预算配置
var defaultBudgets = map[int]*TierBudget{
	3: {MaxHotStandby: 3, MaxSwitchPerHour: 20, GuardIntensity: 10, RecoveryPriority: 3}, // Diamond
	2: {MaxHotStandby: 2, MaxSwitchPerHour: 10, GuardIntensity: 7, RecoveryPriority: 2},  // Platinum
	1: {MaxHotStandby: 1, MaxSwitchPerHour: 5, GuardIntensity: 5, RecoveryPriority: 1},   // Standard
}

// NewDDoSBudget 创建预算模型
func NewDDoSBudget() *DDoSBudget {
	budgets := make(map[int]*TierBudget)
	for k, v := range defaultBudgets {
		b := *v // copy
		budgets[k] = &b
	}
	return &DDoSBudget{tierBudgets: budgets}
}

// CheckBudget 检查指定等级的预算是否允许执行指定动作
func (b *DDoSBudget) CheckBudget(cellLevel int, action string) (bool, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	budget, ok := b.tierBudgets[cellLevel]
	if !ok {
		// 未知等级使用 Standard 预算
		budget = b.tierBudgets[1]
		if budget == nil {
			return false, fmt.Errorf("no budget for level %d", cellLevel)
		}
	}

	switch action {
	case "activate_standby":
		if budget.SwitchCount >= budget.MaxSwitchPerHour {
			log.Printf("[DDoSBudget] ⚠️ 等级 %d 切换预算已耗尽: %d/%d",
				cellLevel, budget.SwitchCount, budget.MaxSwitchPerHour)
			return false, nil
		}
		return true, nil
	case "hot_standby":
		return budget.CurrentUsage < 1.0, nil
	default:
		return true, nil
	}
}

// ConsumeBudget 扣减预算
func (b *DDoSBudget) ConsumeBudget(cellLevel int, action string, cost float64) {
	b.mu.Lock()
	defer b.mu.Unlock()

	budget, ok := b.tierBudgets[cellLevel]
	if !ok {
		budget = b.tierBudgets[1]
		if budget == nil {
			return
		}
	}

	switch action {
	case "activate_standby":
		budget.SwitchCount++
		budget.CurrentUsage = float64(budget.SwitchCount) / float64(budget.MaxSwitchPerHour)
	default:
		budget.CurrentUsage += cost
		if budget.CurrentUsage > 1.0 {
			budget.CurrentUsage = 1.0
		}
	}
}

// GetBudgetStatus 返回所有等级的预算状态
func (b *DDoSBudget) GetBudgetStatus() map[int]*TierBudget {
	b.mu.RLock()
	defer b.mu.RUnlock()

	result := make(map[int]*TierBudget, len(b.tierBudgets))
	for k, v := range b.tierBudgets {
		cp := *v
		result[k] = &cp
	}
	return result
}

// ResetHourlyCounters 重置每小时计数器（由定时器调用）
func (b *DDoSBudget) ResetHourlyCounters() {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, budget := range b.tierBudgets {
		budget.SwitchCount = 0
		budget.CurrentUsage = 0
	}
}
