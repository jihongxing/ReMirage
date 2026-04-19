// Package strategy - 策略引擎
// 负责根据威胁等级自动调整防御参数
package strategy

import (
	"fmt"
	"log"
	"sync"
	"time"
)

// DefenseLevel 防御等级
type DefenseLevel int

const (
	LevelLow    DefenseLevel = 1  // 低威胁
	LevelMedium DefenseLevel = 2  // 中威胁
	LevelHigh   DefenseLevel = 3  // 高威胁
	LevelCrit   DefenseLevel = 4  // 严重威胁
	LevelMax    DefenseLevel = 5  // 极限防御
)

// StrategyEngine 策略引擎
type StrategyEngine struct {
	currentLevel   DefenseLevel
	threatCount    uint64
	lastAdjustTime time.Time
	mu             sync.RWMutex
	callback       func(level DefenseLevel) error
}

// DefenseParams 防御参数
type DefenseParams struct {
	Level          DefenseLevel
	JitterMeanUs   uint32
	JitterStddevUs uint32
	NoiseIntensity uint32
	PaddingRate    uint32
}

// NewStrategyEngine 创建策略引擎
func NewStrategyEngine(callback func(level DefenseLevel) error) *StrategyEngine {
	return &StrategyEngine{
		currentLevel:   LevelLow,
		lastAdjustTime: time.Now(),
		callback:       callback,
	}
}

// UpdateByThreat 根据威胁更新策略
func (se *StrategyEngine) UpdateByThreat(threatType uint8, severity uint32) {
	se.mu.Lock()
	defer se.mu.Unlock()

	se.threatCount++

	// 计算新的防御等级
	newLevel := se.calculateLevel(threatType, severity)

	// 如果等级变化，且距离上次调整超过 10 秒
	if newLevel != se.currentLevel && time.Since(se.lastAdjustTime) > 10*time.Second {
		oldLevel := se.currentLevel
		se.currentLevel = newLevel
		se.lastAdjustTime = time.Now()

		log.Printf("🔄 [策略引擎] 威胁等级变化: %s → %s (威胁计数: %d)",
			levelName(oldLevel), levelName(newLevel), se.threatCount)

		// 回调更新防御参数
		if se.callback != nil {
			if err := se.callback(newLevel); err != nil {
				log.Printf("❌ [策略引擎] 更新防御参数失败: %v", err)
			} else {
				log.Printf("✅ [策略引擎] 防御参数已更新: %s", se.GetParams().String())
			}
		}
	}
}

// calculateLevel 计算防御等级
func (se *StrategyEngine) calculateLevel(threatType uint8, severity uint32) DefenseLevel {
	// 基于威胁类型和严重程度计算等级
	baseLevel := se.currentLevel

	// 高危威胁类型（主动探测、重放攻击）
	if threatType == 1 || threatType == 2 {
		if severity >= 8 {
			return LevelMax
		} else if severity >= 6 {
			return LevelCrit
		} else if severity >= 4 {
			return LevelHigh
		}
	}

	// 中危威胁类型（时序攻击、DPI 检测）
	if threatType == 3 || threatType == 4 {
		if severity >= 7 {
			return LevelCrit
		} else if severity >= 5 {
			return LevelHigh
		}
	}

	// 威胁计数累积效应
	if se.threatCount > 100 {
		if baseLevel < LevelMax {
			return baseLevel + 1
		}
	} else if se.threatCount > 50 {
		if baseLevel < LevelCrit {
			return baseLevel + 1
		}
	}

	return baseLevel
}

// GetParams 获取当前防御参数
func (se *StrategyEngine) GetParams() *DefenseParams {
	se.mu.RLock()
	defer se.mu.RUnlock()

	return levelToParams(se.currentLevel)
}

// GetLevel 获取当前防御等级
func (se *StrategyEngine) GetLevel() DefenseLevel {
	se.mu.RLock()
	defer se.mu.RUnlock()
	return se.currentLevel
}

// ResetThreatCount 重置威胁计数
func (se *StrategyEngine) ResetThreatCount() {
	se.mu.Lock()
	defer se.mu.Unlock()
	se.threatCount = 0
	log.Println("🔄 [策略引擎] 威胁计数已重置")
}

// levelToParams 将防御等级转换为具体参数
func levelToParams(level DefenseLevel) *DefenseParams {
	switch level {
	case LevelLow:
		return &DefenseParams{
			Level:          LevelLow,
			JitterMeanUs:   10000,  // 10ms
			JitterStddevUs: 3000,   // 3ms
			NoiseIntensity: 5,      // 5%
			PaddingRate:    10,     // 10%
		}
	case LevelMedium:
		return &DefenseParams{
			Level:          LevelMedium,
			JitterMeanUs:   30000,  // 30ms
			JitterStddevUs: 10000,  // 10ms
			NoiseIntensity: 15,     // 15%
			PaddingRate:    20,     // 20%
		}
	case LevelHigh:
		return &DefenseParams{
			Level:          LevelHigh,
			JitterMeanUs:   50000,  // 50ms
			JitterStddevUs: 15000,  // 15ms
			NoiseIntensity: 20,     // 20%
			PaddingRate:    25,     // 25%
		}
	case LevelCrit:
		return &DefenseParams{
			Level:          LevelCrit,
			JitterMeanUs:   80000,  // 80ms
			JitterStddevUs: 25000,  // 25ms
			NoiseIntensity: 25,     // 25%
			PaddingRate:    30,     // 30%
		}
	case LevelMax:
		return &DefenseParams{
			Level:          LevelMax,
			JitterMeanUs:   100000, // 100ms
			JitterStddevUs: 30000,  // 30ms
			NoiseIntensity: 30,     // 30%
			PaddingRate:    35,     // 35%
		}
	default:
		return levelToParams(LevelLow)
	}
}

// levelName 获取等级名称
func levelName(level DefenseLevel) string {
	switch level {
	case LevelLow:
		return "🟢 低威胁"
	case LevelMedium:
		return "🟡 中威胁"
	case LevelHigh:
		return "🟠 高威胁"
	case LevelCrit:
		return "🔴 严重威胁"
	case LevelMax:
		return "🚨 极限防御"
	default:
		return "❓ 未知"
	}
}

// String 格式化输出防御参数
func (dp *DefenseParams) String() string {
	return fmt.Sprintf("Jitter=%dus±%dus, Noise=%d%%, Padding=%d%%",
		dp.JitterMeanUs, dp.JitterStddevUs, dp.NoiseIntensity, dp.PaddingRate)
}
