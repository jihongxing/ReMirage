package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/tls"
	_ "embed"
	encoding_base64 "encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"phantom-client/pkg/daemon"
	"phantom-client/pkg/entitlement"
	"phantom-client/pkg/gtclient"
	"phantom-client/pkg/killswitch"
	"phantom-client/pkg/memsafe"
	"phantom-client/pkg/nicdetect"
	"phantom-client/pkg/persist"
	"phantom-client/pkg/resonance"
	"phantom-client/pkg/token"
	"phantom-client/pkg/tun"
)

//go:embed wintun.dll
var embeddedWintunDLL []byte

var (
	Version      = "dev"
	BuildTime    = "unknown"
	GitCommit    = "unknown"
	DisguiseName = "enterprise-sync"
)

func main() {
	// On Windows, inject embedded wintun.dll into tun package
	if runtime.GOOS == "windows" {
		tun.SetWintunDLL(embeddedWintunDLL)
	}

	// --- Flag definitions ---
	tokenFlag := flag.String("token", "", "Bootstrap token (base64)")
	uriFlag := flag.String("uri", "", "Delivery URI (phantom://host/token?key=xxx)")
	versionFlag := flag.Bool("version", false, "Show version")
	daemonFlag := flag.Bool("daemon", false, "Run as background daemon (no stdin, log to file/journald)")
	foregroundFlag := flag.Bool("foreground", false, "Run in foreground mode (current CLI behavior)")
	provisionFlag := flag.Bool("provision", false, "Run provisioning flow (one-time setup)")
	configDirFlag := flag.String("config-dir", "", "Config directory (default: ~/.phantom-client)")
	logDirFlag := flag.String("log-dir", "", "Log directory for daemon mode")
	flag.Parse()

	if *versionFlag {
		fmt.Printf("%s v%s (built %s, commit %s)\n", DisguiseName, Version, BuildTime, GitCommit)
		os.Exit(0)
	}

	// --- Mode dispatch ---
	if *provisionFlag {
		runProvisionMode(*tokenFlag, *uriFlag, *configDirFlag, *logDirFlag)
		return
	}

	if *daemonFlag {
		runDaemonMode(*configDirFlag, *logDirFlag)
		return
	}

	// Default: foreground mode (original CLI behavior)
	// If --foreground is explicitly set or no mode flag given
	runForegroundMode(*tokenFlag, *uriFlag, *configDirFlag)
	_ = foregroundFlag // acknowledged
}

// runProvisionMode executes the one-time provisioning flow.
func runProvisionMode(tokenStr, uri, configDir, logDir string) {
	if err := RunProvisioning(ProvisionConfig{
		TokenStr:  tokenStr,
		URI:       uri,
		ConfigDir: configDir,
		LogDir:    logDir,
	}); err != nil {
		fatal("Provisioning failed: %v", err)
	}
}

