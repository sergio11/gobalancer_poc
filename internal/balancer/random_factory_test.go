package balancer

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"gobalancer/internal/backend"
	"gobalancer/internal/config"
)

func TestRandom_ReturnsBackend(t *testing.T) {
	cfg := []config.BackendConfig{
		{URL: "http://localhost:9001", Weight: 1},
		{URL: "http://localhost:9002", Weight: 1},
	}
	pool, _ := backend.NewBackendPool(cfg)
	r := NewRandom(pool)
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	b, err := r.NextBackend(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b == nil {
		t.Error("expected non-nil backend from Random")
	}
}

func TestRandom_NoHealthy(t *testing.T) {
	cfg := []config.BackendConfig{
		{URL: "http://localhost:9001", Weight: 1},
	}
	pool, _ := backend.NewBackendPool(cfg)
	pool.GetBackends()[0].SetStatus(backend.StatusUnhealthy)

	r := NewRandom(pool)
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	_, err := r.NextBackend(req)
	if err != ErrNoHealthyBackends {
		t.Errorf("expected ErrNoHealthyBackends, got %v", err)
	}
}

func TestNewBalancer_AllAlgorithms(t *testing.T) {
	cfg := []config.BackendConfig{{URL: "http://localhost:9001", Weight: 1}}
	tests := []struct {
		algo    string
		wantErr bool
	}{
		{"round-robin", false},
		{"roundrobin", false},
		{"", false},
		{"least-connections", false},
		{"leastconnections", false},
		{"least_conn", false},
		{"weighted-round-robin", false},
		{"weighted", false},
		{"wrr", false},
		{"random", false},
		{"ip-hash", false},
		{"iphash", false},
		{"unknown", true},
	}

	for _, tt := range tests {
		pool, _ := backend.NewBackendPool(cfg)
		b, err := NewBalancer(tt.algo, pool)
		if tt.wantErr {
			if err == nil {
				t.Errorf("NewBalancer(%q): expected error, got nil", tt.algo)
			}
		} else {
			if err != nil {
				t.Errorf("NewBalancer(%q): unexpected error: %v", tt.algo, err)
			}
			if b == nil {
				t.Errorf("NewBalancer(%q): expected non-nil balancer", tt.algo)
			}
		}
	}
}
