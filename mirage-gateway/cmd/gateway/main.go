package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"mirage-gateway/pkg/api"
	"mirage-gateway/pkg/api/proto"
	"mirage-gateway/pkg/cortex"
	"mirage-gateway/pkg/ebpf"
	"mirage-gateway/pkg/gswitch"
	"mirage-gateway/pkg/phantom"
	"mirage-gateway/pkg/security"
	"mirage-gateway/pkg/strategy"
	"mirage-gateway/pkg/threat"
	"mirage-gateway/pkg/tproxy"

	"go.yaml.in/yaml/v2"
)

var (
	configPath = flag.String("config", "configs/gateway.yaml", "配置文件路径")
	healthPort = flag.Int("health-port", 8081, "健康检查端口")
)

// GatewayConfig 网关配置
type GatewayConfig struct {
	GatewayID string `yaml:"gateway_id"` // 空则自动生成
	Network   struct {
		Interface string `yaml:"interface"`
	} `yaml:"network"`
	Defense struct {
		Level          int           `yaml:"level"`
		AutoAdjust     bool          `yaml:"auto_adjust"`
		UpdateInterval time.Duration `yaml:"update_interval"`
	} `yaml:"defense"`
	MCC struct {
		Endpoint string             `yaml:"endpoint"`
		TLS      security.TLSConfig `yaml:"tls"`
	} `yaml:"mcc"`
	Security SecurityConfig `yaml:"security"`
	Phantom  struct {
		HoneypotIP string `yaml:"honeypot_ip"`
	} `yaml:"phantom"`
	TPROXY struct {
		ListenAddr string `yaml:"listen_addr"`
	} `yaml:"tproxy"`
}

// SecurityConfig 安全加固配置
type SecurityConfig struct {
	RAMShield struct {
		Enabled           bool          `yaml:"enabled"`
		DisableCoreDump   bool          `yaml:"disable_core_dump"`
		CheckSwapInterval time.Duration `yaml:"check_swap_interval"`
	} `yaml:"ram_shield"`
	CertPinning struct {
		Enabled    bool   `yaml:"enabled"`
		PresetHash string `yaml:"preset_hash"`
	} `yaml:"cert_pinning"`
	AntiDebug struct {
		Enabled       bool          `yaml:"enabled"`
		CheckInterval time.Duration `yaml:"check_interval"`
	} `yaml:"anti_debug"`
	GracefulShutdown struct {
		Timeout time.Duration `yaml:"timeout"`
	} `yaml:"graceful_shutdown"`
}

// EnhancedHealthStatus 增强健康状态
type EnhancedHealthStatus struct {
	Status              string `json:"status"`
	EBPFLoaded          bool   `json:"ebpf_loaded"`
	GRPCClientConnected bool   `json:"grpc_client_connected"`
	GRPCServerRunning   bool   `json:"grpc_server_running"`
	ThreatLevel         int    `json:"threat_level"`
	BlacklistCount      int    `json:"blacklist_count"`
	Uptime              string `json:"uptime"`
}

