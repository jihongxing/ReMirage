// Package security - mTLS 证书管理与影子认证
package security

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log"
	"os"
	"sync"

	"github.com/fsnotify/fsnotify"
)

// TLSConfig 对应 gateway.yaml 中 mcc.tls 段
type TLSConfig struct {
	Enabled    bool   `yaml:"enabled"`
	CertFile   string `yaml:"cert_file"`
	KeyFile    string `yaml:"key_file"`
	CAFile     string `yaml:"ca_file"`
	ServerName string `yaml:"server_name"`
}

// TLSManager mTLS 证书管理器
type TLSManager struct {
	certFile   string
	keyFile    string
	caFile     string
	serverName string
	enabled    bool
	mu         sync.RWMutex
	tlsConfig  *tls.Config
	watcher    *fsnotify.Watcher
}

// NewTLSManager 从配置创建 TLS 管理器
func NewTLSManager(cfg TLSConfig) (*TLSManager, error) {
	tm := &TLSManager{
		certFile:   cfg.CertFile,
		keyFile:    cfg.KeyFile,
		caFile:     cfg.CAFile,
		serverName: cfg.ServerName,
		enabled:    cfg.Enabled,
	}

	if !cfg.Enabled {
		log.Println("[TLS] mTLS 已禁用（开发模式）")
		return tm, nil
	}

	if err := tm.loadCerts(); err != nil {
		return nil, fmt.Errorf("加载证书失败: %w", err)
	}

	log.Println("[TLS] mTLS 证书已加载")
	return tm, nil
}

// loadCerts 加载证书文件
func (tm *TLSManager) loadCerts() error {
	cert, err := tls.LoadX509KeyPair(tm.certFile, tm.keyFile)
	if err != nil {
		return fmt.Errorf("加载证书对失败 (cert=%s, key=%s): %w", tm.certFile, tm.keyFile, err)
	}

	caCert, err := os.ReadFile(tm.caFile)
	if err != nil {
		return fmt.Errorf("读取 CA 文件失败 (%s): %w", tm.caFile, err)
	}

	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caCert) {
		return fmt.Errorf("解析 CA 证书失败: %s", tm.caFile)
	}

	tm.mu.Lock()
	defer tm.mu.Unlock()

	tm.tlsConfig = &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caPool,
		ClientCAs:    caPool,
		ServerName:   tm.serverName,
		MinVersion:   tls.VersionTLS13,
	}

	return nil
}

// GetClientTLSConfig 返回 gRPC 客户端用的 tls.Config
func (tm *TLSManager) GetClientTLSConfig() (*tls.Config, error) {
	if !tm.enabled {
		return nil, nil
	}

	tm.mu.RLock()
	defer tm.mu.RUnlock()

	if tm.tlsConfig == nil {
		return nil, fmt.Errorf("TLS 配置未初始化")
	}

	return &tls.Config{
		Certificates: tm.tlsConfig.Certificates,
		RootCAs:      tm.tlsConfig.RootCAs,
		ServerName:   tm.serverName,
		MinVersion:   tls.VersionTLS13,
	}, nil
}

// GetServerTLSConfig 返回 gRPC 服务端用的 tls.Config
func (tm *TLSManager) GetServerTLSConfig() (*tls.Config, error) {
	if !tm.enabled {
		return nil, nil
	}

	tm.mu.RLock()
	defer tm.mu.RUnlock()

	if tm.tlsConfig == nil {
		return nil, fmt.Errorf("TLS 配置未初始化")
	}

	return &tls.Config{
		Certificates: tm.tlsConfig.Certificates,
		ClientCAs:    tm.tlsConfig.ClientCAs,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		MinVersion:   tls.VersionTLS13,
	}, nil
}

// StartCertWatcher 启动证书文件监控，60 秒内检测变更并热重载
func (tm *TLSManager) StartCertWatcher(ctx context.Context) error {
	if !tm.enabled {
		return nil
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("创建文件监控器失败: %w", err)
	}
	tm.watcher = watcher

	for _, f := range []string{tm.certFile, tm.keyFile, tm.caFile} {
		if err := watcher.Add(f); err != nil {
			log.Printf("[TLS] 无法监控文件 %s: %v", f, err)
		}
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
					log.Printf("[TLS] 检测到证书变更: %s，重新加载", event.Name)
					if err := tm.loadCerts(); err != nil {
						log.Printf("[TLS] ⚠️ 证书热重载失败，保持旧证书: %v", err)
					} else {
						log.Println("[TLS] ✅ 证书热重载成功")
					}
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Printf("[TLS] 文件监控错误: %v", err)
			}
		}
	}()

	return nil
}

// IsEnabled 返回 TLS 是否启用
func (tm *TLSManager) IsEnabled() bool {
	return tm.enabled
}

// Close 关闭 watcher
func (tm *TLSManager) Close() error {
	if tm.watcher != nil {
		return tm.watcher.Close()
	}
	return nil
}
