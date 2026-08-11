package e2e

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"gobalancer/internal/api"
	"gobalancer/internal/backend"
	"gobalancer/internal/config"
)

// TestE2E_LoadBalancerTrafficDistributionAndFailover verifies round-robin
// distribution, failover to healthy backends and admin stats end-to-end.
func TestE2E_LoadBalancerTrafficDistributionAndFailover(t *testing.T) {
	b1 := newMockBackend(t, "1", nil)
	b2 := newMockBackend(t, "2", nil)
	b3 := newMockBackend(t, "3", nil)

	env := setupTestLB(t, lbOptions{
		algorithm: "round-robin",
		backends:  []*mockBackend{b1, b2, b3},
		healthCfg: config.HealthCheckConfig{
			Interval:    25 * time.Millisecond,
			Timeout:     50 * time.Millisecond,
			MaxFailures: 1,
		},
		runChecker: true,
	})

	// 1. Send 6 requests and verify Round Robin distribution (2 per backend)
	for i := 0; i < 6; i++ {
		resp, body := env.get("/test", "")
		if resp.StatusCode != http.StatusOK {
			t.Errorf("request %d returned status %d, body %s", i, resp.StatusCode, body)
		}
	}

	if b1.hitCount() != 2 || b2.hitCount() != 2 || b3.hitCount() != 2 {
		t.Errorf("expected even distribution (2 each), got b1=%d, b2=%d, b3=%d",
			b1.hitCount(), b2.hitCount(), b3.hitCount())
	}

	// 2. Simulate backend 2 failure and wait for the checker to mark it unhealthy
	b2.setHealthy(false)
	waitForBackendStatus(t, env.pool, "backend-2", backend.StatusUnhealthy)

	c1Before := b1.hitCount()
	c3Before := b3.hitCount()

	for i := 0; i < 4; i++ {
		resp, _ := env.get("/test", "")
		if resp.StatusCode != http.StatusOK {
			t.Errorf("request after failure %d returned status %d", i, resp.StatusCode)
		}
	}

	if b2.hitCount() != 2 {
		t.Errorf("unhealthy backend 2 should not receive requests, count remains 2, got %d", b2.hitCount())
	}
	if b1.hitCount()-c1Before != 2 || b3.hitCount()-c3Before != 2 {
		t.Errorf("expected 2 requests each to b1 and b3 after b2 failed, got b1=+%d, b3=+%d",
			b1.hitCount()-c1Before, b3.hitCount()-c3Before)
	}

	// 3. Test Admin API /api/stats reflects the failover
	stats := fetchStats(t, env)
	if stats.TotalBackends != 3 {
		t.Errorf("expected 3 total backends in stats, got %d", stats.TotalBackends)
	}
	if stats.HealthyBackends != 2 {
		t.Errorf("expected 2 healthy backends in stats, got %d", stats.HealthyBackends)
	}
	if stats.UnhealthyCount != 1 {
		t.Errorf("expected 1 unhealthy backend in stats, got %d", stats.UnhealthyCount)
	}

	// 4. Verify backend list endpoint reports the unhealthy status
	_, body := env.get("/api/backends", "")
	var backends []api.BackendDTO
	if err := json.NewDecoder(strings.NewReader(body)).Decode(&backends); err != nil {
		t.Fatalf("failed to decode backends: %v", err)
	}
	found := false
	for _, b := range backends {
		if b.ID == "backend-2" && b.Status == backend.StatusUnhealthy {
			found = true
		}
	}
	if !found {
		t.Errorf("expected backend-2 to be reported as UNHEALTHY, got %+v", backends)
	}
}
