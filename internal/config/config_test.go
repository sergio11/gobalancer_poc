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
loadBalancer:
  algorithm: round-robin
healthCheck:
  interval: 5s
  timeout: 2s
  maxFailures: 3
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
	if cfg.LoadBalancer.Algorithm != "round-robin" {
		t.Errorf("expected round-robin, got %s", cfg.LoadBalancer.Algorithm)
	}
	if cfg.HealthCheck.Interval != 5*time.Second {
		t.Errorf("expected interval 5s, got %v", cfg.HealthCheck.Interval)
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
