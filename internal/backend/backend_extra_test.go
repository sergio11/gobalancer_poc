package backend

import (
	"testing"
	"time"

	"gobalancer/internal/config"
)

func TestNewBackend_InvalidURL(t *testing.T) {
	_, err := NewBackend("bad", "://invalid-url", 1)
	if err == nil {
		t.Errorf("expected error for invalid URL, got nil")
	}
}

func TestNewBackend_DefaultWeight(t *testing.T) {
	b, err := NewBackend("b1", "http://localhost:9001", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b.Weight != 1 {
		t.Errorf("expected default weight 1 for weight<=0, got %d", b.Weight)
	}
}

func TestBackend_SetGetStatus(t *testing.T) {
	b, _ := NewBackend("b1", "http://localhost:9001", 1)
	b.SetStatus(StatusUnhealthy)
	if b.GetStatus() != StatusUnhealthy {
		t.Errorf("expected UNHEALTHY, got %s", b.GetStatus())
	}
	b.SetStatus(StatusHealthy)
	if b.GetStatus() != StatusHealthy {
		t.Errorf("expected HEALTHY, got %s", b.GetStatus())
	}
}

func TestBackend_RecordSuccessResetsFailures(t *testing.T) {
	b, _ := NewBackend("b1", "http://localhost:9001", 1)
	b.RecordFailure(5)
	b.RecordFailure(5)
	if b.Failures.Load() != 2 {
		t.Errorf("expected 2 failures, got %d", b.Failures.Load())
	}
	b.RecordSuccess(10 * time.Millisecond)
	if b.Failures.Load() != 0 {
		t.Errorf("expected failures to reset to 0 after success, got %d", b.Failures.Load())
	}
}

func TestBackendPool_InvalidURL(t *testing.T) {
	cfg := []config.BackendConfig{
		{URL: "://bad-url", Weight: 1},
	}
	_, err := NewBackendPool(cfg)
	if err == nil {
		t.Errorf("expected error for invalid backend URL in pool, got nil")
	}
}

func TestBackendPool_GetBackendByURL(t *testing.T) {
	cfg := []config.BackendConfig{
		{URL: "http://localhost:9001", Weight: 1},
		{URL: "http://localhost:9002", Weight: 2},
	}
	pool, _ := NewBackendPool(cfg)

	found := pool.GetBackendByURL("http://localhost:9001")
	if found == nil {
		t.Errorf("expected to find backend by URL")
	}

	notFound := pool.GetBackendByURL("http://localhost:9999")
	if notFound != nil {
		t.Errorf("expected nil for non-existent backend URL")
	}
}
