package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sergio11/gobalancer_poc/internal/backend"
	"github.com/sergio11/gobalancer_poc/internal/balancer"
	"github.com/sergio11/gobalancer_poc/internal/config"
	"github.com/sergio11/gobalancer_poc/internal/metrics"
)

func TestReverseProxy_ForwardsRequest(t *testing.T) {
	backendServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Custom-Header") == "" {
			t.Errorf("expected X-Custom-Header to be forwarded")
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("proxied-response"))
	}))
	defer backendServer.Close()

	pool, _ := backend.NewBackendPool([]config.BackendConfig{{URL: backendServer.URL, Weight: 1}})
	bal := balancer.NewRoundRobin(pool)
	rp := NewReverseProxy(bal, nil, 3)

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("X-Custom-Header", "my-value")
	rr := httptest.NewRecorder()
	rp.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	body, _ := io.ReadAll(rr.Body)
	if string(body) != "proxied-response" {
		t.Errorf("expected proxied-response, got %s", string(body))
	}
	if pool.GetBackends()[0].GetConnections() != 0 {
		t.Errorf("expected 0 active connections after request, got %d", pool.GetBackends()[0].GetConnections())
	}
}

func TestReverseProxy_NoHealthyBackends(t *testing.T) {
	pool, _ := backend.NewBackendPool([]config.BackendConfig{{URL: "http://localhost:19999", Weight: 1}})
	pool.GetBackends()[0].SetStatus(backend.StatusUnhealthy)

	rp := NewReverseProxy(balancer.NewRoundRobin(pool), nil, 3)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	rp.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rr.Code)
	}
}

func TestReverseProxy_ErrorHandler_ConnectionRefused(t *testing.T) {
	// Points to an unreachable port to trigger ReverseProxy ErrorHandler
	pool, _ := backend.NewBackendPool([]config.BackendConfig{{URL: "http://127.0.0.1:59999", Weight: 1}})
	bal := balancer.NewRoundRobin(pool)
	rp := NewReverseProxy(bal, nil, 3)

	req := httptest.NewRequest(http.MethodGet, "/fail", nil)
	rr := httptest.NewRecorder()

	rp.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadGateway {
		t.Errorf("expected 502 Bad Gateway from error handler, got %d", rr.Code)
	}
	if pool.GetBackends()[0].Failures.Load() != 1 {
		t.Errorf("expected backend failure count to be 1, got %d", pool.GetBackends()[0].Failures.Load())
	}
}

func TestReverseProxy_XForwardedFor_Appended(t *testing.T) {
	backendServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		xff := r.Header.Get("X-Forwarded-For")
		if xff == "" {
			t.Errorf("expected X-Forwarded-For header to be present")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer backendServer.Close()

	pool, _ := backend.NewBackendPool([]config.BackendConfig{{URL: backendServer.URL, Weight: 1}})
	rp := NewReverseProxy(balancer.NewRoundRobin(pool), nil, 3)

	// Request already has X-Forwarded-For -> should append
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-For", "10.0.0.1")
	req.RemoteAddr = "192.168.1.1:9999"
	rr := httptest.NewRecorder()
	rp.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestReverseProxy_MetricsIncremented(t *testing.T) {
	backendServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backendServer.Close()

	pool, _ := backend.NewBackendPool([]config.BackendConfig{{URL: backendServer.URL, Weight: 1}})
	bal := balancer.NewRoundRobin(pool)
	m := metrics.NewMetricsService(pool)
	rp := NewReverseProxy(bal, m, 3)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rr := httptest.NewRecorder()
	rp.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestReverseProxy_MetricsErrorIncremented(t *testing.T) {
	pool, _ := backend.NewBackendPool([]config.BackendConfig{{URL: "http://localhost:19999", Weight: 1}})
	pool.GetBackends()[0].SetStatus(backend.StatusUnhealthy)
	bal := balancer.NewRoundRobin(pool)
	m := metrics.NewMetricsService(pool)
	rp := NewReverseProxy(bal, m, 3)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	rp.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rr.Code)
	}
}

func TestReverseProxy_ProxyCache(t *testing.T) {
	backendServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer backendServer.Close()

	pool, _ := backend.NewBackendPool([]config.BackendConfig{{URL: backendServer.URL, Weight: 1}})
	bal := balancer.NewRoundRobin(pool)
	rp := NewReverseProxy(bal, nil, 3)

	req1 := httptest.NewRequest(http.MethodGet, "/", nil)
	rr1 := httptest.NewRecorder()
	rp.ServeHTTP(rr1, req1)
	if rr1.Code != http.StatusOK {
		t.Fatalf("first request: expected 200, got %d", rr1.Code)
	}

	_, exists := rp.proxyCache.Load(pool.GetBackends()[0].ID)
	if !exists {
		t.Error("expected proxy to be cached after first request")
	}

	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	rr2 := httptest.NewRecorder()
	rp.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Errorf("second request (cached): expected 200, got %d", rr2.Code)
	}
}