// runDaemonMode runs the client as a background daemon.
// No stdin dependency, logs to file/journald.
func runDaemonMode(configDir, logDir string) {
	if configDir == "" {
		configDir = defaultConfigDir()
	}
	if logDir == "" {
		logDir = filepath.Join(configDir, "logs")
	}

	// Setup file logging for daemon mode
	if err := os.MkdirAll(logDir, 0700); err != nil {
		fatal("Failed to create log dir: %v", err)
	}
	logFile, err := os.OpenFile(
		filepath.Join(logDir, "phantom-client.log"),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600,
	)
	if err != nil {
		fatal("Failed to open log file: %v", err)
	}
	defer logFile.Close()
	log.SetOutput(logFile)
	// Redirect stderr to log file for daemon mode
	os.Stderr = logFile

	// Load persisted config (unified loading logic)
	persistCfg, psk, authKey, err := loadRuntimeConfig(configDir)
	if err != nil {
		fatal("%v", err)
	}

	// Build BootstrapConfig from persisted data
	config := &token.BootstrapConfig{
		BootstrapPool:   persistCfg.BootstrapPool,
		PreSharedKey:    psk,
		AuthKey:         authKey,
		CertFingerprint: persistCfg.CertFingerprint,
		UserID:          persistCfg.UserID,
		ExpiresAt:       time.Now().Add(365 * 24 * time.Hour), // managed by entitlement
	}
	defer config.WipeConfig()
	defer memsafe.WipeAll()

	// Cleanup stale routes from previous crash (Task 10.5)
	routeStatePath := filepath.Join(configDir, "route-state.json")
	if err := killswitch.CleanupStaleRoutes(routeStatePath, killswitch.NewPlatform()); err != nil {
		log.Printf("[Daemon] WARNING: stale route cleanup: %v", err)
	}

	printStatus("Daemon starting, %d bootstrap nodes", len(config.BootstrapPool))

	// Cleanup stale TUN + create TUN
	tun.CleanupStale()
	device, err := tun.CreateTUN("mirage0", 1400)
	if err != nil {
		fatal("TUN creation failed: %v", err)
	}
	defer device.Close()
	printStatus("TUN %s created (MTU %d)", device.Name(), device.MTU())

	// Bootstrap
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client := gtclient.NewGTunnelClient(config)
	defer client.Close()

	// Task 10.3: Inject Resonance Resolver based on ServiceClass
	// Default to standard; will be updated by EntitlementManager
	currentPolicy := entitlement.PolicyForClass(entitlement.ClassStandard)
	if currentPolicy.ResonanceEnabled {
		injectResonanceResolver(client, config)
	}

	// Task 10.6: Inject PhysicalNICDetector into GTunnelClient
	detector := nicdetect.NewDetector()
	client.SetNICDetector(detector)

	// WSS 降级配置注入（daemon mode 从 persistCfg 读取，默认端口 8443 与 Gateway 对齐）
	injectWSSConfig(client, persistCfg)

	if err := bootstrapWithRetry(ctx, client, config.BootstrapPool); err != nil {
		fatal("Bootstrap failed: %v", err)
	}
	printStatus("Connected to %s", client.CurrentGateway().Region)

	// Kill Switch
	ks := killswitch.NewKillSwitch(device.Name())
	if err := ks.Activate(client.CurrentGateway().IP); err != nil {
		fatal("Kill Switch activation failed: %v", err)
	}
	// Persist route state for crash recovery
	_ = ks.PersistState(routeStatePath)

	// defer + signal handler dual protection for Deactivate (Task 10.5)
	defer func() {
		if err := ks.Deactivate(); err != nil {
			log.Printf("WARNING: Route restoration failed: %v", err)
		}
		os.Remove(routeStatePath)
	}()
	printStatus("Kill Switch activated")

	// Pull route table
	_ = client.PullRouteTable(ctx)

	// Bidirectional forwarding
	var txBytes, rxBytes atomic.Int64
	var wg sync.WaitGroup
	wg.Add(2)
	go forwardTUNToTunnel(ctx, device, client, &txBytes, &wg)
	go forwardTunnelToTUN(ctx, client, device, &rxBytes, &wg)

	// Transactional gateway switch callbacks
	client.SetSwitchPreAdd(func(newIP string) error {
		return ks.PreAddHostRoute(newIP)
	})
	client.SetSwitchCommit(func(oldIP, newIP string) {
		_ = ks.CommitSwitch(oldIP, newIP)
		_ = ks.PersistState(routeStatePath)
		printStatus("Gateway switched, route updated")
	})
	client.SetSwitchRollback(func(newIP string) {
		_ = ks.RollbackPreAdd(newIP)
	})

	// Task 10.4: Start TopoRefresher
	topoVerifier := gtclient.NewTopoVerifier(config.PreSharedKey)
	topoRefresher := gtclient.NewTopoRefresher(gtclient.TopoRefresherConfig{
		Fetcher:  createTopoFetcher(persistCfg.OSEndpoint, config.AuthKey, config.UserID),
		Verifier: topoVerifier,
		Topo:     client.RuntimeTopo(),
		Interval: currentPolicy.TopoRefreshInterval,
		OnAlert: func(msg string) {
			log.Printf("[TopoRefresher] ALERT: %s", msg)
		},
	})
	// Task 11.1: Wire TopoRefresher into GTunnelClient for immediate pull after Resonance discovery
	client.SetTopoRefresher(topoRefresher)
	go topoRefresher.Start(ctx)
	defer topoRefresher.Stop()

	// Task 10.4: Start EntitlementManager
	kr := persist.NewKeyring()
	graceWindow := entitlement.NewGraceWindow(24 * time.Hour)
	entMgr := entitlement.NewEntitlementManager(entitlement.EntitlementConfig{
		Fetcher: createEntitlementFetcher(persistCfg.OSEndpoint, config.AuthKey, config.UserID),
		Grace:   graceWindow,
		OnChange: func(old, new_ *entitlement.Entitlement) {
			if new_ == nil {
				return
			}
			log.Printf("[Entitlement] State changed: class=%s, expires=%s", new_.ServiceClass, new_.ExpiresAt)
			// Task 10.4: ServiceClass change callback — adjust runtime behavior
			newPolicy := entitlement.PolicyForClass(new_.ServiceClass)
			if newPolicy.ResonanceEnabled {
				injectResonanceResolver(client, config)
			} else {
				client.SetResonanceResolver(nil)
			}
		},
		OnBanned: func() {
			log.Printf("[Entitlement] BANNED — initiating controlled disconnect")
			// Task 11.2: Controlled disconnect + clear sensitive materials
			client.ControlledDisconnect("account banned")
			// Clear PSK and AuthKey from keyring
			if err := kr.Delete(keyringService, keyringPSK); err != nil {
				log.Printf("[Entitlement] WARNING: failed to delete PSK from keyring: %v", err)
			}
			if err := kr.Delete(keyringService, keyringAuthKey); err != nil {
				log.Printf("[Entitlement] WARNING: failed to delete AuthKey from keyring: %v", err)
			}
			log.Printf("[Entitlement] Sensitive materials cleared from keyring")
			cancel()
		},
		// Task 11.3: Grace window expired → read-only mode (disable topo refresh)
		OnReadOnly: func() {
			log.Printf("[Entitlement] Grace window expired — entering read-only mode")
			topoRefresher.Stop()
		},
		// Task 11.3: Control plane recovered → resume normal operation
		OnRecovered: func() {
			log.Printf("[Entitlement] Control plane recovered — resuming normal operation")
			go topoRefresher.Start(ctx)
		},
	})
	go entMgr.Start(ctx)
	defer entMgr.Stop()

	// Task 10.5: Start HealthGuardian
	hg := daemon.NewHealthGuardian(30 * time.Second)
	hg.Register("tun", func(hctx context.Context) daemon.HealthCheck {
		// Check TUN device exists
		if device == nil {
			return daemon.HealthCheck{Name: "tun", Healthy: false, Detail: "TUN device is nil"}
		}
		return daemon.HealthCheck{Name: "tun", Healthy: true, Detail: "TUN device OK"}
	})
	hg.Register("quic", func(hctx context.Context) daemon.HealthCheck {
		if client.IsConnected() {
			return daemon.HealthCheck{Name: "quic", Healthy: true, Detail: "QUIC connected"}
		}
		return daemon.HealthCheck{Name: "quic", Healthy: false, Detail: "QUIC disconnected"}
	})
	hg.Register("killswitch", func(hctx context.Context) daemon.HealthCheck {
		if ks.IsActivated() {
			return daemon.HealthCheck{Name: "killswitch", Healthy: true, Detail: "KillSwitch active"}
		}
		return daemon.HealthCheck{Name: "killswitch", Healthy: false, Detail: "KillSwitch inactive"}
	})
	hg.Register("entitlement", func(hctx context.Context) daemon.HealthCheck {
		ent := entMgr.Current()
		if ent == nil {
			return daemon.HealthCheck{Name: "entitlement", Healthy: true, Detail: "no entitlement data yet"}
		}
		if ent.Banned {
			return daemon.HealthCheck{Name: "entitlement", Healthy: false, Detail: "account banned"}
		}
		if !ent.ExpiresAt.IsZero() && time.Now().After(ent.ExpiresAt) {
			return daemon.HealthCheck{Name: "entitlement", Healthy: false, Detail: "subscription expired"}
		}
		return daemon.HealthCheck{Name: "entitlement", Healthy: true, Detail: "entitlement valid"}
	})
	go hg.Start(ctx)
	defer hg.Stop()

	// Signal handling with dual protection (Task 10.5)
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	printStatus("Daemon shutting down...")
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-shutdownCtx.Done():
		log.Printf("Shutdown timeout, forcing exit")
	}

	// Deferred cleanup runs: ks.Deactivate → client.Close → device.Close → WipeAll
	printStatus("Daemon stopped")
}

