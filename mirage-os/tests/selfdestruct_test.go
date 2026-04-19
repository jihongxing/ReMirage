package tests

import (
	"context"
	"testing"
	"time"

	pb "mirage-os/gateway-bridge/proto"
)

// TestNodeSelfDestruct 节点自毁集成测试
func TestNodeSelfDestruct(t *testing.T) {
	env := setupBridge(t)
	defer env.cleanup()

	ctx := context.Background()
	gwID := "test-gw-selfdestruct"

	testDB.Exec("INSERT INTO gateways (id, status, last_heartbeat) VALUES ($1, 'ONLINE', NOW())", gwID)

	// 发送心跳确认在线
	resp, err := env.client.SyncHeartbeat(ctx, &pb.HeartbeatRequest{
		GatewayId: gwID,
		Timestamp: time.Now().Unix(),
		Status:    pb.GatewayStatus_ONLINE,
	})
	if err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	if !resp.Ack {
		t.Fatal("expected ack=true")
	}

	// 模拟心跳超时：直接更新数据库中的 last_heartbeat 为过去
	_, err = testDB.Exec(
		"UPDATE gateways SET last_heartbeat = NOW() - INTERVAL '310 seconds', status = 'OFFLINE' WHERE id = $1",
		gwID,
	)
	if err != nil {
		t.Fatalf("update heartbeat: %v", err)
	}

	// 验证状态变为 OFFLINE
	var status string
	err = testDB.QueryRow("SELECT status FROM gateways WHERE id = $1", gwID).Scan(&status)
	if err != nil {
		t.Fatalf("query status: %v", err)
	}
	if status != "OFFLINE" {
		t.Fatalf("expected OFFLINE, got %s", status)
	}

	// 验证心跳超时（last_heartbeat 超过 300 秒）
	var heartbeatAge float64
	err = testDB.QueryRow(
		"SELECT EXTRACT(EPOCH FROM (NOW() - last_heartbeat)) FROM gateways WHERE id = $1", gwID,
	).Scan(&heartbeatAge)
	if err != nil {
		t.Fatalf("query heartbeat age: %v", err)
	}
	if heartbeatAge < 300 {
		t.Fatalf("expected heartbeat age >= 300s, got %.0fs", heartbeatAge)
	}
}
