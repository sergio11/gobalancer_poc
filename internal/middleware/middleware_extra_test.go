package middleware

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
	limiter := NewRateLimiter(0, 0, false)
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
	limiter := NewRateLimiter(100, 200, true)
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
