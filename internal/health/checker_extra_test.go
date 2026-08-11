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

func TestHealthChecker_RecoveryAfterFailure(t *testing.T) {
	healthy := true

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if healthy {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("OK"))
		} else {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("ERROR"))
		}
	}))
	defer srv.Close()

	pool, _ := backend.NewBackendPool([]config.BackendConfig{
		{URL: srv.URL, Weight: 1},
	})

	cfg := config.HealthCheckConfig{
		Interval:    10 * time.Millisecond,
		Timeout:     50 * time.Millisecond,
		MaxFailures: 1,
	}

	checker := NewHealthChecker(pool, cfg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start healthy
	checker.CheckAll(ctx)
	time.Sleep(30 * time.Millisecond)
	if !pool.GetBackends()[0].IsHealthy() {
		t.Errorf("expected backend to start HEALTHY")
	}

	// Go unhealthy (returns 500 + body)
	healthy = false
	checker.CheckAll(ctx)
	time.Sleep(30 * time.Millisecond)
	if pool.GetBackends()[0].IsHealthy() {
		t.Errorf("expected backend to be UNHEALTHY after failure")
	}

	// Recover
	healthy = true
	checker.CheckAll(ctx)
	time.Sleep(30 * time.Millisecond)
	if !pool.GetBackends()[0].IsHealthy() {
		t.Errorf("expected backend to recover to HEALTHY")
	}
}

func TestHealthChecker_InvalidRequest(t *testing.T) {
	b, _ := backend.NewBackend("b-bad", "http://127.0.0.1:9090", 1)
	b.URL.Scheme = "http://invalid-scheme-with-space "

	pool, _ := backend.NewBackendPool([]config.BackendConfig{
		{URL: "http://localhost:9090", Weight: 1},
	})
	pool.GetBackends()[0] = b

	cfg := config.HealthCheckConfig{
		Interval:    20 * time.Millisecond,
		Timeout:     50 * time.Millisecond,
		MaxFailures: 1,
	}
	checker := NewHealthChecker(pool, cfg)
	checker.CheckBackend(context.Background(), b)

	if b.IsHealthy() {
		t.Errorf("expected unhealthy for invalid request URL")
	}
}

func TestHealthChecker_Start_CtxCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	pool, _ := backend.NewBackendPool([]config.BackendConfig{
		{URL: srv.URL, Weight: 1},
	})
	cfg := config.HealthCheckConfig{
		Interval:    50 * time.Millisecond,
		Timeout:     50 * time.Millisecond,
		MaxFailures: 3,
	}
	checker := NewHealthChecker(pool, cfg)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		checker.Start(ctx)
		close(done)
	}()

	time.Sleep(30 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return after context cancellation")
	}
}

func TestHealthChecker_StartTickerAndStop(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	pool, _ := backend.NewBackendPool([]config.BackendConfig{
		{URL: srv.URL, Weight: 1},
	})
	cfg := config.HealthCheckConfig{
		Interval:    10 * time.Millisecond,
		Timeout:     50 * time.Millisecond,
		MaxFailures: 3,
	}
	checker := NewHealthChecker(pool, cfg)
	ctx := context.Background()

	// Run in background and wait for ticker.C to fire multiple times
	go checker.Start(ctx)
	time.Sleep(35 * time.Millisecond)
	checker.Stop()
	time.Sleep(20 * time.Millisecond)
}
