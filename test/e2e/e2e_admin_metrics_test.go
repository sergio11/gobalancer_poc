package e2e

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gobalancer/internal/api"
	"gobalancer/internal/backend"
	"gobalancer/internal/config"
)

// TestE2E_AdminAPI_HealthAndBackends verifies the admin health endpoint and
// the full backend listing through the live pipeline.
func TestE2E_AdminAPI_HealthAndBackends(t *testing.T) {
	b1 := newMockBackend(t, "1", nil)
	b2 := newMockBackend(t, "2", nil)

	env := setupTestLB(t, lbOptions{
		algorithm: "round-robin",
		backends:  []*mockBackend{b1, b2},
		weights:   []int{3, 1},
	})

	// /health
	resp, body := env.get("/health", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on /health, got %d", resp.StatusCode)
	}
	if !strings.Contains(body, `"UP"`) || !strings.Contains(body, "GoBalancer") {
		t.Errorf("unexpected /health body: %s", body)
	}

	// /api/backends
	resp, body = env.get("/api/backends", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on /api/backends, got %d", resp.StatusCode)
	}
	var backends []api.BackendDTO
	if err := json.NewDecoder(strings.NewReader(body)).Decode(&backends); err != nil {
		t.Fatalf("failed to decode backends: %v", err)
	}
	if len(backends) != 2 {
		t.Fatalf("expected 2 backends, got %d", len(backends))
	}

	for _, b := range backends {
		if b.ID == "" || b.URL == "" || b.Status == "" {
			t.Errorf("backend entry missing required fields: %+v", b)
		}
		if b.Status != backend.StatusHealthy {
			t.Errorf("expected healthy status, got %+v", b)
		}
		if b.Weight <= 0 {
			t.Errorf("expected positive weight, got %+v", b)
		}
		if b.LastHealthCheck.IsZero() {
			t.Errorf("expected last_health_check to be set, got %+v", b)
		}
	}

	// /api/stats initial state
	stats := fetchStats(t, env)
	if stats.TotalBackends != 2 || stats.HealthyBackends != 2 || stats.UnhealthyCount != 0 {
		t.Errorf("unexpected initial stats: %+v", stats)
	}
}

// TestE2E_AdminAPI_Reload_InvalidConfig verifies reload with an unusable
// config path returns a 400 and leaves the balancer untouched.
func TestE2E_AdminAPI_Reload_InvalidConfig(t *testing.T) {
	b1 := newMockBackend(t, "1", nil)
	b2 := newMockBackend(t, "2", nil)

	env := setupTestLB(t, lbOptions{
		algorithm:  "round-robin",
		backends:   []*mockBackend{b1, b2},
		configPath: filepath.Join(t.TempDir(), "does-not-exist.yaml"),
	})

	resp, body := env.post("/api/reload", "")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 on reload with missing config, got %d: %s", resp.StatusCode, body)
	}

	// Balancer still serves traffic after the failed reload.
	for i := 0; i < 4; i++ {
		r, _ := env.get("/test", "")
		if r.StatusCode != http.StatusOK {
			t.Errorf("traffic failed after rejected reload: %d", r.StatusCode)
		}
	}
}

