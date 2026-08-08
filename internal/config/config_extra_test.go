package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadConfig_Defaults(t *testing.T) {
	// When optional fields missing, defaults should be applied
	content := `
server:
  port: 9090
backends:
  - url: http://localhost:8080
`
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "defaults.yaml")
	_ = os.WriteFile(cfgPath, []byte(content), 0644)

	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.LoadBalancer.Algorithm != "round-robin" {
		t.Errorf("expected default algorithm round-robin, got %s", cfg.LoadBalancer.Algorithm)
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
	if cfg.Backends[0].Weight != 1 {
		t.Errorf("expected default weight 1 for backend with no weight, got %d", cfg.Backends[0].Weight)
	}
}

func TestLoadConfig_FileNotFound(t *testing.T) {
	_, err := LoadConfig("/nonexistent/path/config.yaml")
	if err == nil {
		t.Errorf("expected error for missing file, got nil")
	}
}

func TestLoadConfig_MalformedYAML(t *testing.T) {
	content := `{invalid yaml: [unclosed`
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "malformed.yaml")
	_ = os.WriteFile(cfgPath, []byte(content), 0644)

	_, err := LoadConfig(cfgPath)
	if err == nil {
		t.Errorf("expected error for malformed YAML, got nil")
	}
}

func TestLoadConfig_EmptyBackendURL(t *testing.T) {
	content := `
server:
  port: 8080
backends:
  - url: ""
`
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "empty_url.yaml")
	_ = os.WriteFile(cfgPath, []byte(content), 0644)

	_, err := LoadConfig(cfgPath)
	if err == nil {
		t.Errorf("expected error for empty backend URL, got nil")
	}
}
