package tests

import (
	"fmt"
	"os"
	"testing"
	"time"

	"mirage-os/gateway-bridge/pkg/config"
	raftpkg "mirage-os/gateway-bridge/pkg/raft"
)

// TestRaftFailover Raft 故障转移集成测试
func TestRaftFailover(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "raft-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// 启动 3 节点 Raft 集群
	peers := []config.PeerConfig{
		{ID: "node-1", Address: "127.0.0.1:17001", Voter: true},
		{ID: "node-2", Address: "127.0.0.1:17002", Voter: true},
		{ID: "node-3", Address: "127.0.0.1:17003", Voter: true},
	}

	clusters := make([]*raftpkg.Cluster, 3)
	for i := 0; i < 3; i++ {
		cfg := config.RaftConfig{
			NodeID:    fmt.Sprintf("node-%d", i+1),
			BindAddr:  fmt.Sprintf("127.0.0.1:%d", 17001+i),
			DataDir:   fmt.Sprintf("%s/node-%d", tmpDir, i+1),
			Bootstrap: i == 0,
			Peers:     peers,
		}
		c, err := raftpkg.NewCluster(cfg)
		if err != nil {
			t.Fatalf("new cluster node-%d: %v", i+1, err)
		}
		if err := c.Start(); err != nil {
			t.Fatalf("start cluster node-%d: %v", i+1, err)
		}
		clusters[i] = c
	}
	defer func() {
		for _, c := range clusters {
			if c != nil {
				c.Shutdown()
			}
		}
	}()

	// 等待 Leader 选举
	var leaderIdx int = -1
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		for i, c := range clusters {
			if c.IsLeader() {
				leaderIdx = i
				break
			}
		}
		if leaderIdx >= 0 {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if leaderIdx < 0 {
		t.Fatal("no leader elected within 10s")
	}
	t.Logf("initial leader: node-%d", leaderIdx+1)

	// 向 Leader 提交一条命令验证正常工作
	err = clusters[leaderIdx].Apply(raftpkg.FSMCommand{
		Type: raftpkg.CmdQuotaUpdate,
		Data: []byte(`{"user_id":"test-user","remaining_quota":100.0}`),
	})
	if err != nil {
		t.Fatalf("apply to leader: %v", err)
	}

	// 停止 Leader
	t.Logf("stopping leader node-%d", leaderIdx+1)
	clusters[leaderIdx].Shutdown()
	clusters[leaderIdx] = nil

	// 等待新 Leader 选举（10 秒内）
	var newLeaderIdx int = -1
	deadline = time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		for i, c := range clusters {
			if c != nil && c.IsLeader() {
				newLeaderIdx = i
				break
			}
		}
		if newLeaderIdx >= 0 {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if newLeaderIdx < 0 {
		t.Fatal("no new leader elected within 10s after leader shutdown")
	}
	t.Logf("new leader: node-%d", newLeaderIdx+1)

	if newLeaderIdx == leaderIdx {
		t.Fatal("new leader should be different from old leader")
	}

	// 向新 Leader 提交命令验证正常工作
	err = clusters[newLeaderIdx].Apply(raftpkg.FSMCommand{
		Type: raftpkg.CmdQuotaUpdate,
		Data: []byte(`{"user_id":"test-user-2","remaining_quota":200.0}`),
	})
	if err != nil {
		t.Fatalf("apply to new leader: %v", err)
	}

	// 验证 FSM 状态
	fsm := clusters[newLeaderIdx].GetFSM()
	q, ok := fsm.GetQuota("test-user-2")
	if !ok {
		t.Fatal("quota not found in new leader FSM")
	}
	if q != 200.0 {
		t.Fatalf("expected quota 200.0, got %f", q)
	}
}
