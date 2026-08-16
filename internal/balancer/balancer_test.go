package balancer

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sergio11/gobalancer_poc/internal/backend"
	"github.com/sergio11/gobalancer_poc/internal/config"
)

func createTestPool(t *testing.T) *backend.BackendPool {
	cfg := []config.BackendConfig{
		{URL: "http://localhost:9001", Weight: 5},
		{URL: "http://localhost:9002", Weight: 3},
		{URL: "http://localhost:9003", Weight: 1},
	}
	pool, err := backend.NewBackendPool(cfg)
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	return pool
}

func TestRoundRobin(t *testing.T) {
	pool := createTestPool(t)
	rr := NewRoundRobin(pool)
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	b1, _ := rr.NextBackend(req)
	b2, _ := rr.NextBackend(req)
	b3, _ := rr.NextBackend(req)
	b4, _ := rr.NextBackend(req)

	if b1.URL.String() != "http://localhost:9001" {
		t.Errorf("expected b1 to be http://localhost:9001, got %s", b1.URL.String())
	}
	if b2.URL.String() != "http://localhost:9002" {
		t.Errorf("expected b2 to be http://localhost:9002, got %s", b2.URL.String())
	}
	if b3.URL.String() != "http://localhost:9003" {
		t.Errorf("expected b3 to be http://localhost:9003, got %s", b3.URL.String())
	}
	if b4.URL.String() != "http://localhost:9001" {
		t.Errorf("expected b4 to wrap around to http://localhost:9001, got %s", b4.URL.String())
	}
}

func TestLeastConnections(t *testing.T) {
	pool := createTestPool(t)
	lc := NewLeastConnections(pool)
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	backends := pool.GetBackends()
	backends[0].IncConnections() // 9001 has 1
	backends[0].IncConnections() // 9001 has 2
	backends[1].IncConnections() // 9002 has 1
	// 9003 has 0

	selected, err := lc.NextBackend(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if selected.URL.String() != "http://localhost:9003" {
		t.Errorf("expected least conn backend http://localhost:9003, got %s", selected.URL.String())
	}
}

func TestWeightedRoundRobin(t *testing.T) {
	pool := createTestPool(t)
	wrr := NewWeightedRoundRobin(pool)
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	counts := make(map[string]int)
	totalRequests := 9 // weights 5+3+1 = 9

	for i := 0; i < totalRequests; i++ {
		b, err := wrr.NextBackend(req)
		if err != nil {
			t.Fatalf("unexpected error at request %d: %v", i, err)
		}
		counts[b.URL.String()]++
	}

	if counts["http://localhost:9001"] != 5 {
		t.Errorf("expected 5 requests for 9001, got %d", counts["http://localhost:9001"])
	}
	if counts["http://localhost:9002"] != 3 {
		t.Errorf("expected 3 requests for 9002, got %d", counts["http://localhost:9002"])
	}
	if counts["http://localhost:9003"] != 1 {
		t.Errorf("expected 1 request for 9003, got %d", counts["http://localhost:9003"])
	}
}

func TestIPHash_Consistency(t *testing.T) {
	pool := createTestPool(t)
	iph := NewIPHash(pool)

	req1 := httptest.NewRequest(http.MethodGet, "/", nil)
	req1.RemoteAddr = "192.168.1.50:12345"

	b1, err := iph.NextBackend(req1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for i := 0; i < 5; i++ {
		bNext, _ := iph.NextBackend(req1)
		if bNext.URL.String() != b1.URL.String() {
			t.Errorf("expected consistent backend %s for same IP, got %s", b1.URL.String(), bNext.URL.String())
		}
	}
}

func TestWeightedRoundRobin_ZeroWeight(t *testing.T) {
	pool := createTestPool(t)
	pool.GetBackends()[0].Weight = 0
	wrr := NewWeightedRoundRobin(pool)
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	b, err := wrr.NextBackend(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b == nil {
		t.Fatal("expected a backend, got nil")
	}
}

func TestNoHealthyBackends(t *testing.T) {
	pool := createTestPool(t)
	for _, b := range pool.GetBackends() {
		b.SetStatus(backend.StatusUnhealthy)
	}

	rr := NewRoundRobin(pool)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	_, err := rr.NextBackend(req)
	if err != ErrNoHealthyBackends {
		t.Errorf("expected ErrNoHealthyBackends, got %v", err)
	}
}