func main() {
	flag.Parse()

	log.Println("🚀 Mirage-Gateway 启动中...")

	// 1. 加载配置
	cfg := loadConfig(*configPath)

	// 全局 context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 1.5 Gateway ID 动态化
	gatewayID := resolveGatewayID(cfg)
	log.Printf("🆔 Gateway ID: %s", gatewayID)

	// 2. 安全加固初始化
	ramShield := security.NewRAMShield()
	if cfg.Security.RAMShield.Enabled || cfg.Security.RAMShield.DisableCoreDump {
		if err := ramShield.DisableCoreDump(); err != nil {
			log.Printf("⚠️ 禁用 core dump 失败（降级运行）: %v", err)
		}
		swapKB, err := ramShield.CheckSwapUsage()
		if err != nil {
			log.Printf("⚠️ swap 检查失败: %v", err)
		} else if swapKB > 0 {
			log.Printf("⚠️ 检测到 swap 使用: %d KB", swapKB)
		}
		log.Println("✅ RAM Shield 已初始化")
	}

	// 证书钉扎
	var certPin *security.CertPin
	if cfg.Security.CertPinning.Enabled {
		certPin = security.NewCertPin(cfg.Security.CertPinning.PresetHash)
		log.Println("✅ 证书钉扎已初始化")
	}

	// 反调试
	var antiDebug *security.AntiDebug
	if cfg.Security.AntiDebug.Enabled {
		interval := cfg.Security.AntiDebug.CheckInterval
		if interval <= 0 {
			interval = 30 * time.Second
		}
		antiDebug = security.NewAntiDebug(interval)
		if err := antiDebug.StartMonitor(ctx); err != nil {
			log.Printf("⚠️ 反调试启动失败: %v", err)
		} else {
			log.Println("✅ 反调试检测已启动")
		}
	}

	// 3. mTLS 初始化（关键模块，失败终止）
	tlsMgr, err := security.NewTLSManager(cfg.MCC.TLS)
	if err != nil {
		log.Fatalf("❌ mTLS 初始化失败: %v", err)
	}
	tlsMgr.StartCertWatcher(ctx)
	log.Println("✅ mTLS 已初始化")

	// 4. eBPF 全量加载（关键模块，失败终止）
	loader := ebpf.NewLoader(cfg.Network.Interface)
	if err := loader.LoadAndAttach(); err != nil {
		log.Fatalf("❌ eBPF 加载失败: %v", err)
	}
	log.Println("✅ eBPF 程序已全量挂载")

	// 5. 策略引擎
	var applier *ebpf.DefenseApplier
	engine := strategy.NewStrategyEngine(func(level strategy.DefenseLevel) error {
		params := strategy.LevelToParams(level)
		config := &ebpf.DefenseConfig{
			Level:          uint32(params.NoiseIntensity),
			JitterMeanUs:   params.JitterMeanUs,
			JitterStddevUs: params.JitterStddevUs,
			PaddingRate:    params.PaddingRate,
			NoiseIntensity: params.NoiseIntensity,
			UpdateInterval: 30 * time.Second,
		}
		if applier != nil {
			return applier.UpdateConfig(config)
		}
		return nil
	})

	defenseConfig := &ebpf.DefenseConfig{
		Level:          uint32(cfg.Defense.Level),
		JitterMeanUs:   50000,
		JitterStddevUs: 15000,
		PaddingRate:    uint32(cfg.Defense.Level),
		NoiseIntensity: uint32(cfg.Defense.Level),
		UpdateInterval: 30 * time.Second,
	}
	applier = ebpf.NewDefenseApplier(loader, defenseConfig)
	applier.Start()
	log.Println("✅ 策略引擎已启动")

	// 6. 威胁编排
	aggregator := threat.NewAggregator(10000)
	aggregator.Start(ctx)

	responder := threat.NewResponder(engine, loader)
	responder.Start(ctx, aggregator.Subscribe())

	blacklist := threat.NewBlacklistManager(loader, 65536)
	blacklist.StartExpiry(ctx)
	log.Println("✅ 威胁编排模块已启动")

	// 7. 威胁监控器 + 事件源注册
	reader, err := ebpf.NewRingBufferReader(loader.GetMap("threat_events"), nil)
	if err != nil {
		log.Fatalf("❌ Ring Buffer 读取器创建失败: %v", err)
	}
	monitor := ebpf.NewThreatMonitor(reader)
	if cfg.Defense.AutoAdjust {
		monitor.SetEngine(engine)
	}
	monitor.RegisterCallback(aggregator.IngestEBPF)
	monitor.Start()
	log.Println("✅ 威胁监控器已启动")

	// 8. G-Switch 管理器
	gswitchMgr := gswitch.NewGSwitchManager(
		loader.GetMap("sni_map"),
		loader.GetMap("domain_ctrl"),
	)
	gswitchMgr.SetJA4Map(loader.GetMap("active_profile_map"))
	gswitchMgr.Start()
	log.Println("✅ G-Switch 域名转生管理器已启动")

	// 9. Cortex 威胁感知中枢（新增）
	cortexAnalyzer := cortex.NewAnalyzer(cortex.DefaultConfig())
	cortexAnalyzer.OnAutoBlock(func(ip string, reason string) {
		if err := blacklist.Add(ip+"/32", time.Now().Add(24*time.Hour), threat.SourceLocal); err != nil {
			log.Printf("⚠️ Cortex 自动封禁失败: %v", err)
		}
	})
	go cortexAnalyzer.Start(ctx)
	log.Println("✅ Cortex 威胁感知中枢已启动")

	// 10. Phantom 影子欺骗管理器（新增）
	phantomMgr := phantom.NewManager()
	phantomMaps := phantom.BuildMapSet(loader)
	if err := phantomMgr.SetMaps(phantomMaps); err != nil {
		log.Printf("⚠️ Phantom 初始化失败（降级运行）: %v", err)
	} else {
		if cfg.Phantom.HoneypotIP != "" {
			phantomMgr.SetHoneypotIP(cfg.Phantom.HoneypotIP)
		}
		phantomMgr.StartEventMonitor()
		log.Println("✅ Phantom 影子欺骗管理器已启动")
	}

	// 11. BurnEngine 实时烧录引擎（新增）
	burnEngine := ebpf.NewBurnEngine(
		loader.GetMap("traffic_stats"),
		loader.GetMap("quota_map"),
		loader.GetMap("whitelist_map"),
	)
	burnEngine.SetOnQuotaExhausted(func(uid string) {
		log.Printf("🚨 [BurnEngine] 用户 %s 配额耗尽，已熔断", uid)
	})
	burnEngine.SetOnQuotaLow(func(uid string, remaining uint64) {
		log.Printf("⚠️ [BurnEngine] 用户 %s 配额不足: %d bytes", uid, remaining)
	})
	burnEngine.Start(ctx)
	log.Println("✅ BurnEngine 实时烧录引擎已启动")

	// 12. TPROXY 透明代理桥接器（新增）
	tproxyAddr := cfg.TPROXY.ListenAddr
	if tproxyAddr == "" {
		tproxyAddr = "0.0.0.0:12345"
	}
	tproxyBridge := tproxy.NewTPROXYBridge(
		tproxyAddr,
		loader.GetSockMap(),
		loader.GetProxyMap(),
		loader.GetConnStateMap(),
	)
	if err := tproxyBridge.Start(); err != nil {
		log.Printf("⚠️ TPROXY 启动失败（降级到用户态转发）: %v", err)
	} else {
		log.Println("✅ TPROXY 透明代理桥接器已启动")
	}

	// 13. gRPC 客户端（非关键，失败降级）
	var grpcClient *api.GRPCClient
	clientTLS, _ := tlsMgr.GetClientTLSConfig()
	grpcClient = api.NewGRPCClient(cfg.MCC.Endpoint, gatewayID, clientTLS)
	go func() {
		if err := grpcClient.Connect(ctx); err != nil {
			log.Printf("⚠️ gRPC 客户端连接失败（降级运行）: %v", err)
		} else {
			grpcClient.StartHeartbeat(ctx, func() *proto.HeartbeatRequest {
				var memStats runtime.MemStats
				runtime.ReadMemStats(&memStats)
				st := proto.GatewayStatus_ONLINE
				if grpcClient.IsDegraded() {
					st = proto.GatewayStatus_DEGRADED
				}
				return &proto.HeartbeatRequest{
					GatewayId:     gatewayID,
					Timestamp:     time.Now().Unix(),
					Status:        st,
					EbpfLoaded:    true,
					ThreatLevel:   int32(responder.GetCurrentLevel()),
					MemoryUsageMb: int32(memStats.Alloc / 1024 / 1024),
				}
			})
			grpcClient.StartTrafficReport(ctx, func() *proto.TrafficRequest {
				return &proto.TrafficRequest{
					GatewayId:     gatewayID,
					Timestamp:     time.Now().Unix(),
					PeriodSeconds: 60,
				}
			})
		}
	}()

	// 设置 gRPC 通知回调
	responder.SetGRPCNotify(func(level threat.ThreatLevel) {
		if grpcClient != nil && grpcClient.IsConnected() {
			grpcClient.ReportThreat([]*proto.ThreatEvent{{
				Timestamp: time.Now().Unix(),
				Severity:  int32(level) * 2,
				SourceIp:  "0.0.0.0",
			}})
		}
	})

	// 14. gRPC 服务端（非关键，失败降级）
	var grpcServer *api.GRPCServer
	serverTLS, _ := tlsMgr.GetServerTLSConfig()
	handler := api.NewCommandHandler(loader, blacklist, gswitchMgr)
	grpcServer = api.NewGRPCServer(50847, serverTLS, handler)
	if err := grpcServer.Start(); err != nil {
		log.Printf("⚠️ gRPC 服务端启动失败（降级运行）: %v", err)
	} else {
		log.Println("✅ gRPC 服务端已启动")
	}

	// 15. 增强健康检查
	startTime := time.Now()
	go startEnhancedHealthServer(*healthPort, startTime, loader, grpcClient, grpcServer, responder, blacklist)
	log.Println("✅ 健康检查端点已启动")

	// 16. 心跳超时看门狗
	watchdogTimeout := 300 * time.Second
	watchdog := security.NewWatchdog(watchdogTimeout, ramShield, ebpf.NewEmergencyManager(loader))
	watchdog.Start()

	if grpcClient != nil {
		grpcClient.SetHeartbeatCallback(func() {
			watchdog.Feed()
		})
	}

	log.Println("🟢 Mirage-Gateway 启动完成")

	// 17. 构建优雅关闭管理器
	emergencyMgr := ebpf.NewEmergencyManager(loader)
	shutdownTimeout := cfg.Security.GracefulShutdown.Timeout
	if shutdownTimeout <= 0 {
		shutdownTimeout = 30 * time.Second
	}
	graceful := security.NewGracefulShutdown(ramShield, emergencyMgr, shutdownTimeout)

	// 注册模块（按启动顺序，关闭时逆序）
	graceful.RegisterModule(&shutdownAdapter{"mTLS", func(ctx context.Context) error { return tlsMgr.Close() }})
	graceful.RegisterModule(&shutdownAdapter{"eBPF", func(ctx context.Context) error { loader.Close(); return nil }})
	graceful.RegisterModule(&shutdownAdapter{"DefenseApplier", func(ctx context.Context) error { applier.Stop(); return nil }})
	graceful.RegisterModule(&shutdownAdapter{"G-Switch", func(ctx context.Context) error { gswitchMgr.Stop(); return nil }})
	graceful.RegisterModule(&shutdownAdapter{"ThreatMonitor", func(ctx context.Context) error { monitor.Stop(); return nil }})
	graceful.RegisterModule(&shutdownAdapter{"Cortex", func(ctx context.Context) error { cortexAnalyzer.Stop(); return nil }})
	graceful.RegisterModule(&shutdownAdapter{"Phantom", func(ctx context.Context) error { phantomMgr.Stop(); return nil }})
	graceful.RegisterModule(&shutdownAdapter{"BurnEngine", func(ctx context.Context) error { burnEngine.Stop(); return nil }})
	graceful.RegisterModule(&shutdownAdapter{"TPROXY", func(ctx context.Context) error { tproxyBridge.Stop(); return nil }})
	if grpcClient != nil {
		graceful.RegisterModule(&shutdownAdapter{"gRPC-Client", func(ctx context.Context) error { grpcClient.Close(); return nil }})
	}
	if grpcServer != nil {
		graceful.RegisterModule(&shutdownAdapter{"gRPC-Server", func(ctx context.Context) error { grpcServer.Stop(); return nil }})
	}
	graceful.RegisterModule(&shutdownAdapter{"Watchdog", func(ctx context.Context) error { watchdog.Stop(); return nil }})
	if antiDebug != nil {
		graceful.RegisterModule(&shutdownAdapter{"AntiDebug", func(ctx context.Context) error { antiDebug.Stop(); return nil }})
	}

	// 忽略未使用变量
	_ = certPin

	// 18. 等待退出信号
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Println("🛑 收到退出信号，开始优雅关闭...")
	cancel()

	if err := graceful.Shutdown(); err != nil {
		log.Printf("⚠️ 优雅关闭出现错误: %v", err)
	}
	log.Println("✅ Mirage-Gateway 已安全退出")
}

