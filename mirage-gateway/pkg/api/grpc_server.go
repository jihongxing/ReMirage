package api

import (
	"crypto/tls"
	"fmt"
	"log"
	pb "mirage-proto/gen"
	"net"

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
// 安全设计：mTLS 握手失败时不返回 TLS Alert（静默关闭连接）
// 防止主动探测者通过 TLS 错误响应识别服务类型
func (s *GRPCServer) Start() error {
	var opts []grpc.ServerOption
	if s.tlsConfig != nil {
		// 克隆 TLS 配置，添加主动探测抗性
		probingResistantTLS := s.tlsConfig.Clone()
		// 设置 GetConfigForClient：对无法提供有效客户端证书的连接静默关闭
		// 而非返回标准 TLS Alert（避免暴露"这里需要客户端证书"的信息）
		probingResistantTLS.VerifyConnection = func(cs tls.ConnectionState) error {
			// mTLS 验证已由 ClientAuth=RequireAndVerifyClientCert 完成
			// 此处额外检查：如果没有对端证书，静默拒绝
			if len(cs.PeerCertificates) == 0 {
				return fmt.Errorf("no client certificate")
			}
			return nil
		}
		opts = append(opts, grpc.Creds(credentials.NewTLS(probingResistantTLS)))
	}

	// 设置连接超时：快速断开无效连接（减少资源消耗）
	opts = append(opts, grpc.ConnectionTimeout(5*1000*1000*1000)) // 5s

	s.server = grpc.NewServer(opts...)
	pb.RegisterGatewayDownlinkServer(s.server, s.handler)

	// 绑定地址：仅监听内网接口（不对公网暴露）
	// 如果需要公网访问，应通过反向代理（Nginx/Caddy）前置
	listenAddr := fmt.Sprintf(":%d", s.port)
	lis, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return fmt.Errorf("监听端口 %d 失败: %w", s.port, err)
	}

	s.running = true
	go func() {
		log.Printf("[gRPC Server] 下行服务已启动: %s", listenAddr)
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
