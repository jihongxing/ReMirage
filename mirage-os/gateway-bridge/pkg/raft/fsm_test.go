package raft

import (
	"bytes"
	"encoding/json"
	"io"
	"testing"

	"github.com/hashicorp/raft"
	"pgregory.net/rapid"
)

func makeLog(cmd FSMCommand) *raft.Log {
	data, _ := json.Marshal(cmd)
	return &raft.Log{Data: data}
}

func makeFSMCommand(t CommandType, data interface{}) FSMCommand {
	raw, _ := json.Marshal(data)
	return FSMCommand{Type: t, Data: raw}
}

// Feature: core-hardening, Property 5: FSM 快照往返一致性
func TestProperty_FSMSnapshotRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		fsm := NewFSM()

		// 生成随机命令序列
		n := rapid.IntRange(1, 20).Draw(t, "numCommands")
		for i := 0; i < n; i++ {
			cmdType := rapid.IntRange(1, 3).Draw(t, "cmdType")
			switch CommandType(cmdType) {
			case CmdQuotaUpdate:
				cmd := makeFSMCommand(CmdQuotaUpdate, QuotaUpdateData{
					UserID:         rapid.StringMatching(`user-[a-z0-9]{4}`).Draw(t, "userID"),
					RemainingQuota: rapid.Float64Range(0, 10000).Draw(t, "quota"),
				})
				fsm.Apply(makeLog(cmd))
			case CmdBlacklistUpdate:
				cmd := makeFSMCommand(CmdBlacklistUpdate, BlacklistUpdateData{
					SourceIP: rapid.StringMatching(`\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}`).Draw(t, "ip"),
					IsBanned: rapid.Bool().Draw(t, "banned"),
					ExpireAt: rapid.Int64Range(0, 2000000000).Draw(t, "expire"),
				})
				fsm.Apply(makeLog(cmd))
			case CmdStrategyUpdate:
				cmd := makeFSMCommand(CmdStrategyUpdate, StrategyUpdateData{
					CellID:         rapid.StringMatching(`cell-[a-z0-9]{4}`).Draw(t, "cellID"),
					DefenseLevel:   rapid.IntRange(0, 4).Draw(t, "level"),
					JitterMeanUs:   uint32(rapid.IntRange(0, 100000).Draw(t, "jitter")),
					NoiseIntensity: uint32(rapid.IntRange(0, 100).Draw(t, "noise")),
					PaddingRate:    uint32(rapid.IntRange(0, 100).Draw(t, "padding")),
					TemplateID:     uint32(rapid.IntRange(0, 10).Draw(t, "template")),
				})
				fsm.Apply(makeLog(cmd))
			}
		}

		// Snapshot
		snap, err := fsm.Snapshot()
		if err != nil {
			t.Fatalf("snapshot: %v", err)
		}
		var buf bytes.Buffer
		sink := &mockSink{buf: &buf}
		if err := snap.Persist(sink); err != nil {
			t.Fatalf("persist: %v", err)
		}

		// Restore 到新 FSM
		fsm2 := NewFSM()
		if err := fsm2.Restore(io.NopCloser(&buf)); err != nil {
			t.Fatalf("restore: %v", err)
		}

		// 断言状态相同
		fsm.mu.RLock()
		fsm2.mu.RLock()
		defer fsm.mu.RUnlock()
		defer fsm2.mu.RUnlock()

		orig, _ := json.Marshal(fsmSnapshot{Quotas: fsm.quotas, Blacklist: fsm.blacklist, Strategies: fsm.strategies})
		restored, _ := json.Marshal(fsmSnapshot{Quotas: fsm2.quotas, Blacklist: fsm2.blacklist, Strategies: fsm2.strategies})
		if !bytes.Equal(orig, restored) {
			t.Fatalf("snapshot round-trip mismatch:\norig:     %s\nrestored: %s", orig, restored)
		}
	})
}

// Feature: core-hardening, Property 6: FSM 命令幂等性
func TestProperty_FSMIdempotency(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		fsm := NewFSM()
		userID := rapid.StringMatching(`user-[a-z0-9]{4}`).Draw(t, "userID")
		quota := rapid.Float64Range(0, 10000).Draw(t, "quota")

		cmd := makeFSMCommand(CmdQuotaUpdate, QuotaUpdateData{
			UserID:         userID,
			RemainingQuota: quota,
		})

		// Apply 两次
		fsm.Apply(makeLog(cmd))
		fsm.Apply(makeLog(cmd))

		// 值应等于命令值（最后写入胜出，非累加）
		got, ok := fsm.GetQuota(userID)
		if !ok {
			t.Fatal("quota not found")
		}
		if got != quota {
			t.Fatalf("expected %f, got %f", quota, got)
		}
	})
}

// mockSink 用于测试的 SnapshotSink
type mockSink struct {
	buf *bytes.Buffer
}

func (s *mockSink) Write(p []byte) (int, error) { return s.buf.Write(p) }
func (s *mockSink) Close() error                { return nil }
func (s *mockSink) ID() string                  { return "mock" }
func (s *mockSink) Cancel() error               { return nil }
