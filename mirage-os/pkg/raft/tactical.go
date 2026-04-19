// Package raft - 全局战术模式广播
package raft

import (
	"encoding/json"
	"fmt"
	"log"
	"time"
)

// TacticalMode 战术模式
type TacticalMode int

const (
	TacticalNormal     TacticalMode = 0 // 常规模式
	TacticalSleep      TacticalMode = 1 // 休眠模式
	TacticalAggressive TacticalMode = 2 // 激进模式
	TacticalStealth    TacticalMode = 3 // 隐匿模式
)

// TacticalConfig 战术配置
type TacticalConfig struct {
	Mode            TacticalMode `json:"mode"`
	SocialJitter    int          `json:"socialJitter"`    // 0-100
	CIDRotationRate int          `json:"cidRotationRate"` // 次/分钟
	FECRedundancy   int          `json:"fecRedundancy"`   // 百分比
	DNAWeights      map[string]int `json:"dnaWeights"`    // B-DNA 模板权重
	StealthFilter   int          `json:"stealthFilter"`   // 隐匿模式下的最低威胁等级
}

// TacticalStateUpdate Raft 命令
type TacticalStateUpdate struct {
	Type      string          `json:"type"`
	Mode      TacticalMode    `json:"mode"`
	Config    TacticalConfig  `json:"config"`
	Timestamp int64           `json:"timestamp"`
	Issuer    string          `json:"issuer"` // 发起者节点 ID
}

// GetTacticalConfig 根据模式获取配置
func GetTacticalConfig(mode TacticalMode) TacticalConfig {
	switch mode {
	case TacticalSleep:
		return TacticalConfig{
			Mode:            TacticalSleep,
			SocialJitter:    10,
			CIDRotationRate: 1,
			FECRedundancy:   10,
			DNAWeights: map[string]int{
				"system_update": 100,
				"background":    80,
				"video_conference": 0,
				"streaming":     0,
				"gaming":        0,
			},
			StealthFilter: 0,
		}
	case TacticalAggressive:
		return TacticalConfig{
			Mode:            TacticalAggressive,
			SocialJitter:    90,
			CIDRotationRate: 25,
			FECRedundancy:   45, // RS(10,8)
			DNAWeights: map[string]int{
				"video_conference": 100,
				"streaming":        90,
				"gaming":           80,
				"system_update":    50,
			},
			StealthFilter: 0,
		}
	case TacticalStealth:
		return TacticalConfig{
			Mode:            TacticalStealth,
			SocialJitter:    70,
			CIDRotationRate: 20,
			FECRedundancy:   35,
			DNAWeights: map[string]int{
				"video_conference": 70,
				"streaming":        60,
				"system_update":    90,
			},
			StealthFilter: 9, // 只推送 Severity > 9 的威胁
		}
	default: // Normal
		return TacticalConfig{
			Mode:            TacticalNormal,
			SocialJitter:    50,
			CIDRotationRate: 5,
			FECRedundancy:   20,
			DNAWeights: map[string]int{
				"video_conference": 80,
				"streaming":        70,
				"gaming":           60,
				"system_update":    50,
			},
			StealthFilter: 0,
		}
	}
}

// BroadcastTacticalMode 广播战术模式到所有节点
func (c *Cluster) BroadcastTacticalMode(mode TacticalMode) error {
	if !c.IsLeader() {
		return fmt.Errorf("只有 Leader 可以广播战术模式")
	}

	config := GetTacticalConfig(mode)
	update := TacticalStateUpdate{
		Type:      "tactical_state_update",
		Mode:      mode,
		Config:    config,
		Timestamp: time.Now().UnixNano(),
		Issuer:    c.config.NodeID,
	}

	data, err := json.Marshal(update)
	if err != nil {
		return fmt.Errorf("序列化战术配置失败: %w", err)
	}

	// 通过 Raft 共识广播
	if err := c.Apply(data, 5*time.Second); err != nil {
		return fmt.Errorf("广播战术配置失败: %w", err)
	}

	log.Printf("[Tactical] ✅ 已广播战术模式: %d 到所有节点", mode)
	return nil
}

// GetCurrentTacticalMode 获取当前战术模式
func (c *Cluster) GetCurrentTacticalMode() TacticalMode {
	if c.fsm == nil {
		return TacticalNormal
	}
	return c.fsm.GetTacticalMode()
}

// GetCurrentTacticalConfig 获取当前战术配置
func (c *Cluster) GetCurrentTacticalConfig() TacticalConfig {
	return GetTacticalConfig(c.GetCurrentTacticalMode())
}
