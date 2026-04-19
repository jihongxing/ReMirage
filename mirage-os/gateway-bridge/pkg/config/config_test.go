package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_ValidConfig(t *testing.T) {
	content := `
grpc:
  port: 50051
database:
  dsn: "postgres://test:test@localhost:5432/test"
redis:
  addr: "localhost:6379"
quota:
  business_price_per_gb: 0.10
  defense_price_per_gb: 0.05
intel:
  ban_threshold: 100
`
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.GRPC.Port != 50051 {
		t.Errorf("expected port 50051, got %d", cfg.GRPC.Port)
	}
	if cfg.Database.DSN != "postgres://test:test@localhost:5432/test" {
		t.Errorf("unexpected DSN: %s", cfg.Database.DSN)
	}
	if cfg.Quota.BusinessPricePerGB != 0.10 {
		t.Errorf("expected business price 0.10, got %f", cfg.Quota.BusinessPricePerGB)
	}
}

func TestLoad_MissingDSN(t *testing.T) {
	content := `
grpc:
  port: 50051
redis:
  addr: "localhost:6379"
`
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for missing DSN")
	}
}

func TestLoad_MissingRedis(t *testing.T) {
	content := `
grpc:
  port: 50051
database:
  dsn: "postgres://test:test@localhost:5432/test"
`
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for missing redis addr")
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/config.yaml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoad_DefaultValues(t *testing.T) {
	content := `
database:
  dsn: "postgres://test:test@localhost:5432/test"
redis:
  addr: "localhost:6379"
`
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.GRPC.Port != 50051 {
		t.Errorf("expected default port 50051, got %d", cfg.GRPC.Port)
	}
	if cfg.Intel.BanThreshold != 100 {
		t.Errorf("expected default ban threshold 100, got %d", cfg.Intel.BanThreshold)
	}
	if cfg.Quota.BusinessPricePerGB != 0.10 {
		t.Errorf("expected default business price 0.10, got %f", cfg.Quota.BusinessPricePerGB)
	}
}
