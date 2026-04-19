package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	goredis "github.com/redis/go-redis/v9"

	"mirage-os/gateway-bridge/pkg/config"
	"mirage-os/gateway-bridge/pkg/crypto"
	"mirage-os/gateway-bridge/pkg/dispatch"
	grpcserver "mirage-os/gateway-bridge/pkg/grpc"
	"mirage-os/gateway-bridge/pkg/intel"
	"mirage-os/gateway-bridge/pkg/quota"
	raftpkg "mirage-os/gateway-bridge/pkg/raft"
	"mirage-os/gateway-bridge/pkg/rest"
	"mirage-os/gateway-bridge/pkg/store"
)

func main() {
	cfgPath := "configs/mirage-os.yaml"
	if p := os.Getenv("CONFIG_PATH"); p != "" {
		cfgPath = p
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Fatalf("[FATAL] load config: %v", err)
	}

	// 连接 PostgreSQL
	db, err := store.NewPostgres(cfg.Database.DSN)
	if err != nil {
		log.Fatalf("[FATAL] connect postgres: %v", err)
	}
	defer db.Close()
	log.Println("[INFO] PostgreSQL connected")

	// 连接 Redis
	rdb := goredis.NewClient(&goredis.Options{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
	ctx := context.Background()
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Printf("[WARN] redis connect failed: %v (running in degraded mode)", err)
	} else {
		log.Println("[INFO] Redis connected")
	}
	defer rdb.Close()

	// 初始化核心模块
	enforcer := quota.NewEnforcer(db, cfg.Quota)
	distributor := intel.NewDistributor(db, rdb, cfg.Intel)
	dispatcher := dispatch.NewStrategyDispatcher(rdb)

	// 加载已封禁 IP
	if err := distributor.LoadBannedIPs(); err != nil {
		log.Printf("[WARN] load banned IPs: %v", err)
	}

	// 启动 Redis 订阅
	subCtx, subCancel := context.WithCancel(ctx)
	defer subCancel()
	distributor.StartSubscriber(subCtx)

	// 启动 gRPC 服务
	srv := grpcserver.NewServer(cfg.GRPC, enforcer, distributor, dispatcher, db, rdb)
	if err := srv.Start(); err != nil {
		log.Fatalf("[FATAL] start grpc: %v", err)
	}

	// 启动内部 REST API（供 api-server 调用）
	restHandler := rest.NewHandler(enforcer, dispatcher, db, rdb)
	mux := http.NewServeMux()
	restHandler.RegisterRoutes(mux)
	restAddr := ":7000"
	if cfg.REST != nil && cfg.REST.Addr != "" {
		restAddr = cfg.REST.Addr
	}
	restServer := &http.Server{Addr: restAddr, Handler: mux}
	go func() {
		log.Printf("[INFO] REST API listening on %s", restAddr)
		if err := restServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("[WARN] REST server error: %v", err)
		}
	}()

	// 初始化 Raft 集群
	var cluster *raftpkg.Cluster
	if cfg.Raft.NodeID != "" {
		cluster, err = raftpkg.NewCluster(cfg.Raft)
		if err != nil {
			log.Fatalf("[FATAL] init raft cluster: %v", err)
		}
		if err := cluster.Start(); err != nil {
			log.Fatalf("[FATAL] start raft cluster: %v", err)
		}
		log.Printf("[INFO] Raft cluster started (node=%s, leader=%v)", cfg.Raft.NodeID, cluster.IsLeader())
	}

	// 初始化 Shamir + HotKey
	shamirEngine, _ := crypto.NewShamirEngine(3, 5)
	hotKey := crypto.NewHotKey()

	// 尝试从本地份额文件恢复热密钥
	sharePath := filepath.Join(cfg.Raft.DataDir, "shamir", "share.dat")
	if shareData, err := os.ReadFile(sharePath); err == nil && len(shareData) >= 33 {
		share := crypto.Share{X: shareData[0], Y: shareData[1:]}
		log.Printf("[INFO] loaded local Shamir share (x=%d)", share.X)
		// 实际部署中需要从其他节点收集足够份额
		_ = shamirEngine
		_ = share
	} else {
		log.Println("[INFO] no local Shamir share found, hot key inactive")
	}

	// 保持引用避免编译器优化
	_ = hotKey
	_ = cluster

	// 优雅退出
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("[INFO] shutting down...")

	// 停用热密钥
	hotKey.Deactivate()
	log.Println("[INFO] hot key deactivated")

	// 关闭 Raft 集群
	if cluster != nil {
		if err := cluster.Shutdown(); err != nil {
			log.Printf("[WARN] raft shutdown: %v", err)
		}
		log.Println("[INFO] raft cluster stopped")
	}

	srv.Stop()
	restServer.Close()
	subCancel()
	log.Println("[INFO] gateway-bridge stopped")
}
