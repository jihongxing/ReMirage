package tests

import (
	"context"
	"testing"
	"time"

	pb "mirage-os/gateway-bridge/proto"
)

// TestLifecycleRuling 生死裁决集成测试
func TestLifecycleRuling(t *testing.T) {
	env := setupBridge(t)
	defer env.cleanup()

	ctx := context.Background()

	// 创建 cell + user + gateway
	cellID := "test-cell-lifecycle"
	userID := "test-user-lifecycle"
	gwID := "test-gw-lifecycle"

	testDB.Exec("INSERT INTO cells (id, name, region, cost_multiplier) VALUES ($1, 'Test Cell', 'test', 1.0)", cellID)
	testDB.Exec("INSERT INTO users (id, cell_id, remaining_quota, is_active) VALUES ($1, $2, 1.0, true)", userID, cellID)
	testDB.Exec("INSERT INTO gateways (id, cell_id, status) VALUES ($1, $2, 'ONLINE')", gwID, cellID)

	// 发送 ReportTraffic 消耗全部配额（10GB * $0.10/GB = $1.00）
	_, err := env.client.ReportTraffic(ctx, &pb.TrafficRequest{
		GatewayId:     gwID,
		BusinessBytes: 10_000_000_000, // 10 GB
		DefenseBytes:  0,
		PeriodSeconds: 60,
	})
	if err != nil {
		t.Fatalf("report traffic: %v", err)
	}

	// SyncHeartbeat 验证配额归零
	resp, err := env.client.SyncHeartbeat(ctx, &pb.HeartbeatRequest{
		GatewayId: gwID,
		Timestamp: time.Now().Unix(),
		Status:    pb.GatewayStatus_ONLINE,
	})
	if err != nil {
		t.Fatalf("sync heartbeat: %v", err)
	}

	if resp.RemainingQuota > 0 {
		t.Fatalf("expected remaining_quota <= 0, got %f", resp.RemainingQuota)
	}
}
