// Package raft - Raft FSM (Finite State Machine)
package raft

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"sync"

	"github.com/hashicorp/raft"
)

// CommandType 命令类型
type CommandType string

const (
	CommandSetQuota         CommandType = "set_quota"
	CommandUpdateGateway    CommandType = "update_gateway"
	CommandAddThreat        CommandType = "add_threat"
	CommandSetConfig        CommandType = "set_config"
	CommandTacticalUpdate   CommandType = "tactical_state_update"
	CommandGhostModeToggle  CommandType = "ghost_mode_toggle"
)

// Command Raft 命令
type Command struct {
	Type    CommandType     `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// FSM Raft 有限状态机
type FSM struct {
	mu           sync.RWMutex
	state        map[string]interface{}
	tacticalMode TacticalMode
	ghostMode    bool
}

// NewFSM 创建 FSM
func NewFSM() *FSM {
	return &FSM{
		state: make(map[string]interface{}),
	}
}

// Apply 应用日志条目
func (f *FSM) Apply(log *raft.Log) interface{} {
	var cmd Command
	if err := json.Unmarshal(log.Data, &cmd); err != nil {
		return fmt.Errorf("解析命令失败: %w", err)
	}
	
	f.mu.Lock()
	defer f.mu.Unlock()
	
	switch cmd.Type {
	case CommandSetQuota:
		return f.applySetQuota(cmd.Payload)
	case CommandUpdateGateway:
		return f.applyUpdateGateway(cmd.Payload)
	case CommandAddThreat:
		return f.applyAddThreat(cmd.Payload)
	case CommandSetConfig:
		return f.applySetConfig(cmd.Payload)
	case CommandTacticalUpdate:
		return f.applyTacticalUpdate(cmd.Payload)
	case CommandGhostModeToggle:
		return f.applyGhostModeToggle(cmd.Payload)
	default:
		return fmt.Errorf("未知命令类型: %s", cmd.Type)
	}
}

// Snapshot 创建快照
func (f *FSM) Snapshot() (raft.FSMSnapshot, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	
	// 深拷贝状态
	snapshot := make(map[string]interface{})
	for k, v := range f.state {
		snapshot[k] = v
	}
	
	return &FSMSnapshot{state: snapshot}, nil
}

// Restore 从快照恢复
func (f *FSM) Restore(rc io.ReadCloser) error {
	defer rc.Close()
	
	var state map[string]interface{}
	if err := json.NewDecoder(rc).Decode(&state); err != nil {
		return fmt.Errorf("解码快照失败: %w", err)
	}
	
	f.mu.Lock()
	defer f.mu.Unlock()
	
	f.state = state
	
	log.Println("[FSM] ✅ 已从快照恢复")
	
	return nil
}

// applySetQuota 应用配额设置
func (f *FSM) applySetQuota(payload json.RawMessage) interface{} {
	var data struct {
		UserID string `json:"user_id"`
		Quota  int64  `json:"quota"`
	}
	
	if err := json.Unmarshal(payload, &data); err != nil {
		return err
	}
	
	key := fmt.Sprintf("quota:%s", data.UserID)
	f.state[key] = data.Quota
	
	log.Printf("[FSM] 设置配额: %s = %d", data.UserID, data.Quota)
	
	return nil
}

// applyUpdateGateway 应用网关更新
func (f *FSM) applyUpdateGateway(payload json.RawMessage) interface{} {
	var data struct {
		GatewayID string                 `json:"gateway_id"`
		Status    map[string]interface{} `json:"status"`
	}
	
	if err := json.Unmarshal(payload, &data); err != nil {
		return err
	}
	
	key := fmt.Sprintf("gateway:%s", data.GatewayID)
	f.state[key] = data.Status
	
	log.Printf("[FSM] 更新网关: %s", data.GatewayID)
	
	return nil
}

// applyAddThreat 应用威胁添加
func (f *FSM) applyAddThreat(payload json.RawMessage) interface{} {
	var data struct {
		IP         string `json:"ip"`
		ThreatType int    `json:"threat_type"`
	}
	
	if err := json.Unmarshal(payload, &data); err != nil {
		return err
	}
	
	key := fmt.Sprintf("threat:%s", data.IP)
	f.state[key] = data.ThreatType
	
	log.Printf("[FSM] 添加威胁: %s (类型: %d)", data.IP, data.ThreatType)
	
	return nil
}

// applySetConfig 应用配置设置
func (f *FSM) applySetConfig(payload json.RawMessage) interface{} {
	var data struct {
		Key   string      `json:"key"`
		Value interface{} `json:"value"`
	}
	
	if err := json.Unmarshal(payload, &data); err != nil {
		return err
	}
	
	f.state[data.Key] = data.Value
	
	log.Printf("[FSM] 设置配置: %s", data.Key)
	
	return nil
}

// Get 获取状态
func (f *FSM) Get(key string) (interface{}, bool) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	
	val, ok := f.state[key]
	return val, ok
}

// GetTacticalMode 获取当前战术模式
func (f *FSM) GetTacticalMode() TacticalMode {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.tacticalMode
}

// GetGhostMode 获取 Ghost Mode 状态
func (f *FSM) GetGhostMode() bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.ghostMode
}

// applyTacticalUpdate 应用战术模式更新
func (f *FSM) applyTacticalUpdate(payload json.RawMessage) interface{} {
	var data TacticalStateUpdate
	if err := json.Unmarshal(payload, &data); err != nil {
		return err
	}
	
	f.tacticalMode = data.Mode
	f.state["tactical_mode"] = data.Mode
	f.state["tactical_config"] = data.Config
	
	log.Printf("[FSM] 战术模式更新: %d (发起者: %s)", data.Mode, data.Issuer)
	return nil
}

// applyGhostModeToggle 应用 Ghost Mode 切换
func (f *FSM) applyGhostModeToggle(payload json.RawMessage) interface{} {
	var data struct {
		Enabled   bool  `json:"enabled"`
		Timestamp int64 `json:"timestamp"`
	}
	if err := json.Unmarshal(payload, &data); err != nil {
		return err
	}
	
	f.ghostMode = data.Enabled
	f.state["ghost_mode"] = data.Enabled
	
	log.Printf("[FSM] Ghost Mode: %v", data.Enabled)
	return nil
}

// FSMSnapshot FSM 快照
type FSMSnapshot struct {
	state map[string]interface{}
}

// Persist 持久化快照
func (s *FSMSnapshot) Persist(sink raft.SnapshotSink) error {
	err := func() error {
		// 编码状态
		if err := json.NewEncoder(sink).Encode(s.state); err != nil {
			return err
		}
		
		return sink.Close()
	}()
	
	if err != nil {
		sink.Cancel()
		return err
	}
	
	return nil
}

// Release 释放快照
func (s *FSMSnapshot) Release() {}
