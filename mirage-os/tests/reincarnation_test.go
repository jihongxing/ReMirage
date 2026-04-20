package tests

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "mirage-proto/gen"
)

// mockDownlinkServer 模拟 Gateway 下行服务
type mockDownlinkServer struct {
	pb.UnimplementedGatewayDownlinkServer
	reincarnation *pb.ReincarnationPush
}

func (s *mockDownlinkServer) PushReincarnation(_ context.Context, req *pb.ReincarnationPush) (*pb.PushResponse, error) {
	s.reincarnation = req
	return &pb.PushResponse{Success: true}, nil
}

func (s *mockDownlinkServer) PushStrategy(_ context.Context, _ *pb.StrategyPush) (*pb.PushResponse, error) {
	return &pb.PushResponse{Success: true}, nil
}

func (s *mockDownlinkServer) PushBlacklist(_ context.Context, _ *pb.BlacklistPush) (*pb.PushResponse, error) {
	return &pb.PushResponse{Success: true}, nil
}

func (s *mockDownlinkServer) PushQuota(_ context.Context, _ *pb.QuotaPush) (*pb.PushResponse, error) {
	return &pb.PushResponse{Success: true}, nil
}

// TestDomainReincarnation 域名转生集成测试
func TestDomainReincarnation(t *testing.T) {
	env := setupBridge(t)
	defer env.cleanup()

	gwID := "test-gw-reincarnation"

	// 启动 mock downlink server
	mock := &mockDownlinkServer{}
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	mockSrv := grpc.NewServer()
	pb.RegisterGatewayDownlinkServer(mockSrv, mock)
	go mockSrv.Serve(lis)
	defer mockSrv.GracefulStop()

	downlinkAddr := lis.Addr().String()

	// 注册 Gateway
	testDB.Exec("INSERT INTO gateways (id, status) VALUES ($1, 'ONLINE')", gwID)
	if err := env.dispatcher.RegisterGateway(gwID, downlinkAddr); err != nil {
		t.Fatalf("register gateway: %v", err)
	}

	// 推送转生指令
	ctx := context.Background()
	conn, err := grpc.NewClient(downlinkAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial downlink: %v", err)
	}
	defer conn.Close()

	client := pb.NewGatewayDownlinkClient(conn)
	resp, err := client.PushReincarnation(ctx, &pb.ReincarnationPush{
		NewDomain:       fmt.Sprintf("new-%d.mirage.internal", time.Now().Unix()),
		NewIp:           "10.0.0.100",
		DeadlineSeconds: 300,
	})
	if err != nil {
		t.Fatalf("push reincarnation: %v", err)
	}
	if !resp.Success {
		t.Fatal("expected success=true")
	}

	// 验证 mock 收到指令
	if mock.reincarnation == nil {
		t.Fatal("mock did not receive reincarnation push")
	}
	if mock.reincarnation.NewDomain == "" {
		t.Fatal("expected non-empty new_domain")
	}
	if mock.reincarnation.NewIp == "" {
		t.Fatal("expected non-empty new_ip")
	}
	if mock.reincarnation.DeadlineSeconds <= 0 {
		t.Fatal("expected deadline_seconds > 0")
	}
}
