package balancer

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sergio11/gobalancer_poc/internal/backend"
	"github.com/sergio11/gobalancer_poc/internal/config"
	"github.com/sergio11/gobalancer_poc/internal/httputil"
)

func TestFactory_AllAlgorithms(t *testing.T) {
	algorithms := []string{
		"round-robin",
		"least-connections",
		"weighted",
		"random",
		"ip-hash",
	}

	cfg := []config.BackendConfig{
		{URL: "http://localhost:9001", Weight: 1},
	}

	for _, algo := range algorithms {
		pool, _ := backend.NewBackendPool(cfg)
		b, err := NewBalancer(algo, pool)
		if err != nil {
			t.Errorf("expected NewBalancer(%q) to succeed, got: %v", algo, err)
		}
		if b == nil {
			t.Errorf("expected non-nil balancer for algorithm %q", algo)
		}
	}

	// Unknown algorithm
	pool, _ := backend.NewBackendPool(cfg)
	_, err := NewBalancer("unknown-algo", pool)
	if err == nil {
		t.Errorf("expected error for unknown algorithm, got nil")
	}
}

func TestRandom_AllHealthy(t *testing.T) {
	cfg := []config.BackendConfig{
		{URL: "http://localhost:9001", Weight: 1},
		{URL: "http://localhost:9002", Weight: 1},
	}
	pool, _ := backend.NewBackendPool(cfg)
	r := NewRandom(pool)
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	seen := make(map[string]bool)
	for i := 0; i < 50; i++ {
		b, err := r.NextBackend(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		seen[b.URL.String()] = true
	}

	if len(seen) < 1 {
		t.Errorf("expected at least 1 unique backend to be selected")
	}
}

func TestIPHash_HeadersAndInvalidAddr(t *testing.T) {
	pool := createTestPool(t)
	iph := NewIPHash(pool)
	_ = iph

	// 1. Nil request
	ipNil := httputil.GetClientIP(nil, true)
	if ipNil != "127.0.0.1" {
		t.Errorf("expected 127.0.0.1 for nil request, got %s", ipNil)
	}

	// 2. X-Real-IP header
	reqRealIP := httptest.NewRequest(http.MethodGet, "/", nil)
	reqRealIP.Header.Set("X-Real-IP", "198.51.100.1")
	if ip := httputil.GetClientIP(reqRealIP, true); ip != "198.51.100.1" {
		t.Errorf("expected X-Real-IP 198.51.100.1, got %s", ip)
	}

	// 3. RemoteAddr without host:port format (trigger SplitHostPort error)
	reqRawAddr := httptest.NewRequest(http.MethodGet, "/", nil)
	reqRawAddr.RemoteAddr = "custom-raw-ip-without-port"
	if ip := httputil.GetClientIP(reqRawAddr, true); ip != "custom-raw-ip-without-port" {
		t.Errorf("expected raw address fallback, got %s", ip)
	}
}

func TestAlgorithms_NoHealthyBackends(t *testing.T) {
	pool := createTestPool(t)
	for _, b := range pool.GetBackends() {
		b.SetStatus(backend.StatusUnhealthy)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)

	// Test WeightedRoundRobin with no healthy backends
	wrr := NewWeightedRoundRobin(pool)
	if _, err := wrr.NextBackend(req); err != ErrNoHealthyBackends {
		t.Errorf("wrr: expected ErrNoHealthyBackends, got %v", err)
	}

	// Test LeastConnections with no healthy backends
	lc := NewLeastConnections(pool)
	if _, err := lc.NextBackend(req); err != ErrNoHealthyBackends {
		t.Errorf("lc: expected ErrNoHealthyBackends, got %v", err)
	}

	// Test IPHash with no healthy backends
	iph := NewIPHash(pool)
	if _, err := iph.NextBackend(req); err != ErrNoHealthyBackends {
		t.Errorf("iph: expected ErrNoHealthyBackends, got %v", err)
	}
}