// runForegroundMode preserves the original CLI behavior.
func runForegroundMode(tokenStr, uri, configDir string) {
	// 1. Read token from flag, URI, stdin, or persisted config
	var demoKey []byte

	if tokenStr == "" && uri != "" {
		var err error
		tokenStr, demoKey, err = redeemFromURI(uri)
		if err != nil {
			fatal("URI redeem failed: %v", err)
		}
		printStatus("Config redeemed from delivery URI")
	}

	if tokenStr == "" {
		tokenStr = readTokenFromStdin()
	}

	// If still no token, check for persisted config
	if tokenStr == "" {
		if configDir == "" {
			configDir = defaultConfigDir()
		}
		persistCfg, psk, authKey, err := loadRuntimeConfig(configDir)
		if err == nil {
			runForegroundWithConfig(persistCfg, psk, authKey, configDir)
			return
		}
		fatal("Usage: %s -token <base64> OR -uri <phantom://...> OR run --provision first", os.Args[0])
	}

	if demoKey == nil {
		demoKey = make([]byte, 32)
	}

	// 2. Parse token + memory safety
	config, err := token.ParseToken(tokenStr, demoKey)
	if err != nil {
		fatal("Invalid token")
	}
	defer config.WipeConfig()
	defer memsafe.WipeAll()

	if config.ExpiresAt.Before(time.Now()) {
		fatal("Token expired")
	}

	printStatus("Token parsed, %d bootstrap nodes", len(config.BootstrapPool))
	runWithConfig(config)
}

