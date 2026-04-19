package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/tls"
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"phantom-client/pkg/gtclient"
	"phantom-client/pkg/killswitch"
	"phantom-client/pkg/memsafe"
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

	tokenFlag := flag.String("token", "", "Bootstrap token (base64)")
	uriFlag := flag.String("uri", "", "Delivery URI (phantom://host/token?key=xxx)")
	versionFlag := flag.Bool("version", false, "Show version")
	flag.Parse()

	if *versionFlag {
		fmt.Printf("%s v%s (built %s, commit %s)\n", DisguiseName, Version, BuildTime, GitCommit)
		os.Exit(0)
	}

	// 1. Read token from flag, URI, or stdin
	tokenStr := *tokenFlag
	var demoKey []byte

	if tokenStr == "" && *uriFlag != "" {
		// 从阅后即焚 URI 获取 token
		var err error
		tokenStr, demoKey, err = redeemFromURI(*uriFlag)
		if err != nil {
			fatal("URI redeem failed: %v", err)
		}
		printStatus("Config redeemed from delivery URI")
	}

	if tokenStr == "" {
		tokenStr = readTokenFromStdin()
	}
	if tokenStr == "" {
		fatal("Usage: %s -token <base64> OR -uri <phantom://...>", os.Args[0])
	}

	// Fallback key for direct token mode
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

	// Check expiry
	if config.ExpiresAt.Before(time.Now()) {
		fatal("Token expired")
	}

	printStatus("Token parsed, %d bootstrap nodes", len(config.BootstrapPool))

	// 3. Cleanup stale resources + create TUN
	tun.CleanupStale()
	device, err := tun.CreateTUN("mirage0", 1400)
	if err != nil {
		fatal("TUN creation failed: %v", err)
	}
	defer device.Close()
	printStatus("TUN %s created (MTU %d)", device.Name(), device.MTU())

	// 4. Bootstrap probe + G-Tunnel
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client := gtclient.NewGTunnelClient(config)
	defer client.Close()

	if err := bootstrapWithRetry(ctx, client, config.BootstrapPool); err != nil {
		fatal("Bootstrap failed: %v", err)
	}
	printStatus("Connected to %s", client.CurrentGateway().Region)

	// 5. Kill Switch activate
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

	// 6. Pull route table
	_ = client.PullRouteTable(ctx)

	// 7. Bidirectional forwarding
	var txBytes, rxBytes atomic.Int64
	var wg sync.WaitGroup

	wg.Add(2)
	go forwardTUNToTunnel(ctx, device, client, &txBytes, &wg)
	go forwardTunnelToTUN(ctx, client, device, &rxBytes, &wg)

	// 8. Gateway switch listener
	client.OnGatewaySwitch(func(newIP string) {
		if err := ks.UpdateGatewayRoute(newIP); err != nil {
			fmt.Fprintf(os.Stderr, "WARNING: Route update failed: %v\n", err)
		}
		printStatus("Gateway switched, route updated")
	})

	// 9. Status display + signal handling
	startTime := time.Now()
	go statusLoop(startTime, &txBytes, &rxBytes, client)

	// Wait for signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	printStatus("Shutting down...")

	// 10. Graceful shutdown with 30s timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	cancel() // cancel main context to stop forwarding

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

	// Deferred cleanup runs: ks.Deactivate → client.Close → device.Close → WipeAll
	printStatus("Disconnected")
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
// URI 格式: phantom://host:port/delivery/TOKEN?key=BASE64KEY
// 或 HTTPS: https://host:port/api/delivery/TOKEN?key=BASE64KEY
func redeemFromURI(uri string) (tokenB64 string, decryptKey []byte, err error) {
	// 将 phantom:// 转换为 https://
	fetchURL := uri
	if len(uri) > 10 && uri[:10] == "phantom://" {
		fetchURL = "https://" + uri[10:]
	}

	printStatus("Redeeming config from delivery endpoint...")

	client := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // 首次连接允许自签名
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

	// 解析返回的 JSON 配置
	var config struct {
		PrivateKey string `json:"private_key"`
		PublicKey  string `json:"public_key"`
		Endpoints  []struct {
			Address  string `json:"address"`
			Port     int    `json:"port"`
			Protocol string `json:"protocol"`
			Priority int    `json:"priority"`
		} `json:"endpoints"`
		SNI        string `json:"sni"`
		CACert     string `json:"ca_cert"`
		UID        string `json:"uid"`
		CellID     string `json:"cell_id"`
		QuotaBytes uint64 `json:"quota_bytes"`
		ExpiresAt  string `json:"expires_at"`
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, fmt.Errorf("read body failed: %w", err)
	}

	if err := json.Unmarshal(body, &config); err != nil {
		return "", nil, fmt.Errorf("parse config failed: %w", err)
	}

	// 将配置转换为 BootstrapConfig token
	bootstrapPool := make([]token.GatewayEndpoint, 0, len(config.Endpoints))
	for _, ep := range config.Endpoints {
		bootstrapPool = append(bootstrapPool, token.GatewayEndpoint{
			IP:     ep.Address,
			Port:   ep.Port,
			Region: config.CellID,
		})
	}

	bootstrapConfig := &token.BootstrapConfig{
		BootstrapPool: bootstrapPool,
		ExpiresAt:     time.Now().Add(30 * 24 * time.Hour),
	}

	// 生成临时加密密钥
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return "", nil, fmt.Errorf("generate key failed: %w", err)
	}

	// 编码为 base64 token
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
			// Trigger reconnect on error
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

		packet, err := client.Receive()
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

func printStatus(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "[%s] %s\n", DisguiseName, fmt.Sprintf(format, args...))
}

func fatal(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "[%s] FATAL: %s\n", DisguiseName, fmt.Sprintf(format, args...))
	memsafe.WipeAll()
	os.Exit(1)
}