// TestE2E_ReloadSwapsBackends is the real dynamic-reload scenario: setup is
// wired to a writable config file and the swap is verified through the API.
func TestE2E_ReloadSwapsBackends(t *testing.T) {
	b1 := newMockBackend(t, "1", nil)
	b2 := newMockBackend(t, "2", nil)
	b3 := newMockBackend(t, "3", nil)

	cfgPath := filepath.Join(t.TempDir(), "reload.yaml")
	writeConfig := func(urls ...string) {
		var b strings.Builder
		b.WriteString("server:\n  port: 8080\nloadBalancer:\n  algorithm: round-robin\n")
		b.WriteString("healthCheck:\n  interval: 5s\n  timeout: 2s\n  maxFailures: 3\n")
		b.WriteString("backends:\n")
		for _, u := range urls {
			b.WriteString("  - url: " + u + "\n")
		}
		if err := os.WriteFile(cfgPath, []byte(b.String()), 0644); err != nil {
			t.Fatalf("failed to write config: %v", err)
		}
	}
	writeConfig(b1.url(), b2.url(), b3.url())

	env := setupTestLB(t, lbOptions{
		algorithm:  "round-robin",
		backends:   []*mockBackend{b1, b2, b3},
		configPath: cfgPath,
	})

	// 1. Traffic flows across all three backends.
	for i := 0; i < 3; i++ {
		env.get("/test", "")
	}
	if b3.hitCount() == 0 {
		t.Errorf("backend 3 should receive traffic before reload")
	}

	// 2. Reload with a config that drops backend 3.
	writeConfig(b1.url(), b2.url())
	resp, body := env.post("/api/reload", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on reload, got %d: %s", resp.StatusCode, body)
	}
	if !strings.Contains(body, "reloaded") {
		t.Errorf("unexpected reload response body: %s", body)
	}

	// 3. Stats now reflect the reduced backend set.
	stats := fetchStats(t, env)
	if stats.TotalBackends != 2 {
		t.Fatalf("expected 2 backends after reload, got %+v", stats)
	}

	// 4. Backend 3 no longer receives traffic.
	b3Before := b3.hitCount()
	for i := 0; i < 6; i++ {
		env.get("/test", "")
	}
	if b3.hitCount() != b3Before {
		t.Errorf("removed backend 3 should not receive traffic after reload, got %d", b3.hitCount())
	}
}

// TestE2E_MetricsEndpoint verifies Prometheus-compatible output reflects
// real traffic through the full pipeline.
func TestE2E_MetricsEndpoint(t *testing.T) {
	b1 := newMockBackend(t, "1", nil)
	b2 := newMockBackend(t, "2", nil)

	env := setupTestLB(t, lbOptions{
		algorithm: "round-robin",
		backends:  []*mockBackend{b1, b2},
	})

	for i := 0; i < 4; i++ {
		env.get("/test", "")
	}

	resp, body := env.get("/metrics", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on /metrics, got %d", resp.StatusCode)
	}

	required := []string{
		"requests_total 4",
		"healthy_backends 2",
		"unhealthy_backends 0",
		"backend_latency_ms{backend=\"backend-1\"",
		"backend_active_connections{backend=\"backend-1\"",
	}
	for _, want := range required {
		if !strings.Contains(body, want) {
			t.Errorf("metrics output missing %q\n---\n%s", want, body)
		}
	}
	if strings.Contains(body, "backend_errors_total 1") {
		t.Errorf("did not expect backend errors, got:\n%s", body)
	}
}

// TestE2E_MetricsCountsErrorsWhenAllDown verifies backend_errors_total is
// incremented when no healthy backend is available.
func TestE2E_MetricsCountsErrorsWhenAllDown(t *testing.T) {
	b1 := newMockBackend(t, "1", nil)
	b2 := newMockBackend(t, "2", nil)

	env := setupTestLB(t, lbOptions{
		algorithm: "round-robin",
		backends:  []*mockBackend{b1, b2},
		healthCfg: config.HealthCheckConfig{
			Interval:    15 * time.Millisecond,
			Timeout:     50 * time.Millisecond,
			MaxFailures: 1,
		},
		runChecker: true,
	})

	b1.setHealthy(false)
	b2.setHealthy(false)
	waitForHealthyCount(t, env, 0)

	resp, _ := env.get("/test", "")
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", resp.StatusCode)
	}

	_, body := env.get("/metrics", "")
	if !strings.Contains(body, "backend_errors_total 1") {
		t.Errorf("expected backend_errors_total 1, got:\n%s", body)
	}
	if !strings.Contains(body, "unhealthy_backends 2") {
		t.Errorf("expected unhealthy_backends 2, got:\n%s", body)
	}
}
