// Package commit - 冷却时间管理器
package commit

import (
	"context"
	"sync"
	"time"
)

// CooldownConfig 冷却时间配置
type CooldownConfig struct {
	PersonaSwitch       time.Duration
	LinkMigration       time.Duration
	GatewayReassignment time.Duration
	SurvivalModeSwitch  time.Duration
}

// DefaultCooldownConfig 默认冷却时间配置
var DefaultCooldownConfig = CooldownConfig{
	PersonaSwitch:       30 * time.Second,
	LinkMigration:       10 * time.Second,
	GatewayReassignment: 60 * time.Second,
	SurvivalModeSwitch:  60 * time.Second,
}

// CooldownManager 冷却时间管理器接口
type CooldownManager interface {
	CheckCooldown(ctx context.Context, txType TxType) error
	RecordCompletion(txType TxType, finishedAt time.Time)
}

// cooldownManagerImpl 冷却时间管理器实现
type cooldownManagerImpl struct {
	mu         sync.RWMutex
	config     CooldownConfig
	lastFinish map[TxType]time.Time
}

// NewCooldownManager 创建冷却时间管理器
func NewCooldownManager(config CooldownConfig) CooldownManager {
	return &cooldownManagerImpl{
		config:     config,
		lastFinish: make(map[TxType]time.Time),
	}
}

// CheckCooldown 检查冷却时间
func (m *cooldownManagerImpl) CheckCooldown(_ context.Context, txType TxType) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	last, ok := m.lastFinish[txType]
	if !ok {
		return nil
	}

	cooldown := m.getCooldownDuration(txType)
	elapsed := time.Since(last)
	if elapsed < cooldown {
		remaining := cooldown - elapsed
		return &ErrCooldownActive{TxType: txType, RemainingSeconds: remaining.Seconds()}
	}
	return nil
}

// RecordCompletion 记录事务完成时间
func (m *cooldownManagerImpl) RecordCompletion(txType TxType, finishedAt time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastFinish[txType] = finishedAt
}

func (m *cooldownManagerImpl) getCooldownDuration(txType TxType) time.Duration {
	switch txType {
	case TxTypePersonaSwitch:
		return m.config.PersonaSwitch
	case TxTypeLinkMigration:
		return m.config.LinkMigration
	case TxTypeGatewayReassignment:
		return m.config.GatewayReassignment
	case TxTypeSurvivalModeSwitch:
		return m.config.SurvivalModeSwitch
	default:
		return 30 * time.Second
	}
}