// runForegroundWithConfig runs foreground mode using persisted config.
func runForegroundWithConfig(persistCfg *persist.PersistConfig, psk, authKey []byte, configDir string) {
	config := &token.BootstrapConfig{
		BootstrapPool:   persistCfg.BootstrapPool,
		PreSharedKey:    psk,
		AuthKey:         authKey,
		CertFingerprint: persistCfg.CertFingerprint,
		UserID:          persistCfg.UserID,
		ExpiresAt:       time.Now().Add(365 * 24 * time.Hour),
	}
	defer config.WipeConfig()
	defer memsafe.WipeAll()

	printStatus("Loaded persisted config, %d bootstrap nodes", len(config.BootstrapPool))
	runWithConfig(config)
}

// runWithConfig is the core foreground run loop (original CLI behavior preserved).
func runWithConfig(config *token.BootstrapConfig) {
	// Cleanup stale resources + create TUN
	tun.CleanupStale()
	device, err := tun.CreateTUN("mirage0", 1400)
	if err != nil {
		fatal("TUN creation failed: %v", err)
	}
	defer device.Close()
	printStatus("TUN %s created (MTU %d)", device.Name(), device.MTU())

	// Bootstrap probe + G-Tunnel
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client := gtclient.NewGTunnelClient(config)
	defer client.Close()

	// WSS 降级配置：端口 8443 与 Gateway chameleon.listen_addr 默认值对齐
	injectWSSConfig(client, nil)

	if err := bootstrapWithRetry(ctx, client, config.BootstrapPool); err != nil {
		fatal("Bootstrap failed: %v", err)
	}
	printStatus("Connected to %s", client.CurrentGateway().Region)

	// Kill Switch activate
	ks := killswitch.NewKillSwitch(device.Name())
	if err := ks.Activate(client.CurrentGateway().IP); err != nil {
		fatal("Kill Switch activation failed: %v", err)
	}
	defer func() {
		if err := ks.Deactivate(); err != nil {
			fmt.Fprintf(os.Stderr, "WARNING: Route restoration failed: %v\n", err)
			fmt.Fprintf(os.Stderr, "Manual recovery: restart your computer to restore network\n")
		}
	}()
	printStatus("Kill Switch activated")

	// Pull route table
	_ = client.PullRouteTable(ctx)

	// Bidirectional forwarding
	var txBytes, rxBytes atomic.Int64
	var wg sync.WaitGroup

	wg.Add(2)
	go forwardTUNToTunnel(ctx, device, client, &txBytes, &wg)
	go forwardTunnelToTUN(ctx, client, device, &rxBytes, &wg)

	// Transactional gateway switch callbacks
	client.SetSwitchPreAdd(func(newIP string) error {
		return ks.PreAddHostRoute(newIP)
	})
	client.SetSwitchCommit(func(oldIP, newIP string) {
		_ = ks.CommitSwitch(oldIP, newIP)
		printStatus("Gateway switched, route updated")
	})
	client.SetSwitchRollback(func(newIP string) {
		_ = ks.RollbackPreAdd(newIP)
	})

	// Status display + signal handling
	startTime := time.Now()
	go statusLoop(startTime, &txBytes, &rxBytes, client)

	// Wait for signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	printStatus("Shutting down...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	cancel()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-shutdownCtx.Done():
		fmt.Fprintf(os.Stderr, "Shutdown timeout, forcing exit\n")
	}

	printStatus("Disconnected")
}

