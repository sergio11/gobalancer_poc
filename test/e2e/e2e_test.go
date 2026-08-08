package e2e

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"gobalancer/internal/api"
	"gobalancer/internal/backend"
	"gobalancer/internal/balancer"
	"gobalancer/internal/config"
	"gobalancer/internal/health"
	"gobalancer/internal/metrics"
	"gobalancer/internal/proxy"
)

func TestE2E_LoadBalancerTrafficDistributionAndFailover(t *testing.T) {
	var count1, count2, count3 uint64

	// 1. Start 3 mock backend HTTP servers
	b1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		atomic.AddUint64(&count1, 1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("backend-1"))
	}))
	defer b1.Close()

	b2State := int32(1) // 1 = healthy, 0 = failing
	b2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			if atomic.LoadInt32(&b2State) == 1 {
				w.WriteHeader(http.StatusOK)
			} else {
				w.WriteHeader(http.StatusInternalServerError)
			}
			return
		}
		atomic.AddUint64(&count2, 1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("backend-2"))
	}))
	defer b2.Close()

	b3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		atomic.AddUint64(&count3, 1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("backend-3"))
	}))
	defer b3.Close()

	// 2. Configure GoBalancer BackendPool & Services
	backendCfgs := []config.BackendConfig{
		{URL: b1.URL, Weight: 1},
		{URL: b2.URL, Weight: 1},
		{URL: b3.URL, Weight: 1},
	}

	pool, err := backend.NewBackendPool(backendCfgs)
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}

	hcCfg := config.HealthCheckConfig{
		Interval:    50 * time.Millisecond,
		Timeout:     50 * time.Millisecond,
		MaxFailures: 1,
	}
	checker := health.NewHealthChecker(pool, hcCfg)

	balEngine, _ := balancer.NewBalancer("round-robin", pool)
	revProxy := proxy.NewReverseProxy(balEngine)
	metricsSvc := metrics.NewMetricsService(pool)
	adminAPI := api.NewAdminAPI(pool, "", nil)

	mux := http.NewServeMux()
	adminAPI.RegisterRoutes(mux)
	mux.HandleFunc("GET /metrics", metricsSvc.Handler())
	mux.Handle("/", revProxy)

	lbServer := httptest.NewServer(mux)
	defer lbServer.Close()

	// 3. Send 6 requests and verify Round Robin distribution (2 per backend)
	client := lbServer.Client()
	for i := 0; i < 6; i++ {
		resp, err := client.Get(lbServer.URL + "/test")
		if err != nil {
			t.Fatalf("request %d failed: %v", i, err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("request %d returned status %d", i, resp.StatusCode)
		}
		_ = body
	}

	if atomic.LoadUint64(&count1) != 2 || atomic.LoadUint64(&count2) != 2 || atomic.LoadUint64(&count3) != 2 {
		t.Errorf("expected even distribution (2 each), got b1=%d, b2=%d, b3=%d",
			atomic.LoadUint64(&count1), atomic.LoadUint64(&count2), atomic.LoadUint64(&count3))
	}

	// 4. Simulate Backend 2 Failure
	atomic.StoreInt32(&b2State, 0)
	checker.CheckAll(t.Context())
	time.Sleep(100 * time.Millisecond) // wait for status update

	// Send 4 more requests - should only go to b1 and b3
	c1Before := atomic.LoadUint64(&count1)
	c3Before := atomic.LoadUint64(&count3)

	for i := 0; i < 4; i++ {
		resp, err := client.Get(lbServer.URL + "/test")
		if err != nil {
			t.Fatalf("request after failure %d failed: %v", i, err)
		}
		resp.Body.Close()
	}

	if atomic.LoadUint64(&count2) != 2 {
		t.Errorf("unhealthy backend 2 should not receive requests, count remains 2, got %d", atomic.LoadUint64(&count2))
	}

	if (atomic.LoadUint64(&count1) - c1Before) != 2 || (atomic.LoadUint64(&count3) - c3Before) != 2 {
		t.Errorf("expected 2 requests each to b1 and b3 after b2 failed, got b1=+%d, b3=+%d",
			atomic.LoadUint64(&count1)-c1Before, atomic.LoadUint64(&count3)-c3Before)
	}

	// 5. Test Admin API /api/stats
	respStats, err := client.Get(lbServer.URL + "/api/stats")
	if err != nil {
		t.Fatalf("failed to fetch stats: %v", err)
	}
	defer respStats.Body.Close()

	var stats api.StatsDTO
	_ = json.NewDecoder(respStats.Body).Decode(&stats)

	if stats.TotalBackends != 3 {
		t.Errorf("expected 3 total backends in stats, got %d", stats.TotalBackends)
	}
	if stats.HealthyBackends != 2 {
		t.Errorf("expected 2 healthy backends in stats, got %d", stats.HealthyBackends)
	}
	if stats.UnhealthyCount != 1 {
		t.Errorf("expected 1 unhealthy backend in stats, got %d", stats.UnhealthyCount)
	}
}
