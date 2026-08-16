package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sergio11/gobalancer_poc/internal/api"
	"github.com/sergio11/gobalancer_poc/internal/backend"
	"github.com/sergio11/gobalancer_poc/internal/balancer"
	"github.com/sergio11/gobalancer_poc/internal/config"
	"github.com/sergio11/gobalancer_poc/internal/health"
	"github.com/sergio11/gobalancer_poc/internal/logger"
	"github.com/sergio11/gobalancer_poc/internal/metrics"
	"github.com/sergio11/gobalancer_poc/internal/middleware"
	"github.com/sergio11/gobalancer_poc/internal/proxy"
)

func TestMain(m *testing.M) {
	logger.Init("error")
	os.Exit(m.Run())
}

// mockBackend is a configurable HTTP backend used across E2E scenarios.
type mockBackend struct {
	id     string
	server *httptest.Server
	hits   atomic.Uint64
	state  atomic.Int32 // 1 = healthy health endpoint, 0 = failing

	mu      sync.Mutex
	headers http.Header
	custom  func(w http.ResponseWriter, r *http.Request)
}

func newMockBackend(t *testing.T, id string, custom func(w http.ResponseWriter, r *http.Request)) *mockBackend {
	t.Helper()
	mb := &mockBackend{id: id, custom: custom}
	mb.state.Store(1)
	mb.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			if mb.state.Load() == 1 {
				w.WriteHeader(http.StatusOK)
			} else {
				w.WriteHeader(http.StatusInternalServerError)
			}
			return
		}
		mb.mu.Lock()
		mb.headers = r.Header.Clone()
		mb.mu.Unlock()
		mb.hits.Add(1)
		if mb.custom != nil {
			mb.custom(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, "backend-%s", mb.id)
	}))
	t.Cleanup(mb.server.Close)
	return mb
}

func (mb *mockBackend) url() string { return mb.server.URL }

func (mb *mockBackend) setHealthy(healthy bool) {
	if healthy {
		mb.state.Store(1)
	} else {
		mb.state.Store(0)
	}
}

func (mb *mockBackend) hitCount() uint64 { return mb.hits.Load() }

func (mb *mockBackend) lastHeaders() http.Header {
	mb.mu.Lock()
	defer mb.mu.Unlock()
	return mb.headers
}

// safeBalancer mirrors the concurrency guard used in cmd/gobalancer/main.go.
type safeBalancer struct {
	mu    sync.RWMutex
	inner balancer.Balancer
}

func (s *safeBalancer) NextBackend(req *http.Request) (*backend.Backend, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.inner.NextBackend(req)
}

// lbOptions configures the full load balancer pipeline for a test.
type lbOptions struct {
	algorithm  string
	backends   []*mockBackend
	weights    []int
	healthCfg  config.HealthCheckConfig
	rateLimit  config.RateLimitConfig
	maxFailures int
	runChecker bool
	configPath string
	extraRoutes func(mux *http.ServeMux)
}

// lbEnv is the assembled load balancer under test.
type lbEnv struct {
	t        *testing.T
	srv      *httptest.Server
	client   *http.Client
	pool     *backend.BackendPool
	backends []*mockBackend
	cancel   context.CancelFunc
}