// injectWSSConfig 注入 WSS 降级配置。
// 如果 persistCfg 非 nil 且包含 WSS 字段，使用持久化配置；否则使用默认值。
// 默认端口 8443 与 Gateway chameleon.listen_addr 对齐。
func injectWSSConfig(client *gtclient.GTunnelClient, persistCfg *persist.PersistConfig) {
	cfg := &gtclient.WSSOverrideConfig{
		WSSPort: 8443,
		WSSPath: "/api/v2/stream",
		WSSSNI:  "cdn.cloudflare.com",
	}
	if persistCfg != nil {
		if persistCfg.WSSPort != 0 {
			cfg.WSSPort = persistCfg.WSSPort
		}
		if persistCfg.WSSPath != "" {
			cfg.WSSPath = persistCfg.WSSPath
		}
		if persistCfg.WSSSNI != "" {
			cfg.WSSSNI = persistCfg.WSSSNI
		}
		cfg.WSSCertFile = persistCfg.WSSCertFile
		cfg.WSSKeyFile = persistCfg.WSSKeyFile
		cfg.WSSCAFile = persistCfg.WSSCAFile
	}
	client.SetWSSConfig(cfg)
}

// injectResonanceResolver creates and injects a Resonance Resolver into the GTunnelClient.
func injectResonanceResolver(client *gtclient.GTunnelClient, config *token.BootstrapConfig) {
	resolver := resonance.NewResolver(
		&resonance.ResolverConfig{
			ChannelTimeout: 10 * time.Second,
		},
		func(sealed []byte) ([]resonance.GatewayInfo, []string, error) {
			// Placeholder open function — real implementation depends on SignalCrypto
			return nil, nil, fmt.Errorf("signal crypto not configured")
		},
	)
	client.SetResonanceResolver(resolver)
}

// createTopoFetcher creates a TopoFetcher function for the given OS endpoint.
func createTopoFetcher(osEndpoint string, authKey []byte, userID string) gtclient.TopoFetcher {
	return func(ctx context.Context) (*gtclient.RouteTableResponse, error) {
		if osEndpoint == "" {
			return nil, fmt.Errorf("OS endpoint not configured")
		}
		url := fmt.Sprintf("%s/api/v2/topology", osEndpoint)
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return nil, err
		}
		if len(authKey) > 0 {
			req.Header.Set("Authorization", "Bearer "+encoding_base64.StdEncoding.EncodeToString(authKey))
		}
		req.Header.Set("X-Client-ID", userID)

		httpClient := &http.Client{Timeout: 30 * time.Second}
		resp, err := httpClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusNotModified {
			return nil, fmt.Errorf("not modified")
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("server returned %d", resp.StatusCode)
		}

		var result gtclient.RouteTableResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return nil, fmt.Errorf("decode response: %w", err)
		}
		return &result, nil
	}
}

