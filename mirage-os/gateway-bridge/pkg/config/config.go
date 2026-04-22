package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	GRPC     GRPCConfig     `yaml:"grpc"`
	REST     *RESTConfig    `yaml:"rest"`
	Database DatabaseConfig `yaml:"database"`
	Redis    RedisConfig    `yaml:"redis"`
	Quota    PricingConfig  `yaml:"quota"`
	Intel    IntelConfig    `yaml:"intel"`
	Raft     RaftConfig     `yaml:"raft"`
}

type RESTConfig struct {
	Addr           string `yaml:"addr"`
	InternalSecret string `yaml:"internal_secret"`
}

type RaftConfig struct {
	NodeID    string       `yaml:"node_id"`
	BindAddr  string       `yaml:"bind_addr"`
	DataDir   string       `yaml:"data_dir"`
	Bootstrap bool         `yaml:"bootstrap"`
	Peers     []PeerConfig `yaml:"peers"`
}

type PeerConfig struct {
	ID      string `yaml:"id"`
	Address string `yaml:"address"`
	Voter   bool   `yaml:"voter"`
}

type GRPCConfig struct {
	Port       int      `yaml:"port"`
	TLSEnabled bool     `yaml:"tls_enabled"`
	CertFile   string   `yaml:"cert_file"`
	KeyFile    string   `yaml:"key_file"`
	CAFile     string   `yaml:"ca_file"`
	AllowedCNs []string `yaml:"allowed_cns"`
}

type DatabaseConfig struct {
	DSN string `yaml:"dsn"`
}

type RedisConfig struct {
	Addr     string `yaml:"addr"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
}

type PricingConfig struct {
	BusinessPricePerGB float64 `yaml:"business_price_per_gb"`
	DefensePricePerGB  float64 `yaml:"defense_price_per_gb"`
}

type IntelConfig struct {
	BanThreshold   int `yaml:"ban_threshold"`
	CleanupDays    int `yaml:"cleanup_days"`
	CleanupMinHits int `yaml:"cleanup_min_hits"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	// 环境变量覆盖
	if dsn := os.Getenv("DATABASE_DSN"); dsn != "" {
		cfg.Database.DSN = dsn
	}
	if addr := os.Getenv("REDIS_ADDR"); addr != "" {
		cfg.Redis.Addr = addr
	}

	// 默认值
	if cfg.GRPC.Port == 0 {
		cfg.GRPC.Port = 50051
	}
	if cfg.Intel.BanThreshold == 0 {
		cfg.Intel.BanThreshold = 100
	}
	if cfg.Intel.CleanupDays == 0 {
		cfg.Intel.CleanupDays = 30
	}
	if cfg.Intel.CleanupMinHits == 0 {
		cfg.Intel.CleanupMinHits = 10
	}
	if cfg.Quota.BusinessPricePerGB == 0 {
		cfg.Quota.BusinessPricePerGB = 0.10
	}
	if cfg.Quota.DefensePricePerGB == 0 {
		cfg.Quota.DefensePricePerGB = 0.05
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) validate() error {
	if c.Database.DSN == "" {
		return fmt.Errorf("config: database.dsn is required")
	}
	if c.Redis.Addr == "" {
		return fmt.Errorf("config: redis.addr is required")
	}
	// 生产模式强制 TLS
	if os.Getenv("MIRAGE_ENV") == "production" && !c.GRPC.TLSEnabled {
		return fmt.Errorf("config: grpc.tls_enabled must be true in production mode")
	}
	return nil
}