// shutdownAdapter 将函数适配为 ShutdownModule 接口
type shutdownAdapter struct {
	name string
	fn   func(ctx context.Context) error
}

func (s *shutdownAdapter) Name() string                       { return s.name }
func (s *shutdownAdapter) Shutdown(ctx context.Context) error { return s.fn(ctx) }

// resolveGatewayID 解析或生成 Gateway ID
// 优先级：配置文件 > 持久化文件 > 自动生成（MAC + hostname SHA-256 前 12 位）
func resolveGatewayID(cfg *GatewayConfig) string {
	// 1. 配置文件指定
	if cfg.GatewayID != "" {
		return cfg.GatewayID
	}

	// 2. 持久化文件
	idFile := "/var/lib/mirage/gateway_id"
	if data, err := os.ReadFile(idFile); err == nil && len(data) > 0 {
		return string(data)
	}

	// 3. 自动生成：MAC + hostname 的 SHA-256 前 12 位
	seed := ""
	if ifaces, err := net.Interfaces(); err == nil {
		for _, iface := range ifaces {
			if iface.HardwareAddr != nil && len(iface.HardwareAddr) > 0 {
				seed += iface.HardwareAddr.String()
				break
			}
		}
	}
	if hostname, err := os.Hostname(); err == nil {
		seed += hostname
	}
	if seed == "" {
		seed = fmt.Sprintf("mirage-%d", time.Now().UnixNano())
	}

	hash := sha256.Sum256([]byte(seed))
	id := "gw-" + hex.EncodeToString(hash[:])[:12]

	// 持久化
	os.MkdirAll("/var/lib/mirage", 0700)
	os.WriteFile(idFile, []byte(id), 0600)

	return id
}