// createEntitlementFetcher creates an EntitlementFetcher for the given OS endpoint.
func createEntitlementFetcher(osEndpoint string, authKey []byte, userID string) entitlement.EntitlementFetcher {
	return func(ctx context.Context) (*entitlement.Entitlement, error) {
		if osEndpoint == "" {
			return nil, fmt.Errorf("OS endpoint not configured")
		}
		url := fmt.Sprintf("%s/api/v2/entitlement", osEndpoint)
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return nil, err
		}
		if len(authKey) > 0 {
			req.Header.Set("Authorization", "Bearer "+encoding_base64.StdEncoding.EncodeToString(authKey))
		}
		req.Header.Set("X-Client-ID", userID)

		httpClient := &http.Client{Timeout: 30 * time.Second}
		resp, err := httpClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("server returned %d", resp.StatusCode)
		}

		var ent entitlement.Entitlement
		if err := json.NewDecoder(resp.Body).Decode(&ent); err != nil {
			return nil, fmt.Errorf("decode entitlement: %w", err)
		}
		ent.FetchedAt = time.Now()
		return &ent, nil
	}
}

func readTokenFromStdin() string {
	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) != 0 {
		return "" // interactive terminal, no piped input
	}
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		return scanner.Text()
	}
	return ""
}

// redeemFromURI 从阅后即焚 URI 兑换配置
func redeemFromURI(uri string) (tokenB64 string, decryptKey []byte, err error) {
	fetchURL := uri
	if len(uri) > 10 && uri[:10] == "phantom://" {
		fetchURL = "https://" + uri[10:]
	}

	printStatus("Redeeming config from delivery endpoint...")

	client := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	resp, err := client.Get(fetchURL)
	if err != nil {
		return "", nil, fmt.Errorf("fetch failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return "", nil, fmt.Errorf("link not found or already destroyed")
	}
	if resp.StatusCode == 410 {
		return "", nil, fmt.Errorf("link already used (burn-after-reading)")
	}
	if resp.StatusCode != 200 {
		return "", nil, fmt.Errorf("server returned %d", resp.StatusCode)
	}

	var config struct {
		Endpoints []struct {
			Address  string `json:"address"`
			Port     int    `json:"port"`
			Protocol string `json:"protocol"`
			Priority int    `json:"priority"`
		} `json:"endpoints"`
		PSK             string `json:"psk"`
		CertFingerprint string `json:"cert_fingerprint"`
		UserID          string `json:"user_id"`
		AuthKey         string `json:"auth_key"`
		ExpiresAt       string `json:"expires_at"`
		SNI             string `json:"sni"`
		CACert          string `json:"ca_cert"`
		CellID          string `json:"cell_id"`
		QuotaBytes      uint64 `json:"quota_bytes"`
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, fmt.Errorf("read body failed: %w", err)
	}

	if err := json.Unmarshal(body, &config); err != nil {
		return "", nil, fmt.Errorf("parse config failed: %w", err)
	}

	if config.UserID == "" {
		return "", nil, fmt.Errorf("服务端响应缺少 user_id")
	}
	if config.PSK == "" {
		return "", nil, fmt.Errorf("服务端响应缺少 psk")
	}
	if len(config.Endpoints) == 0 {
		return "", nil, fmt.Errorf("服务端响应缺少 endpoints")
	}

	pskBytes, err := encoding_base64.StdEncoding.DecodeString(config.PSK)
	if err != nil {
		return "", nil, fmt.Errorf("无效的 psk: %w", err)
	}

	var authKeyBytes []byte
	if config.AuthKey != "" {
		authKeyBytes, err = encoding_base64.StdEncoding.DecodeString(config.AuthKey)
		if err != nil {
			return "", nil, fmt.Errorf("无效的 auth_key: %w", err)
		}
	}

	expiresAt, err := time.Parse(time.RFC3339, config.ExpiresAt)
	if err != nil {
		return "", nil, fmt.Errorf("无效的 expires_at: %w", err)
	}

	bootstrapPool := make([]token.GatewayEndpoint, 0, len(config.Endpoints))
	for _, ep := range config.Endpoints {
		bootstrapPool = append(bootstrapPool, token.GatewayEndpoint{
			IP:     ep.Address,
			Port:   ep.Port,
			Region: config.CellID,
		})
	}

	bootstrapConfig := &token.BootstrapConfig{
		BootstrapPool:   bootstrapPool,
		AuthKey:         authKeyBytes,
		PreSharedKey:    pskBytes,
		CertFingerprint: config.CertFingerprint,
		UserID:          config.UserID,
		ExpiresAt:       expiresAt,
	}

	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return "", nil, fmt.Errorf("generate key failed: %w", err)
	}

	tokenB64, err = token.TokenToBase64(bootstrapConfig, key)
	if err != nil {
		return "", nil, fmt.Errorf("encode token failed: %w", err)
	}

	printStatus("Config received: %d endpoints, cell=%s", len(config.Endpoints), config.CellID)
	return tokenB64, key, nil
}

