package grpc

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"

	"mirage-os/gateway-bridge/pkg/config"
	"mirage-os/gateway-bridge/pkg/dispatch"
	"mirage-os/gateway-bridge/pkg/intel"
	"mirage-os/gateway-bridge/pkg/quota"
	pb "mirage-proto/gen"
)

type Server struct {
	pb.UnimplementedGatewayUplinkServer
	enforcer    *quota.Enforcer
	distributor *intel.Distributor
	dispatcher  *dispatch.StrategyDispatcher
	db          *sql.DB
	rdb         *goredis.Client
	grpcServer  *grpc.Server
	port        int
	tlsEnabled  bool
	tlsCert     string
	tlsKey      string
	tlsCA       string
}

func NewServer(cfg config.GRPCConfig, enforcer *quota.Enforcer,
	distributor *intel.Distributor, dispatcher *dispatch.StrategyDispatcher,
	db *sql.DB, rdb *goredis.Client) *Server {
	return &Server{
		enforcer:    enforcer,
		distributor: distributor,
		dispatcher:  dispatcher,
		db:          db,
		rdb:         rdb,
		port:        cfg.Port,
		tlsEnabled:  cfg.TLSEnabled,
		tlsCert:     cfg.CertFile,
		tlsKey:      cfg.KeyFile,
		tlsCA:       cfg.CAFile,
	}
}

func (s *Server) Start() error {
	var opts []grpc.ServerOption
	if s.tlsEnabled {
		creds, err := credentials.NewServerTLSFromFile(s.tlsCert, s.tlsKey)
		if err != nil {
			return fmt.Errorf("load tls: %w", err)
		}
		opts = append(opts, grpc.Creds(creds))
	}

	s.grpcServer = grpc.NewServer(opts...)
	pb.RegisterGatewayUplinkServer(s.grpcServer, s)

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", s.port))
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	log.Printf("[INFO] gRPC server listening on :%d (TLS=%v)", s.port, s.tlsEnabled)
	go func() {
		if err := s.grpcServer.Serve(lis); err != nil {
			log.Printf("[ERROR] grpc serve: %v", err)
		}
	}()
	return nil
}

func (s *Server) Stop() {
	if s.grpcServer != nil {
		s.grpcServer.GracefulStop()
	}
}

// SyncHeartbeat 处理心跳
func (s *Server) SyncHeartbeat(ctx context.Context, req *pb.HeartbeatRequest) (*pb.HeartbeatResponse, error) {
	if req.GatewayId == "" {
		return nil, status.Error(codes.InvalidArgument, "gateway_id is required")
	}
	if req.Timestamp == 0 {
		return nil, status.Error(codes.InvalidArgument, "timestamp is required")
	}

	// UPSERT gateways 表
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO gateways (id, status, last_heartbeat, ebpf_loaded, threat_level, active_connections, memory_usage_mb, updated_at)
		VALUES ($1, $2, NOW(), $3, $4, $5, $6, NOW())
		ON CONFLICT (id) DO UPDATE SET
			status = EXCLUDED.status,
			last_heartbeat = NOW(),
			ebpf_loaded = EXCLUDED.ebpf_loaded,
			threat_level = EXCLUDED.threat_level,
			active_connections = EXCLUDED.active_connections,
			memory_usage_mb = EXCLUDED.memory_usage_mb,
			updated_at = NOW()
	`, req.GatewayId, mapStatus(req.Status), req.EbpfLoaded, req.ThreatLevel, req.ActiveConnections, req.MemoryUsageMb)
	if err != nil {
		log.Printf("[ERROR] update gateway: %v", err)
		return nil, status.Error(codes.Internal, "database error")
	}

	// Redis 缓存在线状态
	rctx := context.Background()
	s.rdb.Set(rctx, fmt.Sprintf("gateway:%s:status", req.GatewayId), "ONLINE", 60*time.Second)

	// 查询剩余配额
	remainingQuota, err := s.enforcer.GetRemainingQuota(req.GatewayId)
	if err != nil {
		// 非致命错误，返回 0
		remainingQuota = 0
	}

	// 重试待推送策略
	_ = s.dispatcher.RetryPending(req.GatewayId)

	return &pb.HeartbeatResponse{
		Ack:            true,
		ServerTime:     time.Now().Unix(),
		RemainingQuota: remainingQuota,
	}, nil
}

// ReportTraffic 处理流量上报
func (s *Server) ReportTraffic(ctx context.Context, req *pb.TrafficRequest) (*pb.TrafficResponse, error) {
	if req.GatewayId == "" {
		return nil, status.Error(codes.InvalidArgument, "gateway_id is required")
	}

	_, err := s.enforcer.Settle(req.GatewayId, req.BusinessBytes, req.DefenseBytes, req.PeriodSeconds)
	if err != nil {
		log.Printf("[ERROR] settle traffic: %v", err)
		return nil, status.Error(codes.Internal, "settlement error")
	}

	return &pb.TrafficResponse{Ack: true}, nil
}

// ReportThreat 处理威胁上报
func (s *Server) ReportThreat(ctx context.Context, req *pb.ThreatRequest) (*pb.ThreatResponse, error) {
	if req.GatewayId == "" {
		return nil, status.Error(codes.InvalidArgument, "gateway_id is required")
	}

	for _, event := range req.Events {
		if err := s.distributor.RecordThreat(event, req.GatewayId); err != nil {
			log.Printf("[ERROR] record threat: %v", err)
			continue
		}
		if _, err := s.distributor.CheckAndBan(event.SourceIp); err != nil {
			log.Printf("[ERROR] check ban: %v", err)
		}
	}

	return &pb.ThreatResponse{Ack: true}, nil
}

func mapStatus(s pb.GatewayStatus) string {
	switch s {
	case pb.GatewayStatus_ONLINE:
		return "ONLINE"
	case pb.GatewayStatus_DEGRADED:
		return "DEGRADED"
	default:
		return "OFFLINE"
	}
}
