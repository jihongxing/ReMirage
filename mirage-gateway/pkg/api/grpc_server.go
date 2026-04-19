package api

import (
	"crypto/tls"
	"fmt"
	"log"
	"net"

	"mirage-gateway/pkg/api/proto"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// GRPCServer 下行 gRPC 服务端
type GRPCServer struct {
	server    *grpc.Server
	handler   *CommandHandler
	port      int
	tlsConfig *tls.Config
	running   bool
}

// NewGRPCServer 创建服务端（mTLS 认证）
func NewGRPCServer(port int, tlsConfig *tls.Config, handler *CommandHandler) *GRPCServer {
	return &GRPCServer{
		handler:   handler,
		port:      port,
		tlsConfig: tlsConfig,
	}
}

// Start 启动 gRPC 服务
func (s *GRPCServer) Start() error {
	var opts []grpc.ServerOption
	if s.tlsConfig != nil {
		opts = append(opts, grpc.Creds(credentials.NewTLS(s.tlsConfig)))
	}

	s.server = grpc.NewServer(opts...)
	proto.RegisterGatewayDownlinkServer(s.server, s.handler)

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", s.port))
	if err != nil {
		return fmt.Errorf("监听端口 %d 失败: %w", s.port, err)
	}

	s.running = true
	go func() {
		log.Printf("[gRPC Server] 下行服务已启动: :%d", s.port)
		if err := s.server.Serve(lis); err != nil {
			log.Printf("[gRPC Server] 服务异常: %v", err)
		}
		s.running = false
	}()

	return nil
}

// IsRunning 是否运行中
func (s *GRPCServer) IsRunning() bool {
	return s.running
}

// Stop 优雅关闭
func (s *GRPCServer) Stop() {
	if s.server != nil {
		s.server.GracefulStop()
		s.running = false
		log.Println("[gRPC Server] 已关闭")
	}
}