// loadConfig 加载配置文件
func loadConfig(path string) *GatewayConfig {
	data, err := os.ReadFile(path)
	if err != nil {
		log.Printf("⚠️ 读取配置文件失败: %v，使用默认配置", err)
		return &GatewayConfig{}
	}

	expanded := os.ExpandEnv(string(data))

	cfg := &GatewayConfig{}
	if err := yaml.Unmarshal([]byte(expanded), cfg); err != nil {
		log.Printf("⚠️ 解析配置文件失败: %v，使用默认配置", err)
		return &GatewayConfig{}
	}

	if cfg.Network.Interface == "" {
		cfg.Network.Interface = "eth0"
	}
	if cfg.MCC.Endpoint == "" {
		cfg.MCC.Endpoint = "https://mirage-os:50847"
	}

	return cfg
}

// startEnhancedHealthServer 启动增强健康检查
func startEnhancedHealthServer(
	port int,
	startTime time.Time,
	loader *ebpf.Loader,
	grpcClient *api.GRPCClient,
	grpcServer *api.GRPCServer,
	responder *threat.Responder,
	blacklist *threat.BlacklistManager,
) {
	mux := http.NewServeMux()

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if grpcClient != nil && grpcClient.IsDegraded() {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("DEGRADED"))
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		status := EnhancedHealthStatus{
			Status:              "running",
			EBPFLoaded:          true,
			GRPCClientConnected: grpcClient != nil && grpcClient.IsConnected(),
			GRPCServerRunning:   grpcServer != nil && grpcServer.IsRunning(),
			ThreatLevel:         int(responder.GetCurrentLevel()),
			BlacklistCount:      blacklist.Count(),
			Uptime:              time.Since(startTime).String(),
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(status)
	})

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	log.Printf("✅ 健康检查端点: http://%s/healthz", addr)
	http.ListenAndServe(addr, mux)
}