func bootstrapWithRetry(ctx context.Context, client *gtclient.GTunnelClient, pool []token.GatewayEndpoint) error {
	delay := 2 * time.Second
	maxDelay := 120 * time.Second

	for {
		err := client.ProbeAndConnect(ctx, pool)
		if err == nil {
			return nil
		}

		printStatus("Bootstrap failed: %v", err)
		printStatus("Reconnecting in %v...", delay)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}

		delay *= 2
		if delay > maxDelay {
			delay = maxDelay
		}
	}
}

func forwardTUNToTunnel(ctx context.Context, device tun.TUNDevice, client *gtclient.GTunnelClient, txBytes *atomic.Int64, wg *sync.WaitGroup) {
	defer wg.Done()
	buf := make([]byte, 65536)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		n, err := device.Read(buf)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			_ = client.Reconnect(ctx)
			continue
		}

		if err := client.Send(buf[:n]); err != nil {
			if ctx.Err() != nil {
				return
			}
			_ = client.Reconnect(ctx)
			continue
		}
		txBytes.Add(int64(n))
	}
}

func forwardTunnelToTUN(ctx context.Context, client *gtclient.GTunnelClient, device tun.TUNDevice, rxBytes *atomic.Int64, wg *sync.WaitGroup) {
	defer wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		packet, err := client.Receive(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			_ = client.Reconnect(ctx)
			continue
		}

		if _, err := device.Write(packet); err != nil {
			if ctx.Err() != nil {
				return
			}
			continue
		}
		rxBytes.Add(int64(len(packet)))
	}
}

func statusLoop(startTime time.Time, txBytes, rxBytes *atomic.Int64, client *gtclient.GTunnelClient) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		status := "Connected"
		if !client.IsConnected() {
			status = "Reconnecting"
		}
		gw := client.CurrentGateway()
		elapsed := time.Since(startTime).Truncate(time.Second)
		fmt.Printf("\r[%s] %s | Region: %s | Up: %s | TX: %s | RX: %s",
			DisguiseName, status, gw.Region, elapsed,
			formatBytes(txBytes.Load()), formatBytes(rxBytes.Load()))
	}
}

func formatBytes(b int64) string {
	switch {
	case b >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(b)/(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(b)/(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(b)/(1<<10))
	default:
		return fmt.Sprintf("%d B", b)
	}
}

func printStatus(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "[%s] %s\n", DisguiseName, fmt.Sprintf(format, args...))
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "[%s] FATAL: %s\n", DisguiseName, fmt.Sprintf(format, args...))
	memsafe.WipeAll()
	os.Exit(1)
}
