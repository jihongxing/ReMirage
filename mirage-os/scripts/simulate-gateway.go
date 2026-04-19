// 模拟 Gateway 心跳数据生成器
// 用法: go run scripts/simulate-gateway.go
package main

import (
	"context"
	"log"
	"math/rand"
	"time"

	pb "mirage-os/api/proto"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	gatewayIDs = []string{
		"gw-iceland-001",
		"gw-switzerland-002",
		"gw-singapore-003",
	}
	cellIDs = []string{
		"cell-is-standard-01",
		"cell-ch-platinum-01",
		"cell-sg-diamond-01",
	}
	regions = []string{
		"EU-North",
		"EU-Central",
		"APAC-South",
	}
	threatIPs = []string{
		"45.33.32.156",
		"185.220.101.1",
		"91.121.87.18",
		"104.244.72.115",
		"198.51.100.23",
	}
)

func main() {
	conn, err := grpc.Dial("localhost:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("连接失败: %v", err)
	}
	defer conn.Close()

	client := pb.NewGatewayServiceClient(conn)
	ctx := context.Background()

	log.Println("🚀 Gateway 模拟器启动")
	log.Println("📡 每 5 秒发送心跳，每 3 秒上报流量，随机触发威胁")

	// 心跳协程
	go func() {
		for {
			for i, gwID := range gatewayIDs {
				sendHeartbeat(ctx, client, gwID, cellIDs[i], regions[i])
			}
			time.Sleep(5 * time.Second)
		}
	}()

	// 流量上报协程
	go func() {
		for {
			for i, gwID := range gatewayIDs {
				sendTraffic(ctx, client, gwID, cellIDs[i])
			}
			time.Sleep(3 * time.Second)
		}
	}()

	// 威胁上报协程（随机触发）
	go func() {
		for {
			time.Sleep(time.Duration(2+rand.Intn(8)) * time.Second)
			gwID := gatewayIDs[rand.Intn(len(gatewayIDs))]
			sendThreat(ctx, client, gwID)
		}
	}()

	// 保持运行
	select {}
}

func sendHeartbeat(ctx context.Context, client pb.GatewayServiceClient, gwID, cellID, region string) {
	req := &pb.HeartbeatRequest{
		GatewayId:          gwID,
		Version:            "2.1.0",
		CurrentThreatLevel: uint32(rand.Intn(3)),
		Status: &pb.GatewayStatus{
			Online:            true,
			ActiveConnections: uint32(50 + rand.Intn(200)),
			UptimeSeconds:     uint64(3600 + rand.Intn(86400)),
			CellId:            cellID,
			Region:            region,
		},
		Resource: &pb.ResourceUsage{
			CpuPercent:   float32(10 + rand.Intn(40)),
			MemoryBytes:  uint64(500_000_000 + rand.Intn(1_000_000_000)),
			BandwidthBps: uint64(100_000_000 + rand.Intn(900_000_000)),
		},
		Timestamp: time.Now().Unix(),
	}

	resp, err := client.SyncHeartbeat(ctx, req)
	if err != nil {
		log.Printf("❌ [%s] 心跳失败: %v", gwID, err)
		return
	}
	log.Printf("💓 [%s] 心跳成功 - 配额: %d bytes, 防御等级: %d",
		gwID, resp.RemainingQuota, resp.DefenseConfig.GetDefenseLevel())
}

func sendTraffic(ctx context.Context, client pb.GatewayServiceClient, gwID, cellID string) {
	cellLevel := "standard"
	if cellID == "cell-ch-platinum-01" {
		cellLevel = "platinum"
	} else if cellID == "cell-sg-diamond-01" {
		cellLevel = "diamond"
	}

	baseTraffic := uint64(10_000_000 + rand.Intn(90_000_000))
	defenseTraffic := uint64(float64(baseTraffic) * (0.1 + rand.Float64()*0.3))

	req := &pb.TrafficReport{
		GatewayId:           gwID,
		Timestamp:           time.Now().Unix(),
		BaseTrafficBytes:    baseTraffic,
		DefenseTrafficBytes: defenseTraffic,
		CellLevel:           cellLevel,
		Breakdown: &pb.TrafficBreakdown{
			NpmPaddingBytes: defenseTraffic / 3,
			VpcNoiseBytes:   defenseTraffic / 3,
			GtunnelFecBytes: defenseTraffic / 3,
		},
	}

	resp, err := client.ReportTraffic(ctx, req)
	if err != nil {
		log.Printf("❌ [%s] 流量上报失败: %v", gwID, err)
		return
	}
	log.Printf("📊 [%s] 流量上报 - 业务: %.2f MB, 防御: %.2f MB, 费用: $%.4f",
		gwID,
		float64(baseTraffic)/1_000_000,
		float64(defenseTraffic)/1_000_000,
		resp.CurrentCostUsd)
}

func sendThreat(ctx context.Context, client pb.GatewayServiceClient, gwID string) {
	threatTypes := []pb.ThreatType{
		pb.ThreatType_THREAT_ACTIVE_PROBING,
		pb.ThreatType_THREAT_JA4_SCAN,
		pb.ThreatType_THREAT_SNI_PROBE,
		pb.ThreatType_THREAT_DPI_INSPECTION,
		pb.ThreatType_THREAT_TIMING_ATTACK,
	}

	req := &pb.ThreatReport{
		GatewayId:   gwID,
		Timestamp:   time.Now().UnixNano(),
		ThreatType:  threatTypes[rand.Intn(len(threatTypes))],
		SourceIp:    threatIPs[rand.Intn(len(threatIPs))],
		SourcePort:  uint32(1024 + rand.Intn(64000)),
		Severity:    uint32(3 + rand.Intn(7)),
		PacketCount: uint32(10 + rand.Intn(1000)),
	}

	resp, err := client.ReportThreat(ctx, req)
	if err != nil {
		log.Printf("❌ [%s] 威胁上报失败: %v", gwID, err)
		return
	}
	log.Printf("🚨 [%s] 威胁检测 - 类型: %s, IP: %s, 动作: %s",
		gwID, req.ThreatType, req.SourceIp, resp.Action)
}
