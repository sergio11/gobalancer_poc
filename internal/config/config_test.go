package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadConfig_Valid(t *testing.T) {
	content := `
server:
  port: 8080
  logLevel: debug
loadBalancer:
  algorithm: round-robin
healthCheck:
  interval: 5s
  timeout: 2s
  maxFailures: 3
rateLimit:
  rate: 1000
  capacity: 2000
  enabled: true
backends:
  - url: http://localhost:9001
    weight: 5
  - url: http://localhost:9002
    weight: 3
`
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp config: %v", err)
	}

	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if cfg.Server.Port != 8080 {
		t.Errorf("expected port 8080, got %d", cfg.Server.Port)
	}
	if cfg.Server.LogLevel != "debug" {
		t.Errorf("expected logLevel debug, got %s", cfg.Server.LogLevel)
	}
	if cfg.LoadBalancer.Algorithm != "round-robin" {
		t.Errorf("expected round-robin, got %s", cfg.LoadBalancer.Algorithm)
	}
	if cfg.HealthCheck.Interval != 5*time.Second {
		t.Errorf("expected interval 5s, got %v", cfg.HealthCheck.Interval)
	}
	if cfg.RateLimit.Rate != 1000 {
		t.Errorf("expected rateLimit rate 1000, got %f", cfg.RateLimit.Rate)
	}
	if cfg.RateLimit.Capacity != 2000 {
		t.Errorf("expected rateLimit capacity 2000, got %f", cfg.RateLimit.Capacity)
	}
	if !cfg.RateLimit.Enabled {
		t.Errorf("expected rateLimit enabled true, got false")
	}
	if len(cfg.Backends) != 2 {
		t.Errorf("expected 2 backends, got %d", len(cfg.Backends))
	}
}

func TestLoadConfig_InvalidPort(t *testing.T) {
	content := `
server:
  port: 999999
backends:
  - url: http://localhost:9001
`
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "invalid_port.yaml")
	_ = os.WriteFile(cfgPath, []byte(content), 0644)

	_, err := LoadConfig(cfgPath)
	if err == nil {
		t.Fatalf("expected error for invalid port, got nil")
	}
}

func TestLoadConfig_NoBackends(t *testing.T) {
	content := `
server:
  port: 8080
backends: []
`
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "no_backends.yaml")
	_ = os.WriteFile(cfgPath, []byte(content), 0644)

	_, err := LoadConfig(cfgPath)
	if err == nil {
		t.Fatalf("expected error for empty backends, got nil")
	}
}

func TestLoadConfig_NewDefaults(t *testing.T) {
	content := `
server:
  port: 8080
backends:
  - url: http://localhost:9001
`
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "defaults.yaml")
	_ = os.WriteFile(cfgPath, []byte(content), 0644)

	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if cfg.Server.LogLevel != "info" {
		t.Errorf("expected default logLevel info, got %s", cfg.Server.LogLevel)
	}
	if cfg.RateLimit.Rate != 5000 {
		t.Errorf("expected default rateLimit rate 5000, got %f", cfg.RateLimit.Rate)
	}
	if cfg.RateLimit.Capacity != 10000 {
		t.Errorf("expected default rateLimit capacity 10000, got %f", cfg.RateLimit.Capacity)
	}
	if cfg.HealthCheck.Interval != 5*time.Second {
		t.Errorf("expected default interval 5s, got %v", cfg.HealthCheck.Interval)
	}
	if cfg.HealthCheck.Timeout != 2*time.Second {
		t.Errorf("expected default timeout 2s, got %v", cfg.HealthCheck.Timeout)
	}
	if cfg.HealthCheck.MaxFailures != 3 {
		t.Errorf("expected default maxFailures 3, got %d", cfg.HealthCheck.MaxFailures)
	}
}
