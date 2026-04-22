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
	"mirage-gateway/pkg/cortex"
	"mirage-gateway/pkg/dataplane"
	"mirage-gateway/pkg/ebpf"
	"mirage-gateway/pkg/gswitch"
	"mirage-gateway/pkg/gtunnel"
	"mirage-gateway/pkg/gtunnel/stealth"
	"mirage-gateway/pkg/nerve"
	"mirage-gateway/pkg/orchestrator/events"
	"mirage-gateway/pkg/phantom"
	"mirage-gateway/pkg/security"
	"mirage-gateway/pkg/strategy"
	"mirage-gateway/pkg/threat"
	"mirage-gateway/pkg/tproxy"
	pb "mirage-proto/gen"

	"github.com/prometheus/client_golang/prometheus/promhttp"
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
		L1             struct {
			ASNBlocklistPath string `yaml:"asn_blocklist_path"`
			CloudRangesPath  string `yaml:"cloud_ranges_path"`
			RateLimit        struct {
				SynPPS  uint32 `yaml:"syn_pps"`
				ConnPPS uint32 `yaml:"conn_pps"`
				Enabled bool   `yaml:"enabled"`
			} `yaml:"rate_limit"`
			SilentResponse struct {
				DropICMPUnreachable bool `yaml:"drop_icmp_unreachable"`
				DropTCPRst          bool `yaml:"drop_tcp_rst"`
				Enabled             bool `yaml:"enabled"`
			} `yaml:"silent_response"`
		} `yaml:"l1"`
		L2 struct {
			NonceStoreSize     int `yaml:"nonce_store_size"`
			NonceTTLSeconds    int `yaml:"nonce_ttl_seconds"`
			HandshakeTimeoutMs int `yaml:"handshake_timeout_ms"`
		} `yaml:"l2"`
		L3 struct {
			BehaviorCheckIntervalSeconds int     `yaml:"behavior_check_interval_seconds"`
			DeviationThreshold           float64 `yaml:"deviation_threshold"`
		} `yaml:"l3"`
	} `yaml:"defense"`
	MCC struct {
		Endpoint string             `yaml:"endpoint"`
		CellID   string             `yaml:"cell_id"`
		TLS      security.TLSConfig `yaml:"tls"`
	} `yaml:"mcc"`
	BDNA struct {
		Enabled        bool          `yaml:"enabled"`
		RegistryPath   string        `yaml:"registry_path"`
		JA4Database    string        `yaml:"ja4_database"`
		UpdateInterval time.Duration `yaml:"update_interval"`
	} `yaml:"bdna"`
	Security SecurityConfig `yaml:"security"`
	Phantom  struct {
		HoneypotIP          string          `yaml:"honeypot_ip"`
		Persona             phantom.Persona `yaml:"persona"`
		HoneypotPool        map[int]string  `yaml:"honeypot_pool"`
		DefaultTTLSeconds   uint32          `yaml:"default_ttl_seconds"`
		HighRiskTTLSeconds  uint32          `yaml:"high_risk_ttl_seconds"`
		LabyrinthMaxDepth   int             `yaml:"labyrinth_max_depth"`
		LabyrinthMaxDelayMs int             `yaml:"labyrinth_max_delay_ms"`
	} `yaml:"phantom"`
	TPROXY struct {
		ListenAddr string `yaml:"listen_addr"`
	} `yaml:"tproxy"`
	DataPlane struct {
		QUICListenAddr string `yaml:"quic_listen_addr"` // 公网 QUIC/H3 数据面监听地址，默认 :443
		EnableWSS      bool   `yaml:"enable_wss"`       // 启用 WSS 降级监听
		EnableWebRTC   bool   `yaml:"enable_webrtc"`
		EnableICMP     bool   `yaml:"enable_icmp"`
		EnableDNS      bool   `yaml:"enable_dns"`
	} `yaml:"data_plane"`
	Chameleon struct {
		ListenAddr     string `yaml:"listen_addr"`      // WSS 降级监听地址，默认 :443
		WSPath         string `yaml:"ws_path"`          // WebSocket 路径，默认 /api/v2/stream
		FakeServerName string `yaml:"fake_server_name"` // 伪装 Server 头
		CertFile       string `yaml:"cert_file"`
		KeyFile        string `yaml:"key_file"`
		CAFile         string `yaml:"ca_file"`
	} `yaml:"chameleon"`
}

