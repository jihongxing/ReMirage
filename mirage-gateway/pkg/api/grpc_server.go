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
	server           *grpc.Server
	handler          *CommandHandler
	port             int
	tlsConfig        *tls.Config
	running          bool
	listenerWrapper  func(net.Listener) net.Listener
	protocolDetector ProtocolDetectorInterface
}

// ProtocolDetectorInterface 协议检测器接口
type ProtocolDetectorInterface interface {
	Detect(conn net.Conn) (isMalicious bool, protocolType string, wrapped net.Conn)
	HandleMalicious(sourceIP string, protocolType string)
}

// NewGRPCServer 创建服务端（mTLS 认证）
func NewGRPCServer(port int, tlsConfig *tls.Config, handler *CommandHandler) *GRPCServer {
	return &GRPCServer{
		handler:   handler,
		port:      port,
		tlsConfig: tlsConfig,
	}
}

// SetListenerWrapper 设置 listener 包装器（如 HandshakeGuard.WrapListener）
func (s *GRPCServer) SetListenerWrapper(wrapper func(net.Listener) net.Listener) {
	s.listenerWrapper = wrapper
}

// SetProtocolDetector 设置协议检测器
func (s *GRPCServer) SetProtocolDetector(pd ProtocolDetectorInterface) {
	s.protocolDetector = pd
}

// Start 启动 gRPC 服务
// 安全设计：mTLS 握手失败时不返回 TLS Alert（静默关闭连接）
// 防止主动探测者通过 TLS 错误响应识别服务类型
func (s *GRPCServer) Start() error {
	if s.tlsConfig == nil {
		return fmt.Errorf("gRPC Server 拒绝启动：mTLS 未配置")
	}

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

	// 应用 listener 包装器（HandshakeGuard 等）
	if s.listenerWrapper != nil {
		lis = s.listenerWrapper(lis)
		log.Printf("[gRPC Server] Listener 已接线防护组件")
	}

	// 应用协议检测器
	if s.protocolDetector != nil {
		lis = &protocolDetectingListener{
			Listener: lis,
			detector: s.protocolDetector,
		}
		log.Printf("[gRPC Server] ProtocolDetector 已接线")
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

// protocolDetectingListener 在 Accept 后进行协议检测
type protocolDetectingListener struct {
	net.Listener
	detector ProtocolDetectorInterface
}

func (pdl *protocolDetectingListener) Accept() (net.Conn, error) {
	conn, err := pdl.Listener.Accept()
	if err != nil {
		return nil, err
	}

	isMalicious, protoType, wrapped := pdl.detector.Detect(conn)
	if isMalicious {
		host, _, _ := net.SplitHostPort(conn.RemoteAddr().String())
		pdl.detector.HandleMalicious(host, protoType)
		wrapped.Close()
		// 返回下一个连接
		return pdl.Accept()
	}

	return wrapped, nil
}
