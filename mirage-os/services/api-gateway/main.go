// Package main - API Gateway 服务入口
package main

import (
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	pb "mirage-os/api/proto"
	"mirage-os/pkg/database"
	"mirage-os/pkg/geo"
	"mirage-os/services/billing"
	"mirage-os/services/provisioning"

	"github.com/go-redis/redis/v8"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

func main() {
	log.Println("🚀 Mirage-OS API Gateway 启动中...")

	// 1. 连接数据库
	dbConfig := &database.Config{
		Host:     getEnv("DB_HOST", "localhost"),
		Port:     5432,
		User:     getEnv("DB_USER", "postgres"),
		Password: getEnv("DB_PASSWORD", "postgres"),
		DBName:   getEnv("DB_NAME", "mirage_os"),
		SSLMode:  "disable",
		TimeZone: "UTC",
	}

	if err := database.Connect(dbConfig); err != nil {
		log.Fatalf("❌ 数据库连接失败: %v", err)
	}
	defer database.Close()

	// 2. 执行数据库迁移
	if err := database.Migrate(); err != nil {
		log.Fatalf("❌ 数据库迁移失败: %v", err)
	}

	// 3. 初始化默认蜂窝
	if err := database.InitDefaultCells(); err != nil {
		log.Printf("⚠️  初始化默认蜂窝失败: %v", err)
	}

	// 4. 初始化 Redis（实时推送）
	rdb := redis.NewClient(&redis.Options{
		Addr: getEnv("REDIS_ADDR", "localhost:6379"),
	})
	log.Println("✅ Redis 连接已建立")

	// 5. 初始化 GeoIP（全球视野坐标对齐）
	geoipPath := getEnv("GEOIP_DB_PATH", "")
	locator, err := geo.NewLocator(geoipPath)
	if err != nil {
		log.Fatalf("❌ GeoIP 初始化失败: %v", err)
	}
	defer locator.Close()
	log.Println("🌍 GeoIP 定位服务已启用")

	// 6. 创建 gRPC 服务器
	grpcServer := grpc.NewServer()
	gatewayServer := NewServer(database.GetDB(), rdb, locator)

	// 7. 注册服务
	pb.RegisterGatewayServiceServer(grpcServer, gatewayServer)

	// 8. 启用反射（用于 grpcurl 调试）
	reflection.Register(grpcServer)

	// 9. 监听端口
	port := getEnv("GRPC_PORT", "50051")
	listener, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Fatalf("❌ 监听端口失败: %v", err)
	}

	log.Printf("✅ gRPC 服务器已启动，监听端口: %s", port)

	// 10.5 初始化 Provisioner（零触达自动化配置引擎）
	prov := provisioning.NewProvisioner(database.GetDB())
	prov.StartCleanupLoop(make(chan struct{}))

	// 10.6 初始化 MoneroManager 并绑定 Provisioner 回调
	moneroRPC := getEnv("MONERO_RPC_URL", "http://localhost:28081")
	moneroWalletRPC := getEnv("MONERO_WALLET_RPC_URL", "http://localhost:28083")
	xmrProcessor := billing.NewXMRProcessor(moneroRPC, moneroWalletRPC, 10)
	xmrProcessor.SetOnConfirmed(func(uid string, amount uint64) {
		if err := prov.OnXMRConfirmed(uid, amount); err != nil {
			log.Printf("❌ [Provisioner] 自动配置失败: %v", err)
		}
	})
	xmrProcessor.StartWatcher()
	log.Println("💰 XMR 支付监听 + 自动化配置引擎已启动")

	// 10.7 启动 Provisioner 内部 HTTP API（供 NestJS 调用）
	provMux := http.NewServeMux()
	provHandler := provisioning.NewHTTPHandler(prov)
	provHandler.RegisterRoutes(provMux)
	go func() {
		provAddr := getEnv("PROVISIONER_ADDR", ":18444")
		log.Printf("✅ Provisioner 内部 API: %s", provAddr)
		if err := http.ListenAndServe(provAddr, provMux); err != nil {
			log.Printf("⚠️ Provisioner HTTP 启动失败: %v", err)
		}
	}()

	// 10. 启动服务器（非阻塞）
	go func() {
		if err := grpcServer.Serve(listener); err != nil {
			log.Fatalf("❌ gRPC 服务器启动失败: %v", err)
		}
	}()

	// 11. 优雅退出
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Println("🛑 收到退出信号，正在关闭服务器...")
	grpcServer.GracefulStop()
	listener.Close()
	log.Println("✅ 服务器已安全退出")
}

// getEnv 获取环境变量，如果不存在则返回默认值
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