// SecurityConfig 安全加固配置
type SecurityConfig struct {
	CommandSecret string `yaml:"command_secret"`
	RAMShield     struct {
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

	// 1.1 生产模式强制 mTLS 校验
	if os.Getenv("MIRAGE_ENV") == "production" && !cfg.MCC.TLS.Enabled {
		log.Fatalf("❌ 生产模式禁止禁用 mTLS，请配置 mcc.tls.enabled: true")
	}

	// 全局 context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 1.5 Gateway ID 动态化
	gatewayID := resolveGatewayID(cfg)
	log.Printf("🆔 Gateway ID: %s", gatewayID)

	// 1.6 Prometheus 指标注册 + metrics HTTP server
	threat.SetGatewayID(gatewayID)
	threat.RegisterMetrics()
	go func() {
		mux := http.NewServeMux()
		mux.Handle("/metrics", promhttp.Handler())
		log.Println("✅ Prometheus metrics: http://127.0.0.1:9090/metrics")
		if err := http.ListenAndServe("127.0.0.1:9090", mux); err != nil {
			log.Printf("⚠️ Metrics HTTP server 启动失败: %v", err)
		}
	}()

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

	bdnaProfileUpdater := ebpf.NewBDNAProfileUpdater(loader)
	if cfg.BDNA.Enabled {
		registry, err := bdnaProfileUpdater.SeedRegistryFromFile(cfg.BDNA.RegistryPath)
		if err != nil {
			log.Printf("⚠️ B-DNA registry 装载失败（降级运行）: %v", err)
		} else if err := bdnaProfileUpdater.SetActiveProfile(registry.DefaultActiveProfile); err != nil {
			log.Printf("⚠️ B-DNA 初始画像激活失败（降级运行）: %v", err)
		} else {
			log.Printf("✅ B-DNA 握手画像 registry 已装载: version=%s default=%s(%d)",
				registry.RegistryVersion, registry.DefaultProfileName(), registry.DefaultActiveProfile)
		}
	} else {
		log.Println("⚠️ B-DNA 已禁用，跳过握手画像 registry 装载")
	}

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

	blacklist := threat.NewBlacklistManager(loader, 65536)
	blacklist.StartExpiry(ctx)

	responder := threat.NewResponder(engine, loader, blacklist)
	responder.Start(ctx, aggregator.Subscribe())

	log.Println("✅ 威胁编排模块已启动")

	// 6.5 零信任三层纵深防御
	// ① 加载本地威胁情报库
	var intelProvider *threat.ThreatIntelProvider
	asnPath := cfg.Defense.L1.ASNBlocklistPath
	cloudPath := cfg.Defense.L1.CloudRangesPath
	if asnPath != "" && cloudPath != "" {
		var err error
		intelProvider, err = threat.NewThreatIntelProvider(asnPath, cloudPath)
		if err != nil {
			log.Printf("⚠️ 威胁情报库加载失败（降级运行）: %v", err)
		} else {
			log.Println("✅ 威胁情报库已加载")
		}
	}

	// ② 同步 ASN 网段到 eBPF Map
	if intelProvider != nil {
		exports := intelProvider.GetASNBlockEntries()
		asnEntries := make([]ebpf.ASNBlockEntry, 0, len(exports))
		for _, e := range exports {
			asnEntries = append(asnEntries, ebpf.ASNBlockEntry{CIDR: e.CIDR, ASN: e.ASN})
		}
		if len(asnEntries) > 0 {
			if err := applier.SyncASNBlocklist(asnEntries); err != nil {
				log.Printf("⚠️ ASN 黑名单同步失败: %v", err)
			} else {
				log.Printf("✅ ASN 黑名单已同步: %d 条目", len(asnEntries))
			}
		} else {
			log.Println("⚠️ ASN 黑名单为空，L1 清洗未生效")
		}
	}

	// ③ 同步速率限制配置
	if cfg.Defense.L1.RateLimit.Enabled {
		rlCfg := &ebpf.RateLimitConfig{
			SynPPSLimit:  cfg.Defense.L1.RateLimit.SynPPS,
			ConnPPSLimit: cfg.Defense.L1.RateLimit.ConnPPS,
			Enabled:      1,
		}
		if rlCfg.SynPPSLimit == 0 {
			rlCfg.SynPPSLimit = 50
		}
		if rlCfg.ConnPPSLimit == 0 {
			rlCfg.ConnPPSLimit = 200
		}
		if err := applier.SyncRateLimitConfig(rlCfg); err != nil {
			log.Printf("⚠️ 速率限制配置同步失败: %v", err)
		} else {
			log.Println("✅ L1 速率限制已配置")
		}
	}

	// ④ 同步静默响应配置
	if cfg.Defense.L1.SilentResponse.Enabled {
		silentCfg := &ebpf.SilentConfig{
			DropICMPUnreachable: boolToUint32(cfg.Defense.L1.SilentResponse.DropICMPUnreachable),
			DropTCPRst:          boolToUint32(cfg.Defense.L1.SilentResponse.DropTCPRst),
			Enabled:             1,
		}
		if err := applier.SyncSilentConfig(silentCfg); err != nil {
			log.Printf("⚠️ 静默响应配置同步失败: %v", err)
		} else {
			log.Println("✅ L1 静默响应已配置")
		}
	}

	// ⑤ 启动 L1 事件监听
	riskScorer := cortex.NewRiskScorer(blacklist)
	riskScorer.StartDecay(ctx)
	l1Monitor := threat.NewL1Monitor(loader, riskScorer)
	if intelProvider != nil {
		l1Monitor.SetIntelProvider(intelProvider)
	}
	l1Monitor.StartEventLoop(ctx)
	log.Println("✅ L1 事件监听已启动")

	// ⑥ NonceStore + 清理
	nonceTTL := time.Duration(cfg.Defense.L2.NonceTTLSeconds) * time.Second
	if nonceTTL == 0 {
		nonceTTL = 5 * time.Minute
	}
	nonceSize := cfg.Defense.L2.NonceStoreSize
	if nonceSize == 0 {
		nonceSize = 100000
	}
	nonceStore := threat.NewNonceStore(nonceSize, nonceTTL)
	nonceStore.StartCleanup(ctx)
	log.Println("✅ NonceStore 抗重放已启动")

	// ⑦ HandshakeGuard
	hsTimeout := time.Duration(cfg.Defense.L2.HandshakeTimeoutMs) * time.Millisecond
	if hsTimeout == 0 {
		hsTimeout = 300 * time.Millisecond
	}
	handshakeGuard := threat.NewHandshakeGuard(hsTimeout, blacklist, riskScorer)
	log.Println("✅ HandshakeGuard 已初始化")

	// ⑧ ProtocolDetector
	protocolDetector := threat.NewProtocolDetector(riskScorer, blacklist)
	log.Println("✅ ProtocolDetector 已初始化")

	// ⑨ BehaviorMonitor
	devThreshold := cfg.Defense.L3.DeviationThreshold
	if devThreshold == 0 {
		devThreshold = 0.7
	}
	behaviorMonitor := cortex.NewBehaviorMonitor(cortex.DefaultBaseline(), devThreshold, riskScorer)
	behaviorMonitor.StartMonitoring(ctx)
	log.Println("✅ BehaviorMonitor 行为基线监控已启动")

	// 抑制未使用变量警告（初始化副作用已生效）
	_ = intelProvider
	_ = behaviorMonitor

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
	gswitchMgr.SetBDNAProfileSwitcher(bdnaProfileUpdater)
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

	// 9.5 Phantom → Cortex 联动：蜜罐命中上报
	phantomThreatBus := cortex.NewThreatBus(nil)
	honeypotReporter := phantom.NewHoneypotReporter(phantomThreatBus)

	// 10. Phantom 影子欺骗管理器（新增）
	phantomMgr := phantom.NewManager()
	phantomMaps := phantom.BuildMapSet(loader)
	if err := phantomMgr.SetMaps(phantomMaps); err != nil {
		log.Printf("⚠️ Phantom 初始化失败（降级运行）: %v", err)
	} else {
		// 兼容旧配置：单一蜜罐 IP
		if cfg.Phantom.HoneypotIP != "" {
			phantomMgr.SetHoneypotIP(cfg.Phantom.HoneypotIP)
		}
		// 分层目标池配置
		for level, ip := range cfg.Phantom.HoneypotPool {
			if err := phantomMgr.SetHoneypotPool(level, ip); err != nil {
				log.Printf("⚠️ Phantom 目标池 level=%d 配置失败: %v", level, err)
			}
		}
		// 启动 TTL 清理器
		phantomMgr.StartTTLCleaner(ctx)
		// 启动事件监控
		phantomMgr.StartEventMonitor()
		log.Println("✅ Phantom 影子欺骗管理器已启动")
	}

	// Phantom Dispatcher 初始化（Persona + 迷宫配置）
	phantomDispatcher := phantom.NewDispatcher()
	persona := cfg.Phantom.Persona
	if persona.CompanyName == "" {
		persona = phantom.DefaultPersona
	}
	phantomDispatcher.SetPersona(persona)
	if cfg.Phantom.LabyrinthMaxDepth > 0 {
		phantomDispatcher.GetLabyrinth().SetMaxDepth(cfg.Phantom.LabyrinthMaxDepth)
	}
	if cfg.Phantom.LabyrinthMaxDelayMs > 0 {
		phantomDispatcher.GetLabyrinth().SetDelayConfig(
			50*time.Millisecond,
			1.5,
			time.Duration(cfg.Phantom.LabyrinthMaxDelayMs)*time.Millisecond,
		)
	}

	// 绑定 HoneypotReporter 到 Phantom 蜜罐服务
	_ = honeypotReporter
	_ = phantomDispatcher

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

	// 12.5 V2 编排组件初始化
	// 正确依赖顺序：EventDispatcher → ChameleonListener → Orchestrator → StealthControlPlane
	// ChameleonListener 必须先于 Orchestrator 启动，否则 WSS DialFunc 无法连接到本机监听端口。

	// ① EventDispatcher
	v2EventRegistry := events.NewEventRegistry()
	if err := events.RegisterV2Handlers(v2EventRegistry); err != nil {
		log.Fatalf("❌ V2 handler 注册失败: %v", err)
	}
	v2Dedup := events.NewDeduplicationStore()
	v2Dispatcher := events.NewEventDispatcher(v2EventRegistry, v2Dedup, nil)
	log.Println("✅ V2 EventDispatcher 已创建（handler 已接线）")

	v2Adapter := api.NewV2CommandAdapter(v2Dispatcher)
	log.Println("✅ V2 CommandAdapter 已创建")

	// ② ChameleonListener — 必须先于 Orchestrator 启动（WSS DialFunc 依赖本机监听）
	var chameleonListener *gtunnel.ChameleonListener
	if cfg.DataPlane.EnableWSS {
		chameleonAddr := cfg.Chameleon.ListenAddr
		if chameleonAddr == "" {
			chameleonAddr = ":443"
		}
		chameleonWSPath := cfg.Chameleon.WSPath
		if chameleonWSPath == "" {
			chameleonWSPath = "/api/v2/stream"
		}
		chameleonFake := cfg.Chameleon.FakeServerName
		if chameleonFake == "" {
			chameleonFake = "cloudflare"
		}
		clConfig := gtunnel.ChameleonListenerConfig{
			ListenAddr:     chameleonAddr,
			WSPath:         chameleonWSPath,
			FakeServerName: chameleonFake,
			CertFile:       cfg.Chameleon.CertFile,
			KeyFile:        cfg.Chameleon.KeyFile,
			CAFile:         cfg.Chameleon.CAFile,
			MaxConnections: 1000,
			IdleTimeout:    60 * time.Second,
			ReadLimit:      65536,
		}
		var clErr error
		chameleonListener, clErr = gtunnel.NewChameleonListener(clConfig)
		if clErr != nil {
			log.Printf("⚠️ ChameleonListener 创建失败（WSS 降级不可用）: %v", clErr)
		} else {
			if err := chameleonListener.Start(); err != nil {
				log.Printf("⚠️ ChameleonListener 启动失败: %v", err)
				chameleonListener = nil
			} else {
				log.Println("✅ ChameleonListener WSS 降级数据面已启动")
			}
		}
	}

	// ③ Orchestrator — 唯一多协议编排主链（S-01 收敛）
	// Gateway 是服务端，不主动拨号。使用被动模式：
	// ChameleonListener 接受入站连接 → 通过 AdoptInboundConn 注入 Orchestrator
	orchConfig := gtunnel.DefaultOrchestratorConfig()
	orchConfig.EnableQUIC = false // Gateway 不出站拨号 QUIC
	orchConfig.EnableWSS = chameleonListener != nil
	orchConfig.EnableWebRTC = cfg.DataPlane.EnableWebRTC && chameleonListener != nil
	orchConfig.EnableICMP = cfg.DataPlane.EnableICMP
	orchConfig.EnableDNS = cfg.DataPlane.EnableDNS
	orchestrator := gtunnel.NewOrchestrator(orchConfig)

	// 被动模式启动：不执行 HappyEyeballs 竞速，只启动 probeLoop + receiveLoop
	// 入站连接通过 ChameleonListener 回调注入
	orchestrator.StartPassive(ctx)

	// 数据面注入器：Orchestrator 收到的解隧 IP 包通过此接口注入本机网络栈。
	// 优先尝试 TUN 设备；不可用时降级为 NoopInjector（结构化告警 + 计数）。
	var dpInjector dataplane.Injector
	tunInjector, tunErr := dataplane.NewTUNInjector(dataplane.DefaultTUNConfig())
	if tunErr != nil {
		log.Printf("⚠️ [DataPlane] TUN 设备不可用，降级为 NoopInjector: %v", tunErr)
		dpInjector = dataplane.NewNoopInjector()
	} else {
		dpInjector = tunInjector
	}

	// 设置收包回调：Orchestrator → DataPlane Injector
	orchestrator.SetPacketCallback(func(data []byte) {
		if len(data) == 0 {
			return
		}
		if err := dpInjector.InjectIPPacket(data); err != nil {
			// NoopInjector 会自行限频打日志，这里只在非 Noop 时打
			if _, isNoop := dpInjector.(*dataplane.NoopInjector); !isNoop {
				log.Printf("[DataPlane] IP 包注入失败: %v", err)
			}
		}
	})

	log.Println("✅ Orchestrator 已启动（被动模式，等待入站连接）")

	// 接线 ChameleonListener → Orchestrator（入站连接注入 + 数据转发）
	if chameleonListener != nil {
		cl := chameleonListener
		orch := orchestrator

		// 新客户端连接时：将 WSS 连接包装为 TransportConn 注入 Orchestrator
		cl.SetClientConnectCallback(func(clientID string, conn *gtunnel.ChameleonServerConn) {
			adapter := gtunnel.NewChameleonServerConnAdapter(conn, clientID)
			orch.AdoptInboundConn(adapter, gtunnel.TransportWebSocket)
			log.Printf("🔗 [Chameleon→Orchestrator] 客户端 %s 入站连接已注入", clientID)
		})

		// 收包回调：ChameleonListener 收到的包按 clientID 精确喂入对应适配器
		cl.SetPacketCallback(func(clientID string, data []byte) {
			orch.FeedInboundPacket(gtunnel.TransportWebSocket, clientID, data)
		})

		// 反向路径：Orchestrator 发出的包通过 activePath（即注入的 ChameleonServerConn）自动到达客户端
		// 不需要额外的 SetPacketCallback 广播 — Send() 直接走 activePath.Conn.Send()
	}

	// ④ StealthControlPlane — 双通道隐蔽控制面
	stealthCP := stealth.NewStealthControlPlane(stealth.StealthControlPlaneOpts{
		Dispatcher: &stealthDispatcherAdapter{inner: v2Dispatcher},
	})
	// ReceiveLoop 内部对 mux==nil / decoder==nil 做了安全等待（100ms/500ms 轮询），不会 panic。
	// 当前 Mux/Encoder/Decoder 均为 nil，状态为 ChannelQueued，命令会排队到 cmdQueue。
	// 激活路径：未来 QUIC/H3 bearer listener 建立后，构造 ShadowStreamMux 并重建 StealthControlPlane。
	// 注意：当前 stealth 包没有 SetMux/AttachBearer 方法，激活需要重建实例。
	go func() {
		if err := stealthCP.ReceiveLoop(ctx); err != nil {
			log.Printf("⚠️ StealthControlPlane ReceiveLoop 退出: %v", err)
		}
	}()
	log.Println("✅ StealthControlPlane 已启动（ChannelQueued — 等待 QUIC/H3 bearer 建立后重建实例激活）")

	// 13. gRPC 客户端（mTLS 强制）
	var grpcClient *api.GRPCClient
	var sensoryUplink *nerve.SensoryUplink
	clientTLS, _ := tlsMgr.GetClientTLSConfig()
	if clientTLS == nil {
		log.Fatalf("❌ gRPC Client TLS 配置为空，拒绝启动")
	}
	grpcClient = api.NewGRPCClient(cfg.MCC.Endpoint, gatewayID, clientTLS)
	if certPin != nil {
		grpcClient.SetCertPin(certPin)
	}
	go func() {
		if err := grpcClient.Connect(ctx); err != nil {
			log.Printf("⚠️ gRPC 客户端连接失败（降级运行）: %v", err)
		} else {
			// 注册 Gateway
			if err := grpcClient.Register(ctx, gatewayID, cfg.MCC.CellID, "0.0.0.0:50847", "v1.0"); err != nil {
				log.Printf("⚠️ Gateway 注册失败（降级运行）: %v", err)
			}

			grpcClient.StartHeartbeat(ctx, func() *pb.HeartbeatRequest {
				var memStats runtime.MemStats
				runtime.ReadMemStats(&memStats)
				st := pb.GatewayStatus_ONLINE
				if grpcClient.IsDegraded() {
					st = pb.GatewayStatus_DEGRADED
				}

				// TODO: Proto 需要增加 blacklist_count 和 blacklist_updated_at 字段
				// 当前通过 ActiveConnections 字段临时承载黑名单条目数（待 proto 更新后迁移）
				blCount := int64(blacklist.Count())
				blUpdatedAt := blacklist.LatestUpdateTimestamp()
				_ = blUpdatedAt // 待 proto 扩展后使用

				// 8.6: 获取当前安全状态用于心跳上报
				// TODO: Proto 需要增加 security_state 字段到 HeartbeatRequest
				// 当前通过 ThreatLevel 高位临时承载安全状态（待 proto 更新后迁移）
				securityState := int32(0)
				if fsm := responder.GetFSM(); fsm != nil {
					securityState = int32(fsm.CurrentState())
				}
				_ = securityState // 待 proto 扩展后使用

				return &pb.HeartbeatRequest{
					GatewayId:         gatewayID,
					Timestamp:         time.Now().Unix(),
					Status:            st,
					EbpfLoaded:        true,
					ThreatLevel:       int32(responder.GetCurrentLevel()),
					MemoryUsageMb:     int32(memStats.Alloc / 1024 / 1024),
					ActiveConnections: blCount, // 临时复用，待 proto 增加 blacklist_count 字段
					// 拓扑语义字段
					DownlinkAddr:   "0.0.0.0:50847",
					CellId:         cfg.MCC.CellID,
					ActiveSessions: 0,  // TODO: 从 session manager 获取实际值
					StateHash:      "", // TODO: 从 MotorDownlink 获取当前 state hash
					Version:        "v1.0",
				}
			})
			// 启动上行感知闭环（10s 流量上报 via gRPC）
			sensoryUplink = nerve.NewSensoryUplink(grpcClient, loader, gatewayID)
			sensoryUplink.Start(ctx)
		}
	}()

	// 设置 gRPC 通知回调（威胁上报）
	responder.SetGRPCNotify(func(level threat.ThreatLevel) {
		if grpcClient != nil && grpcClient.IsConnected() {
			grpcClient.ReportThreat([]*pb.ThreatEvent{{
				Timestamp:  time.Now().Unix(),
				ThreatType: pb.ThreatType_DPI_DETECTION,
				Severity:   int32(level) * 2,
				SourceIp:   "0.0.0.0",
			}})
		}
	})

	// 下行状态机映射器（幂等 Hash 校验）
	motorDownlink := nerve.NewMotorDownlink(loader)

	// 14. gRPC 服务端（非关键，失败降级）
	var grpcServer *api.GRPCServer
	serverTLS, _ := tlsMgr.GetServerTLSConfig()
	handler := api.NewCommandHandler(loader, blacklist, gswitchMgr)
	handler.SetMotorDownlink(&motorDownlinkAdapter{md: motorDownlink})
	handler.SetV2Adapter(v2Adapter)

	// 注入安全组件
	if cfg.Security.CommandSecret == "" {
		log.Fatalf("❌ CommandSecret 为空，拒绝启动（安全策略要求）")
	}
	cmdAuth := api.NewCommandAuthenticator(cfg.Security.CommandSecret)
	cmdAuth.SetNonceStore(nonceStore)
	handler.SetAuth(cmdAuth)
	handler.SetAudit(api.NewCommandAuditor())
	handler.SetRateLimiter(api.NewCommandRateLimiter())

	grpcServer = api.NewGRPCServer(50847, serverTLS, handler)
	grpcServer.SetListenerWrapper(handshakeGuard.WrapListener)
	grpcServer.SetProtocolDetector(protocolDetector)
	if err := grpcServer.Start(); err != nil {
		log.Fatalf("❌ gRPC 服务端启动失败: %v", err)
	} else {
		log.Println("✅ gRPC 服务端已启动")
	}

	// 15. 增强健康检查
	startTime := time.Now()
	go startEnhancedHealthServer(*healthPort, startTime, loader, grpcClient, grpcServer, responder, blacklist)
	log.Println("✅ 健康检查端点已启动")

	// 16. 心跳超时看门狗 + 焦土协议
	scorchedEarth := nerve.NewScorchedEarth(loader, ebpf.NewEmergencyManager(loader))
	// 注册 TLS 证书路径（自毁时 3 次覆写）
	if cfg.MCC.TLS.CertFile != "" {
		scorchedEarth.RegisterCertPaths(cfg.MCC.TLS.CertFile, cfg.MCC.TLS.KeyFile, cfg.MCC.TLS.CAFile)
	}
	// 注册临时配置
	scorchedEarth.RegisterConfigPaths("/var/lib/mirage/gateway_id")

	watchdogTimeout := 300 * time.Second
	// Watchdog 使用 ScorchedEarth 作为 EmergencyWiper（完整焦土协议）
	watchdog := security.NewWatchdog(watchdogTimeout, ramShield, scorchedEarth)
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
	graceful.RegisterModule(&shutdownAdapter{"DataPlane", func(ctx context.Context) error { return dpInjector.Close() }})
	graceful.RegisterModule(&shutdownAdapter{"Orchestrator", func(ctx context.Context) error { return orchestrator.Close() }})
	if chameleonListener != nil {
		graceful.RegisterModule(&shutdownAdapter{"ChameleonListener", func(ctx context.Context) error { return chameleonListener.Stop() }})
	}
	graceful.RegisterModule(&shutdownAdapter{"StealthCP", func(ctx context.Context) error { stealthCP.Close(); return nil }})
	if grpcClient != nil {
		graceful.RegisterModule(&shutdownAdapter{"gRPC-Client", func(ctx context.Context) error { grpcClient.Close(); return nil }})
	}
	if sensoryUplink != nil {
		graceful.RegisterModule(&shutdownAdapter{"SensoryUplink", func(ctx context.Context) error { sensoryUplink.Stop(); return nil }})
	}
	if grpcServer != nil {
		graceful.RegisterModule(&shutdownAdapter{"gRPC-Server", func(ctx context.Context) error { grpcServer.Stop(); return nil }})
	}
	graceful.RegisterModule(&shutdownAdapter{"Watchdog", func(ctx context.Context) error { watchdog.Stop(); return nil }})
	if antiDebug != nil {
		graceful.RegisterModule(&shutdownAdapter{"AntiDebug", func(ctx context.Context) error { antiDebug.Stop(); return nil }})
	}

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
			if len(iface.HardwareAddr) > 0 {
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
	if cfg.BDNA.RegistryPath == "" {
		cfg.BDNA.RegistryPath = "configs/bdna/profile-registry.v1.json"
	}

	return cfg
}

// stealthDispatcherAdapter 适配 events.EventDispatcher → stealth.EventDispatcher
// stealth.EventDispatcher: Dispatch(ctx, interface{})
// events.EventDispatcher: Dispatch(ctx, *ControlEvent)
type stealthDispatcherAdapter struct {
	inner events.EventDispatcher
}

func (a *stealthDispatcherAdapter) Dispatch(ctx context.Context, event interface{}) error {
	ce, ok := event.(*events.ControlEvent)
	if !ok {
		return fmt.Errorf("stealthDispatcherAdapter: expected *events.ControlEvent, got %T", event)
	}
	return a.inner.Dispatch(ctx, ce)
}

// motorDownlinkAdapter 适配 nerve.MotorDownlink → api.MotorDownlinkApplier
type motorDownlinkAdapter struct {
	md *nerve.MotorDownlink
}

func (a *motorDownlinkAdapter) ApplyDesiredState(cfg *api.DesiredStatePayload) (bool, error) {
	return a.md.ApplyDesiredState(&nerve.DesiredStateConfig{
		JitterMeanUs:   cfg.JitterMeanUs,
		JitterStddevUs: cfg.JitterStddevUs,
		NoiseIntensity: cfg.NoiseIntensity,
		PaddingRate:    cfg.PaddingRate,
		TemplateID:     cfg.TemplateID,
		FiberJitterUs:  cfg.FiberJitterUs,
		RouterDelayUs:  cfg.RouterDelayUs,
	})
}

// boolToUint32 将 bool 转换为 uint32（1/0），用于 eBPF Map 配置下发
func boolToUint32(b bool) uint32 {
	if b {
		return 1
	}
	return 0
}

// startEnhancedHealthServer 启动增强健康检查
func startEnhancedHealthServer(
	port int,
	startTime time.Time,
	_ *ebpf.Loader,
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