func setupTestLB(t *testing.T, opts lbOptions) *lbEnv {
	t.Helper()
	if len(opts.backends) == 0 {
		t.Fatal("setupTestLB requires at least one backend")
	}
	if opts.algorithm == "" {
		opts.algorithm = "round-robin"
	}
	if opts.maxFailures == 0 {
		opts.maxFailures = 1
	}

	backendCfgs := make([]config.BackendConfig, 0, len(opts.backends))
	for i, mb := range opts.backends {
		w := 1
		if opts.weights != nil && i < len(opts.weights) {
			w = opts.weights[i]
		}
		backendCfgs = append(backendCfgs, config.BackendConfig{URL: mb.url(), Weight: w})
	}

	pool, err := backend.NewBackendPool(backendCfgs)
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}

	hcCfg := opts.healthCfg
	if hcCfg.Interval <= 0 {
		hcCfg.Interval = 15 * time.Millisecond
	}
	if hcCfg.Timeout <= 0 {
		hcCfg.Timeout = 50 * time.Millisecond
	}
	if hcCfg.MaxFailures <= 0 {
		hcCfg.MaxFailures = 1
	}

	env := &lbEnv{t: t, pool: pool, backends: opts.backends}

	if opts.runChecker {
		checker := health.NewHealthChecker(pool, hcCfg)
		ctx, cancel := context.WithCancel(context.Background())
		env.cancel = cancel
		go checker.Start(ctx)
		t.Cleanup(func() {
			cancel()
			checker.Stop()
		})
	}

	balEngine, err := balancer.NewBalancer(opts.algorithm, pool)
	if err != nil {
		t.Fatalf("failed to create balancer: %v", err)
	}
	sb := &safeBalancer{inner: balEngine}
	metricsSvc := metrics.NewMetricsService(pool)
	revProxy := proxy.NewReverseProxy(sb, metricsSvc, opts.maxFailures)

	onReload := func(newCfg *config.Config) error {
		newPool, err := backend.NewBackendPool(newCfg.Backends)
		if err != nil {
			return err
		}
		newBal, err := balancer.NewBalancer(newCfg.LoadBalancer.Algorithm, newPool)
		if err != nil {
			return err
		}
		pool.ReplaceBackends(newPool.GetBackends())
		sb.mu.Lock()
		sb.inner = newBal
		sb.mu.Unlock()
		return nil
	}

	adminAPI := api.NewAdminAPI(pool, opts.configPath, onReload, "")

	mux := http.NewServeMux()
	adminAPI.RegisterRoutes(mux)
	mux.HandleFunc("GET /metrics", metricsSvc.Handler())
	if opts.extraRoutes != nil {
		opts.extraRoutes(mux)
	}
	mux.Handle("/", revProxy)

	limiter := middleware.NewRateLimiter(opts.rateLimit.Rate, opts.rateLimit.Capacity, opts.rateLimit.Enabled, false)
	t.Cleanup(limiter.Stop)

	handler := middleware.Chain(
		mux,
		middleware.Recovery,
		middleware.RequestID,
		middleware.Logger,
		limiter.Middleware,
	)

	srv := httptest.NewServer(handler)
	env.srv = srv
	env.client = srv.Client()
	t.Cleanup(srv.Close)
	return env
}

func (env *lbEnv) get(path string, xff string) (*http.Response, string) {
	env.t.Helper()
	req, err := http.NewRequest(http.MethodGet, env.srv.URL+path, nil)
	if err != nil {
		env.t.Fatalf("failed to build request: %v", err)
	}
	if xff != "" {
		req.Header.Set("X-Forwarded-For", xff)
	}
	resp, err := env.client.Do(req)
	if err != nil {
		env.t.Fatalf("request %s failed: %v", path, err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp, string(body)
}

func (env *lbEnv) post(path string, body string) (*http.Response, string) {
	env.t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req, err := http.NewRequest(http.MethodPost, env.srv.URL+path, reader)
	if err != nil {
		env.t.Fatalf("failed to build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := env.client.Do(req)
	if err != nil {
		env.t.Fatalf("request %s failed: %v", path, err)
	}
	respBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp, string(respBody)
}

func fetchStats(t *testing.T, env *lbEnv) api.StatsDTO {
	t.Helper()
	resp, err := env.client.Get(env.srv.URL + "/api/stats")
	if err != nil {
		t.Fatalf("failed to fetch stats: %v", err)
	}
	defer resp.Body.Close()
	var stats api.StatsDTO
	if err := json.NewDecoder(resp.Body).Decode(&stats); err != nil {
		t.Fatalf("failed to decode stats: %v", err)
	}
	return stats
}

func waitForBackendStatus(t *testing.T, pool *backend.BackendPool, id string, want backend.BackendStatus) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		for _, b := range pool.GetBackends() {
			if b.ID == id && b.GetStatus() == want {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("backend %s did not reach status %s", id, want)
}

func waitForHealthyCount(t *testing.T, env *lbEnv, want int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if fetchStats(t, env).HealthyBackends == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("expected %d healthy backends, got %d", want, fetchStats(t, env).HealthyBackends)
}
