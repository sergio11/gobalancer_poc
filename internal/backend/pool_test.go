package backend

import (
	"testing"

	"gobalancer/internal/config"
)

func TestBackendPool_AddBackend(t *testing.T) {
	pool := &BackendPool{}

	b, err := NewBackend("manual-1", "http://localhost:9099", 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	pool.AddBackend(b)

	backends := pool.GetBackends()
	if len(backends) != 1 {
		t.Errorf("expected 1 backend after AddBackend, got %d", len(backends))
	}
	if backends[0].ID != "manual-1" {
		t.Errorf("expected backend ID 'manual-1', got %s", backends[0].ID)
	}
}

func TestBackendPool_GetBackendByURL_Found(t *testing.T) {
	cfg := []config.BackendConfig{
		{URL: "http://localhost:9001", Weight: 1},
		{URL: "http://localhost:9002", Weight: 2},
	}
	pool, _ := NewBackendPool(cfg)

	found := pool.GetBackendByURL("http://localhost:9001")
	if found == nil {
		t.Errorf("expected to find backend by URL http://localhost:9001")
	}
	if found.URL.String() != "http://localhost:9001" {
		t.Errorf("wrong backend returned: %s", found.URL.String())
	}

	notFound := pool.GetBackendByURL("http://localhost:9999")
	if notFound != nil {
		t.Errorf("expected nil for non-existent backend URL")
	}
}

func TestBackendPool_ReplaceBackends(t *testing.T) {
	pool, _ := NewBackendPool([]config.BackendConfig{
		{URL: "http://localhost:9001", Weight: 1},
		{URL: "http://localhost:9002", Weight: 2},
	})

	replacement, err := NewBackend("new-1", "http://localhost:9101", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	replacement2, err := NewBackend("new-2", "http://localhost:9102", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	replacement3, err := NewBackend("new-3", "http://localhost:9103", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	pool.ReplaceBackends([]*Backend{replacement, replacement2, replacement3})

	backends := pool.GetBackends()
	if len(backends) != 3 {
		t.Fatalf("expected 3 backends after ReplaceBackends, got %d", len(backends))
	}
	if backends[0].ID != "new-1" || backends[2].ID != "new-3" {
		t.Errorf("unexpected backends after replace: %v", backends)
	}
	if pool.GetBackendByURL("http://localhost:9001") != nil {
		t.Errorf("old backends should be removed after ReplaceBackends")
	}
}
