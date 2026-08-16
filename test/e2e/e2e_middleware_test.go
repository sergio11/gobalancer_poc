package e2e

import (
	"net/http"
	"strings"
	"testing"

	"github.com/sergio11/gobalancer_poc/internal/config"
)

// TestE2E_RequestIDHeader verifies the RequestID middleware injects (or echoes)
// the X-Request-ID response header through the live pipeline.
func TestE2E_RequestIDHeader(t *testing.T) {
	b1 := newMockBackend(t, "1", nil)

	env := setupTestLB(t, lbOptions{
		algorithm: "round-robin",
		backends:  []*mockBackend{b1},
	})

	// Generated request ID.
	resp, _ := env.get("/test", "")
	if got := resp.Header.Get("X-Request-ID"); got == "" {
		t.Errorf("expected generated X-Request-ID header")
	}

	// Echoed request ID.
	req, err := http.NewRequest(http.MethodGet, env.srv.URL+"/test", nil)
	if err != nil {
		t.Fatalf("failed to build request: %v", err)
	}
	req.Header.Set("X-Request-ID", "echo-me-123")
	hr, err := env.client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	hr.Body.Close()
	if got := hr.Header.Get("X-Request-ID"); got != "echo-me-123" {
		t.Errorf("expected X-Request-ID to be echoed, got %q", got)
	}
}

// TestE2E_RateLimiter_Returns429 verifies the token bucket rate limiter
// rejects requests beyond capacity with 429 Too Many Requests.
func TestE2E_RateLimiter_Returns429(t *testing.T) {
	b1 := newMockBackend(t, "1", nil)

	env := setupTestLB(t, lbOptions{
		algorithm: "round-robin",
		backends:  []*mockBackend{b1},
		rateLimit: config.RateLimitConfig{
			Rate:     0,
			Capacity: 2,
			Enabled:  true,
		},
	})

	for i := 0; i < 2; i++ {
		resp, _ := env.get("/test", "")
		if resp.StatusCode != http.StatusOK {
			t.Errorf("request %d should pass the limiter, got %d", i+1, resp.StatusCode)
		}
	}

	resp, body := env.get("/test", "")
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("expected 429 on the third request, got %d", resp.StatusCode)
	}
	if !strings.Contains(body, "Too Many Requests") {
		t.Errorf("expected 429 body, got %q", body)
	}
	if retry := resp.Header.Get("Retry-After"); retry == "" {
		t.Errorf("expected Retry-After header on 429")
	}
}

// TestE2E_RecoveryMiddleware_Panic verifies panics in handlers are recovered
// and returned as 500 Internal Server Error.
func TestE2E_RecoveryMiddleware_Panic(t *testing.T) {
	b1 := newMockBackend(t, "1", nil)

	env := setupTestLB(t, lbOptions{
		algorithm: "round-robin",
		backends:  []*mockBackend{b1},
		extraRoutes: func(mux *http.ServeMux) {
			mux.HandleFunc("GET /panic", func(w http.ResponseWriter, r *http.Request) {
				panic("boom")
			})
		},
	})

	resp, body := env.get("/panic", "")
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected 500 after panic, got %d", resp.StatusCode)
	}
	if !strings.Contains(body, "Internal Server Error") {
		t.Errorf("unexpected panic body: %q", body)
	}
}

// TestE2E_XForwardedHeaders verifies the proxy director injects the
// X-Forwarded-* headers before forwarding to the backend.
func TestE2E_XForwardedHeaders(t *testing.T) {
	b1 := newMockBackend(t, "1", nil)

	env := setupTestLB(t, lbOptions{
		algorithm: "round-robin",
		backends:  []*mockBackend{b1},
	})

	env.get("/test", "203.0.113.7")

	h := b1.lastHeaders()
	if h.Get("X-Forwarded-For") == "" {
		t.Errorf("expected X-Forwarded-For header, got none")
	}
	if got := h.Get("X-Forwarded-Host"); got == "" {
		t.Errorf("expected X-Forwarded-Host header, got none")
	}
	if got := h.Get("X-Forwarded-Proto"); got != "http" {
		t.Errorf("expected X-Forwarded-Proto http, got %q", got)
	}
}

// TestE2E_BackendErrorStatus_Passthrough verifies backend HTTP error statuses
// are forwarded unchanged (not converted to 502 Bad Gateway).
func TestE2E_BackendErrorStatus_Passthrough(t *testing.T) {
	b1 := newMockBackend(t, "1", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/not-found":
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte("nope"))
		case "/server-error":
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("oops"))
		default:
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("ok"))
		}
	})

	env := setupTestLB(t, lbOptions{
		algorithm: "round-robin",
		backends:  []*mockBackend{b1},
	})

	resp, body := env.get("/not-found", "")
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 passthrough, got %d", resp.StatusCode)
	}
	if body != "nope" {
		t.Errorf("expected backend body to pass through, got %q", body)
	}

	resp, body = env.get("/server-error", "")
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected 500 passthrough, got %d", resp.StatusCode)
	}
	if body != "oops" {
		t.Errorf("expected backend body to pass through, got %q", body)
	}
}
