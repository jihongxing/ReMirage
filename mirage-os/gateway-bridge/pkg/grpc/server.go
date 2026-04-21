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
	"mirage-os/gateway-bridge/pkg/topology"
	pb "mirage-proto/gen"
)

type Server struct {
	pb.UnimplementedGatewayUplinkServer
	enforcer        *quota.Enforcer
	distributor     *intel.Distributor
	dispatcher      *dispatch.StrategyDispatcher
	blacklistSyncer *dispatch.BlacklistSyncer
	quotaDispatcher *dispatch.QuotaDispatcher
	registry        *topology.Registry
	downlink        *DownlinkService
	db              *sql.DB
	rdb             *goredis.Client
	grpcServer      *grpc.Server
	port            int
	tlsEnabled      bool
	tlsCert         string
	tlsKey          string
	tlsCA           string
	allowedCNs      []string
}

func NewServer(cfg config.GRPCConfig, enforcer *quota.Enforcer,
	distributor *intel.Distributor, dispatcher *dispatch.StrategyDispatcher,
	db *sql.DB, rdb *goredis.Client, registry *topology.Registry) *Server {
	syncer := dispatch.NewBlacklistSyncer(db, dispatcher)
	quotaDisp := dispatch.NewQuotaDispatcher(db, dispatcher)
	connMgr := NewGatewayConnectionManager(rdb)
	downlink := NewDownlinkService(connMgr, rdb)
	return &Server{
		enforcer:        enforcer,
		distributor:     distributor,
		dispatcher:      dispatcher,
		blacklistSyncer: syncer,
		quotaDispatcher: quotaDisp,
		registry:        registry,
		downlink:        downlink,
		db:              db,
		rdb:             rdb,
		port:            cfg.Port,
		tlsEnabled:      cfg.TLSEnabled,
		tlsCert:         cfg.CertFile,
		tlsKey:          cfg.KeyFile,
		tlsCA:           cfg.CAFile,
		allowedCNs:      cfg.AllowedCNs,
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

		// TLS 启用且配置了 CN 白名单时，注入 CN 校验拦截器
		if len(s.allowedCNs) > 0 {
			opts = append(opts,
				grpc.UnaryInterceptor(CNWhitelistInterceptor(s.allowedCNs)),
				grpc.StreamInterceptor(CNWhitelistStreamInterceptor(s.allowedCNs)),
			)
			log.Printf("[INFO] gRPC CN whitelist enabled: %v", s.allowedCNs)
		}
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

	// 更新拓扑索引
	s.registry.UpdateHeartbeat(req.GatewayId, req.ActiveSessions, req.StateHash)

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

	// 黑名单一致性校验：比对 Gateway 上报的黑名单条目数与 OS 侧记录
	// TODO: 待 proto 增加 blacklist_count / blacklist_updated_at 字段后，
	//       使用专用字段替代 ActiveConnections 临时承载
	go s.checkBlacklistConsistency(req.GatewayId, int(req.ActiveConnections))

	// 检查心跳超时的 Gateway，批量标记其会话为 disconnected
	go s.markStaleGatewaySessions()

	// 状态对齐检查
	needsSync := false
	desiredHash := ""
	if req.StateHash != "" {
		_, expectedHash, err := s.downlink.GetDesiredState(ctx, req.GatewayId)
		if err == nil && expectedHash != req.StateHash {
			needsSync = true
			desiredHash = expectedHash
			log.Printf("[INFO] state_hash mismatch for %s: expected=%s, got=%s", req.GatewayId, expectedHash, req.StateHash)
		}
	}

	return &pb.HeartbeatResponse{
		Ack:              true,
		ServerTime:       time.Now().Unix(),
		RemainingQuota:   remainingQuota,
		NeedsFullSync:    needsSync,
		DesiredStateHash: desiredHash,
	}, nil
}

// ReportTraffic 处理流量上报
func (s *Server) ReportTraffic(ctx context.Context, req *pb.TrafficRequest) (*pb.TrafficResponse, error) {
	if req.GatewayId == "" {
		return nil, status.Error(codes.InvalidArgument, "gateway_id is required")
	}

	// 幂等去重：基于 gateway_id + sequence_number
	if req.SequenceNumber > 0 {
		var exists bool
		err := s.db.QueryRowContext(ctx,
			`SELECT EXISTS(SELECT 1 FROM billing_logs WHERE gateway_id = $1 AND sequence_number = $2)`,
			req.GatewayId, req.SequenceNumber).Scan(&exists)
		if err == nil && exists {
			return &pb.TrafficResponse{Ack: true}, nil
		}
	}

	// 优先使用 user_id 直接定位用户
	userID := req.UserId
	_, err := s.enforcer.SettleForUser(req.GatewayId, userID, req.BusinessBytes, req.DefenseBytes, req.PeriodSeconds, req.SessionId, req.SequenceNumber)
	if err != nil {
		log.Printf("[ERROR] settle traffic: %v", err)
		return nil, status.Error(codes.Internal, "settlement error")
	}

	// 配额变更后触发对应用户的配额下发
	if userID != "" {
		go s.quotaDispatcher.PushQuotaForUser(context.Background(), userID)
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

		// 评估全局封禁：同一 source_ip 被 >= 3 个不同 Gateway 上报 → 全局封禁 + 黑名单同步
		banned, err := s.evaluateMultiGatewayBan(ctx, event.SourceIp)
		if err != nil {
			log.Printf("[ERROR] evaluate multi-gateway ban: %v", err)
		}
		if banned {
			log.Printf("[INFO] global ban triggered for %s, syncing blacklist", event.SourceIp)
			if syncErr := s.blacklistSyncer.SyncSingleIP(event.SourceIp); syncErr != nil {
				log.Printf("[ERROR] blacklist sync for %s: %v", event.SourceIp, syncErr)
			}
		}
	}

	return &pb.ThreatResponse{Ack: true}, nil
}

// evaluateMultiGatewayBan 检查同一 source_ip 是否被 >= 3 个不同 Gateway 上报，如果是则标记全局封禁
func (s *Server) evaluateMultiGatewayBan(ctx context.Context, sourceIP string) (bool, error) {
	var distinctCount int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(DISTINCT reported_by_gateway)
		FROM threat_intel
		WHERE source_ip = $1 AND reported_by_gateway IS NOT NULL
	`, sourceIP).Scan(&distinctCount)
	if err != nil {
		return false, fmt.Errorf("count distinct gateways: %w", err)
	}

	if distinctCount >= 3 {
		// 检查是否已经封禁
		var alreadyBanned bool
		err = s.db.QueryRowContext(ctx, `
			SELECT BOOL_OR(is_banned) FROM threat_intel WHERE source_ip = $1
		`, sourceIP).Scan(&alreadyBanned)
		if err != nil {
			return false, fmt.Errorf("check existing ban: %w", err)
		}
		if alreadyBanned {
			return false, nil // 已封禁，无需重复操作
		}

		_, err = s.db.ExecContext(ctx, `
			UPDATE threat_intel SET is_banned = true WHERE source_ip = $1
		`, sourceIP)
		if err != nil {
			return false, fmt.Errorf("set global ban: %w", err)
		}
		return true, nil
	}

	return false, nil
}

// checkBlacklistConsistency 校验 Gateway 上报的黑名单条目数与 OS 侧记录是否一致
// 不一致时触发全量黑名单同步
func (s *Server) checkBlacklistConsistency(gatewayID string, reportedCount int) {
	osCount, _, err := s.blacklistSyncer.GetBannedSummary()
	if err != nil {
		log.Printf("[WARN] get banned summary for consistency check: %v", err)
		return
	}

	if reportedCount != osCount {
		log.Printf("[INFO] blacklist inconsistency detected for gateway %s: reported=%d, os=%d, triggering full sync",
			gatewayID, reportedCount, osCount)
		if syncErr := s.blacklistSyncer.SyncAll(); syncErr != nil {
			log.Printf("[ERROR] full blacklist sync failed: %v", syncErr)
		}
	}
}

// markStaleGatewaySessions 检查心跳超时的 Gateway，批量标记其会话为 disconnected
func (s *Server) markStaleGatewaySessions() {
	ctx := context.Background()
	// 查找心跳超时超过 90 秒的 Gateway
	rows, err := s.db.QueryContext(ctx, `
		SELECT id FROM gateways
		WHERE last_heartbeat < NOW() - INTERVAL '90 seconds'
		  AND status != 'OFFLINE'
	`)
	if err != nil {
		log.Printf("[ERROR] query stale gateways: %v", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var gatewayID string
		if err := rows.Scan(&gatewayID); err != nil {
			continue
		}
		result, err := s.db.ExecContext(ctx, `
			UPDATE gateway_sessions SET status = 'disconnected', disconnected_at = NOW(), updated_at = NOW()
			WHERE gateway_id = $1 AND status = 'active'
		`, gatewayID)
		if err != nil {
			log.Printf("[ERROR] mark stale sessions for gateway %s: %v", gatewayID, err)
			continue
		}
		affected, _ := result.RowsAffected()
		if affected > 0 {
			log.Printf("[INFO] marked %d sessions as disconnected for stale gateway %s", affected, gatewayID)
		}
	}
}

// ReportSessionEvent 处理会话事件上报
func (s *Server) ReportSessionEvent(ctx context.Context, req *pb.SessionEventRequest) (*pb.SessionEventResponse, error) {
	if req.GatewayId == "" || req.SessionId == "" {
		return nil, status.Error(codes.InvalidArgument, "gateway_id and session_id required")
	}

	switch req.EventType {
	case pb.SessionEventType_SESSION_CONNECTED:
		_, err := s.db.ExecContext(ctx, `
			INSERT INTO gateway_sessions (id, session_id, gateway_id, user_id, client_id, status, connected_at, created_at, updated_at)
			VALUES (gen_random_uuid(), $1, $2, $3, $4, 'active', NOW(), NOW(), NOW())
			ON CONFLICT (session_id) DO UPDATE SET gateway_id = $2, status = 'active', disconnected_at = NULL, updated_at = NOW()
		`, req.SessionId, req.GatewayId, req.UserId, req.ClientId)
		if err != nil {
			log.Printf("[ERROR] upsert gateway_session: %v", err)
			return nil, status.Error(codes.Internal, "db error")
		}
		_, err = s.db.ExecContext(ctx, `
			INSERT INTO client_sessions (id, session_id, client_id, user_id, current_gateway_id, status, created_at, updated_at)
			VALUES (gen_random_uuid(), $1, $2, $3, $4, 'active', NOW(), NOW())
			ON CONFLICT (session_id) DO UPDATE SET current_gateway_id = $4, status = 'active', updated_at = NOW()
		`, req.SessionId, req.ClientId, req.UserId, req.GatewayId)
		if err != nil {
			log.Printf("[ERROR] upsert client_session: %v", err)
			return nil, status.Error(codes.Internal, "db error")
		}

	case pb.SessionEventType_SESSION_DISCONNECTED:
		_, err := s.db.ExecContext(ctx, `
			UPDATE gateway_sessions SET status = 'disconnected', disconnected_at = NOW(), updated_at = NOW() WHERE session_id = $1
		`, req.SessionId)
		if err != nil {
			log.Printf("[ERROR] disconnect gateway_session: %v", err)
		}
		_, err = s.db.ExecContext(ctx, `
			UPDATE client_sessions SET status = 'disconnected', updated_at = NOW() WHERE session_id = $1
		`, req.SessionId)
		if err != nil {
			log.Printf("[ERROR] disconnect client_session: %v", err)
		}
	}

	return &pb.SessionEventResponse{Ack: true}, nil
}

// RegisterGateway 处理 Gateway 注册
func (s *Server) RegisterGateway(ctx context.Context, req *pb.RegisterRequest) (*pb.RegisterResponse, error) {
	if req.GatewayId == "" {
		return nil, status.Error(codes.InvalidArgument, "gateway_id is required")
	}

	info := &topology.GatewayInfo{
		GatewayID:    req.GatewayId,
		CellID:       req.CellId,
		DownlinkAddr: req.DownlinkAddr,
		Version:      req.Version,
	}
	if req.Capabilities != nil {
		info.EBPFSupported = req.Capabilities.EbpfSupported
		info.MaxConnections = req.Capabilities.MaxConnections
		info.MaxSessions = req.Capabilities.MaxSessions
	}

	if err := s.registry.Register(ctx, info); err != nil {
		return nil, status.Errorf(codes.Internal, "register failed: %v", err)
	}

	// Also register in StrategyDispatcher for downlink connections
	if err := s.dispatcher.RegisterGateway(req.GatewayId, req.DownlinkAddr); err != nil {
		log.Printf("[WARN] dispatcher register gateway %s: %v", req.GatewayId, err)
	}

	return &pb.RegisterResponse{
		Success:        true,
		Message:        "registered",
		AssignedCellId: req.CellId,
	}, nil
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
