package backend

import (
	"testing"
	"time"

	"gobalancer/internal/config"
)

func TestBackend_ConnectionsAndStatus(t *testing.T) {
	b, err := NewBackend("b1", "http://localhost:8081", 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !b.IsHealthy() {
		t.Errorf("expected initial status HEALTHY")
	}

	b.IncConnections()
	b.IncConnections()
	if b.GetConnections() != 2 {
		t.Errorf("expected 2 active connections, got %d", b.GetConnections())
	}

	b.DecConnections()
	if b.GetConnections() != 1 {
		t.Errorf("expected 1 active connection, got %d", b.GetConnections())
	}

	b.RecordFailure(2)
	if !b.IsHealthy() {
		t.Errorf("expected healthy after 1 failure when threshold is 2")
	}

	b.RecordFailure(2)
	if b.IsHealthy() {
		t.Errorf("expected UNHEALTHY after 2 failures")
	}

	b.RecordSuccess(10 * time.Millisecond)
	if !b.IsHealthy() {
		t.Errorf("expected HEALTHY after success")
	}
}

func TestBackendPool_FilterHealthy(t *testing.T) {
	cfg := []config.BackendConfig{
		{URL: "http://localhost:8081", Weight: 1},
		{URL: "http://localhost:8082", Weight: 2},
	}

	pool, err := NewBackendPool(cfg)
	if err != nil {
		t.Fatalf("unexpected error creating pool: %v", err)
	}

	backends := pool.GetBackends()
	if len(backends) != 2 {
		t.Fatalf("expected 2 backends in pool, got %d", len(backends))
	}

	backends[0].SetStatus(StatusUnhealthy)
	healthy := pool.GetHealthyBackends()
	if len(healthy) != 1 {
		t.Fatalf("expected 1 healthy backend, got %d", len(healthy))
	}
	if healthy[0].URL.String() != "http://localhost:8082" {
		t.Errorf("expected healthy backend to be http://localhost:8082, got %s", healthy[0].URL.String())
	}
}
