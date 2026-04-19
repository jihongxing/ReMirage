package raft

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"github.com/hashicorp/raft"
)

// CommandType FSM 命令类型
type CommandType int

const (
	CmdQuotaUpdate CommandType = iota + 1
	CmdBlacklistUpdate
	CmdStrategyUpdate
)

// FSMCommand FSM 命令
type FSMCommand struct {
	Type CommandType     `json:"type"`
	Data json.RawMessage `json:"data"`
}

// QuotaUpdateData 配额变更数据
type QuotaUpdateData struct {
	UserID         string  `json:"user_id"`
	RemainingQuota float64 `json:"remaining_quota"`
}

// BlacklistUpdateData 黑名单变更数据
type BlacklistUpdateData struct {
	SourceIP string `json:"source_ip"`
	IsBanned bool   `json:"is_banned"`
	ExpireAt int64  `json:"expire_at"`
}

// StrategyUpdateData 策略变更数据
type StrategyUpdateData struct {
	CellID         string `json:"cell_id"`
	DefenseLevel   int    `json:"defense_level"`
	JitterMeanUs   uint32 `json:"jitter_mean_us"`
	NoiseIntensity uint32 `json:"noise_intensity"`
	PaddingRate    uint32 `json:"padding_rate"`
	TemplateID     uint32 `json:"template_id"`
}

// FSM Raft 有限状态机
type FSM struct {
	mu         sync.RWMutex
	quotas     map[string]float64
	blacklist  map[string]*BlacklistUpdateData
	strategies map[string]*StrategyUpdateData
}

func NewFSM() *FSM {
	return &FSM{
		quotas:     make(map[string]float64),
		blacklist:  make(map[string]*BlacklistUpdateData),
		strategies: make(map[string]*StrategyUpdateData),
	}
}

func (f *FSM) Apply(log *raft.Log) interface{} {
	var cmd FSMCommand
	if err := json.Unmarshal(log.Data, &cmd); err != nil {
		return fmt.Errorf("unmarshal command: %w", err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	switch cmd.Type {
	case CmdQuotaUpdate:
		var data QuotaUpdateData
		if err := json.Unmarshal(cmd.Data, &data); err != nil {
			return fmt.Errorf("unmarshal quota: %w", err)
		}
		f.quotas[data.UserID] = data.RemainingQuota
	case CmdBlacklistUpdate:
		var data BlacklistUpdateData
		if err := json.Unmarshal(cmd.Data, &data); err != nil {
			return fmt.Errorf("unmarshal blacklist: %w", err)
		}
		f.blacklist[data.SourceIP] = &data
	case CmdStrategyUpdate:
		var data StrategyUpdateData
		if err := json.Unmarshal(cmd.Data, &data); err != nil {
			return fmt.Errorf("unmarshal strategy: %w", err)
		}
		f.strategies[data.CellID] = &data
	default:
		return fmt.Errorf("unknown command type: %d", cmd.Type)
	}
	return nil
}

type fsmSnapshot struct {
	Quotas     map[string]float64              `json:"quotas"`
	Blacklist  map[string]*BlacklistUpdateData `json:"blacklist"`
	Strategies map[string]*StrategyUpdateData  `json:"strategies"`
}

func (f *FSM) Snapshot() (raft.FSMSnapshot, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	snap := &fsmSnapshot{
		Quotas:     make(map[string]float64, len(f.quotas)),
		Blacklist:  make(map[string]*BlacklistUpdateData, len(f.blacklist)),
		Strategies: make(map[string]*StrategyUpdateData, len(f.strategies)),
	}
	for k, v := range f.quotas {
		snap.Quotas[k] = v
	}
	for k, v := range f.blacklist {
		cp := *v
		snap.Blacklist[k] = &cp
	}
	for k, v := range f.strategies {
		cp := *v
		snap.Strategies[k] = &cp
	}
	return snap, nil
}

func (f *FSM) Restore(rc io.ReadCloser) error {
	defer rc.Close()
	var snap fsmSnapshot
	if err := json.NewDecoder(rc).Decode(&snap); err != nil {
		return fmt.Errorf("decode snapshot: %w", err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.quotas = snap.Quotas
	f.blacklist = snap.Blacklist
	f.strategies = snap.Strategies
	return nil
}

func (s *fsmSnapshot) Persist(sink raft.SnapshotSink) error {
	data, err := json.Marshal(s)
	if err != nil {
		sink.Cancel()
		return err
	}
	if _, err := sink.Write(data); err != nil {
		sink.Cancel()
		return err
	}
	return sink.Close()
}

func (s *fsmSnapshot) Release() {}

func (f *FSM) GetQuota(userID string) (float64, bool) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	v, ok := f.quotas[userID]
	return v, ok
}

func (f *FSM) GetBlacklist() map[string]*BlacklistUpdateData {
	f.mu.RLock()
	defer f.mu.RUnlock()
	cp := make(map[string]*BlacklistUpdateData, len(f.blacklist))
	for k, v := range f.blacklist {
		entry := *v
		cp[k] = &entry
	}
	return cp
}

func (f *FSM) GetStrategy(cellID string) (*StrategyUpdateData, bool) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	v, ok := f.strategies[cellID]
	if !ok {
		return nil, false
	}
	cp := *v
	return &cp, true
}
