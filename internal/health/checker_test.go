package health

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"gobalancer/internal/backend"
	"gobalancer/internal/config"
)

func TestHealthChecker_HealthyAndUnhealthy(t *testing.T) {
	// Create mock healthy backend server
	healthyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer healthyServer.Close()

	// Create mock failing backend server
	unhealthyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer unhealthyServer.Close()

	cfg := []config.BackendConfig{
		{URL: healthyServer.URL, Weight: 1},
		{URL: unhealthyServer.URL, Weight: 1},
	}

	pool, err := backend.NewBackendPool(cfg)
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}

	hcCfg := config.HealthCheckConfig{
		Interval:    100 * time.Millisecond,
		Timeout:     50 * time.Millisecond,
		MaxFailures: 1,
	}

	checker := NewHealthChecker(pool, hcCfg)

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	checker.CheckAll(ctx)
	time.Sleep(100 * time.Millisecond)

	backends := pool.GetBackends()
	if !backends[0].IsHealthy() {
		t.Errorf("expected first backend (%s) to be HEALTHY", healthyServer.URL)
	}
	if backends[1].IsHealthy() {
		t.Errorf("expected second backend (%s) to be UNHEALTHY", unhealthyServer.URL)
	}
}
