package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"phantom-client/pkg/daemon"
	"phantom-client/pkg/gtclient"
	"phantom-client/pkg/persist"
	"phantom-client/pkg/token"
)

const (
	keyringService = "phantom-client"
	keyringPSK     = "psk"
	keyringAuthKey = "auth_key"
)

// ProvisionConfig holds parameters for the provisioning flow.
type ProvisionConfig struct {
	TokenStr  string // base64 token (mutually exclusive with URI)
	URI       string // delivery URI (mutually exclusive with Token)
	ConfigDir string // directory for persisted config
	LogDir    string // directory for daemon logs
}

// RunProvisioning executes the one-time provisioning flow:
// 1. Accept token/URI → redeem config
// 2. Verify connectivity (probe bootstrap pool)
// 3. Persist non-sensitive config to PersistStore
// 4. Store sensitive materials to Keyring
// 5. Register system service
func RunProvisioning(cfg ProvisionConfig) error {
	printStatus("Starting provisioning flow...")

	// --- Step 1: Obtain BootstrapConfig ---
	var bootstrapCfg *token.BootstrapConfig

	if cfg.URI != "" {
		tokenB64, key, err := redeemFromURI(cfg.URI)
		if err != nil {
			return fmt.Errorf("URI redeem: %w", err)
		}
		bootstrapCfg, err = token.ParseToken(tokenB64, key)
		if err != nil {
			return fmt.Errorf("parse redeemed token: %w", err)
		}
	} else if cfg.TokenStr != "" {
		demoKey := make([]byte, 32)
		var err error
		bootstrapCfg, err = token.ParseToken(cfg.TokenStr, demoKey)
		if err != nil {
			return fmt.Errorf("parse token: %w", err)
		}
	} else {
		return fmt.Errorf("either --token or --uri must be provided for provisioning")
	}
	defer bootstrapCfg.WipeConfig()

	if bootstrapCfg.ExpiresAt.Before(time.Now()) {
		return fmt.Errorf("token expired at %s", bootstrapCfg.ExpiresAt)
	}

	printStatus("Token parsed, %d bootstrap nodes", len(bootstrapCfg.BootstrapPool))

	// --- Step 2: Verify connectivity ---
	printStatus("Verifying connectivity...")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client := gtclient.NewGTunnelClient(bootstrapCfg)
	err := client.ProbeAndConnect(ctx, bootstrapCfg.BootstrapPool)
	client.Close()
	if err != nil {
		return fmt.Errorf("connectivity verification failed: %w", err)
	}
	printStatus("Connectivity verified")

	// --- Step 3: Persist non-sensitive config ---
	configDir := cfg.ConfigDir
	if configDir == "" {
		configDir = defaultConfigDir()
	}
	configPath := filepath.Join(configDir, "config.json")

	persistCfg := &persist.PersistConfig{
		BootstrapPool:   bootstrapCfg.BootstrapPool,
		CertFingerprint: bootstrapCfg.CertFingerprint,
		UserID:          bootstrapCfg.UserID,
		OSEndpoint:      deriveOSEndpoint(bootstrapCfg),
	}

	if err := persist.Save(configPath, persistCfg); err != nil {
		return fmt.Errorf("persist config: %w", err)
	}
	printStatus("Config persisted to %s", configPath)

	// --- Step 4: Store sensitive materials to Keyring ---
	kr := persist.NewKeyring()
	if len(bootstrapCfg.PreSharedKey) > 0 {
		if err := kr.Store(keyringService, keyringPSK, bootstrapCfg.PreSharedKey); err != nil {
			log.Printf("[Provision] WARNING: failed to store PSK in keyring: %v", err)
		}
	}
	if len(bootstrapCfg.AuthKey) > 0 {
		if err := kr.Store(keyringService, keyringAuthKey, bootstrapCfg.AuthKey); err != nil {
			log.Printf("[Provision] WARNING: failed to store AuthKey in keyring: %v", err)
		}
	}
	printStatus("Sensitive materials stored in keyring")

	// --- Step 5: Register system service ---
	logDir := cfg.LogDir
	if logDir == "" {
		logDir = filepath.Join(configDir, "logs")
	}
	if err := os.MkdirAll(logDir, 0700); err != nil {
		log.Printf("[Provision] WARNING: failed to create log dir: %v", err)
	}

	execPath, err := os.Executable()
	if err != nil {
		execPath = os.Args[0]
	}

	dm := daemon.NewDaemonManager("phantom-client", execPath, configDir, logDir)
	if err := dm.Install(); err != nil {
		log.Printf("[Provision] WARNING: service registration failed: %v (manual registration may be needed)", err)
	} else {
		printStatus("System service registered")
	}

	printStatus("Provisioning complete! Start with: --daemon or systemctl start phantom-client")
	return nil
}

// defaultConfigDir returns the platform-appropriate config directory.
func defaultConfigDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".phantom-client"
	}
	return filepath.Join(home, ".phantom-client")
}

// loadRuntimeConfig 统一配置装载逻辑（daemon 和 foreground 共用）。
// 加载 PersistConfig → 从 Keyring 加载 PSK/AuthKey → 校验必填字段 → 返回。
func loadRuntimeConfig(configDir string) (*persist.PersistConfig, []byte, []byte, error) {
	configPath := filepath.Join(configDir, "config.json")
	persistCfg, err := persist.Load(configPath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("no valid config found at %s — run with --provision first", configPath)
	}

	// 校验必填字段
	if len(persistCfg.BootstrapPool) == 0 {
		return nil, nil, nil, fmt.Errorf("bootstrap_pool is empty in config.json — re-run provisioning with latest token")
	}
	if persistCfg.UserID == "" {
		return nil, nil, nil, fmt.Errorf("user_id missing in config.json — re-run provisioning with latest token")
	}
	if persistCfg.OSEndpoint == "" {
		return nil, nil, nil, fmt.Errorf("os_endpoint missing in config.json — re-run provisioning with latest token")
	}

	// 从 Keyring 加载敏感材料
	kr := persist.NewKeyring()
	psk, err := kr.Load(keyringService, keyringPSK)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to load PSK from keyring: %w — run with --provision first", err)
	}
	authKey, _ := kr.Load(keyringService, keyringAuthKey) // optional

	return persistCfg, psk, authKey, nil
}

// deriveOSEndpoint 从 BootstrapConfig 推导 OS 控制面地址。
// 优先级：token.OSEndpoint > bootstrap pool 第一个 gateway 地址推导
func deriveOSEndpoint(cfg *token.BootstrapConfig) string {
	// 1. 如果 token 直接包含 OSEndpoint
	if cfg.OSEndpoint != "" {
		return cfg.OSEndpoint
	}

	// 2. 从 bootstrap pool 第一个 gateway 推导
	if len(cfg.BootstrapPool) > 0 {
		ep := cfg.BootstrapPool[0]
		port := ep.Port
		if port == 0 {
			port = 443
		}
		return fmt.Sprintf("https://%s:%d", ep.IP, port)
	}

	return ""
}
