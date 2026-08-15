package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestLoggerMiddleware_WriteHeader(t *testing.T) {
	handler := Logger(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	req := httptest.NewRequest(http.MethodGet, "/missing", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestLoggerMiddleware_Write(t *testing.T) {
	handler := Logger(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("hello world"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Body.String() != "hello world" {
		t.Errorf("expected 'hello world', got %s", rr.Body.String())
	}
}

func TestGetRequestID_NoContext(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	id := GetRequestID(req.Context())
	if id != "" {
		t.Errorf("expected empty string for missing request ID, got %q", id)
	}
}

func TestMiddlewareChain_Empty(t *testing.T) {
	called := false
	base := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	handler := Chain(base)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if !called {
		t.Errorf("expected handler to be called with empty Chain")
	}
}

func TestRateLimiter_Cleanup(t *testing.T) {
	rl := NewRateLimiter(10, 20, true, false)

	// Add a fresh client
	rl.mu.Lock()
	rl.clients["recent-ip"] = &clientBucket{
		tokens:     10,
		lastUpdate: time.Now(),
	}
	// Add an old client (expired)
	rl.clients["old-ip"] = &clientBucket{
		tokens:     0,
		lastUpdate: time.Now().Add(-11 * time.Minute),
	}
	rl.mu.Unlock()

	rl.cleanup()

	rl.mu.Lock()
	defer rl.mu.Unlock()

	if _, exists := rl.clients["old-ip"]; exists {
		t.Errorf("expected old-ip to be cleaned up")
	}
	if _, exists := rl.clients["recent-ip"]; !exists {
		t.Errorf("expected recent-ip to still be present")
	}
}

