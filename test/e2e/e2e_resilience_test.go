package e2e

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"gobalancer/internal/backend"
	"gobalancer/internal/config"
)

// TestE2E_AllBackendsDown_Returns503 verifies that when every backend is
// unhealthy the load balancer answers 503 Service Unavailable.
func TestE2E_AllBackendsDown_Returns503(t *testing.T) {
	b1 := newMockBackend(t, "1", nil)
	b2 := newMockBackend(t, "2", nil)
	b3 := newMockBackend(t, "3", nil)

	env := setupTestLB(t, lbOptions{
		algorithm: "round-robin",
		backends:  []*mockBackend{b1, b2, b3},
		healthCfg: config.HealthCheckConfig{
			Interval:    15 * time.Millisecond,
			Timeout:     50 * time.Millisecond,
			MaxFailures: 1,
		},
		runChecker: true,
	})

	b1.setHealthy(false)
	b2.setHealthy(false)
	b3.setHealthy(false)
	waitForHealthyCount(t, env, 0)

	resp, body := env.get("/test", "")
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when all backends are down, got %d", resp.StatusCode)
	}
	if !strings.Contains(body, "Service Unavailable") {
		t.Errorf("expected 503 body to mention Service Unavailable, got %q", body)
	}

	// No backend may have received any traffic
	if b1.hitCount()+b2.hitCount()+b3.hitCount() != 0 {
		t.Errorf("no backend should receive traffic when all are down")
	}
}

// TestE2E_BackendRecoveryCycle verifies automatic reincorporation: a failing
// backend is bypassed and, once healthy again, traffic flows back to it.
func TestE2E_BackendRecoveryCycle(t *testing.T) {
	b1 := newMockBackend(t, "1", nil)
	b2 := newMockBackend(t, "2", nil)
	b3 := newMockBackend(t, "3", nil)

	env := setupTestLB(t, lbOptions{
		algorithm: "round-robin",
		backends:  []*mockBackend{b1, b2, b3},
		healthCfg: config.HealthCheckConfig{
			Interval:    15 * time.Millisecond,
			Timeout:     50 * time.Millisecond,
			MaxFailures: 1,
		},
		runChecker: true,
	})

	// 1. Take backend 2 down
	b2.setHealthy(false)
	waitForBackendStatus(t, env.pool, "backend-2", backend.StatusUnhealthy)

	for i := 0; i < 3; i++ {
		env.get("/test", "")
	}
	b2HitsDown := b2.hitCount()
	if b2HitsDown != 0 {
		t.Fatalf("backend 2 received traffic while unhealthy: %d", b2HitsDown)
	}

	// 2. Bring backend 2 back and wait for the checker to mark it HEALTHY
	b2.setHealthy(true)
	waitForBackendStatus(t, env.pool, "backend-2", backend.StatusHealthy)

	c1Before := b1.hitCount()
	c3Before := b3.hitCount()
	for i := 0; i < 3; i++ {
		env.get("/test", "")
	}

	if b2.hitCount() == 0 {
		t.Errorf("recovered backend 2 should receive traffic again")
	}
	if b1.hitCount() == c1Before && b3.hitCount() == c3Before && b2.hitCount() == 0 {
		t.Errorf("no traffic observed after recovery")
	}
}
