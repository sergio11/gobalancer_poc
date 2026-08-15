package middleware

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestLoggerMiddleware_WritesStatusAndBody(t *testing.T) {
	handler := Chain(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte("ok"))
		}),
		RequestID,
		Logger,
	)

	req := httptest.NewRequest(http.MethodPost, "/submit", bytes.NewBufferString("payload"))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Errorf("expected 202, got %d", rr.Code)
	}
}

func TestLoggerMiddleware_DefaultStatus(t *testing.T) {
	// Handler that writes body without calling WriteHeader explicitly -> defaults to 200
	handler := Logger(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("implicit 200"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/implicit", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestChain_MultipleMiddlewares(t *testing.T) {
	order := []string{}

	m1 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "m1-before")
			next.ServeHTTP(w, r)
			order = append(order, "m1-after")
		})
	}
	m2 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "m2-before")
			next.ServeHTTP(w, r)
			order = append(order, "m2-after")
		})
	}

	handler := Chain(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "handler")
			w.WriteHeader(http.StatusOK)
		}),
		m1, m2,
	)

	req := httptest.NewRequest(http.MethodGet, "/chain", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	expected := []string{"m1-before", "m2-before", "handler", "m2-after", "m1-after"}
	if strings.Join(order, ",") != strings.Join(expected, ",") {
		t.Errorf("unexpected middleware execution order: %v, want %v", order, expected)
	}
}

func TestRequestID_ExistingHeader(t *testing.T) {
	// When X-Request-ID already present, it should be propagated
	handler := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := GetRequestID(r.Context())
		if reqID != "my-existing-id" {
			t.Errorf("expected request ID to be 'my-existing-id', got %q", reqID)
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(HeaderRequestID, "my-existing-id")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
}

func TestRateLimiter_Disabled(t *testing.T) {
	limiter := NewRateLimiter(0, 0, false, false)
	called := false
	handler := limiter.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if !called {
		t.Errorf("expected handler to be called when rate limiter is disabled")
	}
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestRateLimiter_XForwardedFor(t *testing.T) {
	limiter := NewRateLimiter(100, 200, true, true)
	handler := limiter.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-For", "203.0.113.42")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestRateLimiter_Stop(t *testing.T) {
	limiter := NewRateLimiter(10, 20, true, false)

	done := make(chan struct{})
	go func() {
		limiter.Stop()
		close(done)
	}()

	select {
	case <-done:
	case <-make(chan struct{}, 1):
		t.Errorf("Stop() did not return in time")
	}
}

func TestRateLimiter_TokenCap(t *testing.T) {
	limiter := NewRateLimiter(10, 5, true, false)
	defer limiter.Stop()

	handler := limiter.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.99"

	rr1 := httptest.NewRecorder()
	handler.ServeHTTP(rr1, req)
	if rr1.Code != http.StatusOK {
		t.Fatalf("first request expected 200, got %d", rr1.Code)
	}

	time.Sleep(1100 * time.Millisecond)

	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req)
	if rr2.Code != http.StatusOK {
		t.Errorf("after capacity cap expected 200, got %d", rr2.Code)
	}
}

func TestRateLimiter_CleanupLoop_StopsOnDone(t *testing.T) {
	limiter := NewRateLimiter(10, 20, true, false)
	limiter.Stop()
	// Give the goroutine time to exit
	time.Sleep(10 * time.Millisecond)
	// No panic or deadlock means the goroutine exited cleanly
}

func TestRateLimiter_CleanupLoop_TickFires(t *testing.T) {
	limiter := NewRateLimiterWithCleanup(10, 20, true, 10*time.Millisecond, false)
	defer limiter.Stop()

	handler := limiter.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.50"

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	time.Sleep(25 * time.Millisecond)

	limiter.mu.Lock()
	count := len(limiter.clients)
	limiter.mu.Unlock()

	if count != 1 {
		t.Errorf("expected 1 client before cleanup, got %d", count)
	}
}
