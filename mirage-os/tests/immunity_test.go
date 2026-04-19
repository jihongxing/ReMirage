package tests

import (
	"context"
	"testing"

	pb "mirage-os/gateway-bridge/proto"
)

// TestGlobalImmunity 全局免疫集成测试
func TestGlobalImmunity(t *testing.T) {
	env := setupBridge(t)
	defer env.cleanup()

	ctx := context.Background()
	gwID := "test-gw-immunity"
	sourceIP := "192.168.99.99"

	testDB.Exec("INSERT INTO gateways (id, status) VALUES ($1, 'ONLINE')", gwID)

	// 发送 100 次 ReportThreat 同一 IP
	for i := 0; i < 100; i++ {
		_, err := env.client.ReportThreat(ctx, &pb.ThreatRequest{
			GatewayId: gwID,
			Events: []*pb.ThreatEvent{{
				SourceIp:   sourceIP,
				SourcePort: 12345,
				ThreatType: pb.ThreatType_ACTIVE_PROBING,
				Severity:   5,
			}},
		})
		if err != nil {
			t.Fatalf("report threat %d: %v", i, err)
		}
	}

	// 查询 threat_intel 断言 is_banned == true
	var isBanned bool
	err := testDB.QueryRow(
		"SELECT BOOL_OR(is_banned) FROM threat_intel WHERE source_ip = $1", sourceIP,
	).Scan(&isBanned)
	if err != nil {
		t.Fatalf("query threat_intel: %v", err)
	}
	if !isBanned {
		t.Fatal("expected IP to be banned after 100 reports")
	}

	// 验证 Redis 黑名单
	exists, err := testRDB.SIsMember(ctx, "mirage:blacklist:global", sourceIP).Result()
	if err != nil {
		t.Fatalf("redis sismember: %v", err)
	}
	if !exists {
		t.Fatal("expected IP in Redis blacklist")
	}
}
