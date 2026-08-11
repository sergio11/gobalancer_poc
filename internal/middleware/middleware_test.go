package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRequestIDMiddleware_Generated(t *testing.T) {
	handler := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := GetRequestID(r.Context())
		if reqID == "" {
			t.Errorf("expected generated request ID in context")
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Header().Get(HeaderRequestID) == "" {
		t.Errorf("expected X-Request-ID header in response")
	}
}

func TestGenerateID(t *testing.T) {
	id1 := generateID()
	id2 := generateID()

	if id1 == "" || id2 == "" {
		t.Errorf("generateID returned empty string")
	}
	if id1 == id2 {
		t.Errorf("expected unique IDs, got identical: %s", id1)
	}
}

func TestRecoveryMiddleware(t *testing.T) {
	panicHandler := Recovery(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("test panic")
	}))

	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	rr := httptest.NewRecorder()

	panicHandler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 Internal Server Error, got %d", rr.Code)
	}
}

func TestRateLimiterMiddleware(t *testing.T) {
	limiter := NewRateLimiter(1, 2, true) // rate 1/s, max capacity 2

	handler := limiter.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/rate", nil)
	req.RemoteAddr = "10.0.0.1"

	// 1st request (consumes token 1) -> 200
	rr1 := httptest.NewRecorder()
	handler.ServeHTTP(rr1, req)
	if rr1.Code != http.StatusOK {
		t.Errorf("req 1 expected 200, got %d", rr1.Code)
	}

	// 2nd request (consumes token 2) -> 200
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req)
	if rr2.Code != http.StatusOK {
		t.Errorf("req 2 expected 200, got %d", rr2.Code)
	}

	// 3rd request (no tokens left) -> 429
	rr3 := httptest.NewRecorder()
	handler.ServeHTTP(rr3, req)
	if rr3.Code != http.StatusTooManyRequests {
		t.Errorf("req 3 expected 429 Too Many Requests, got %d", rr3.Code)
	}

	// Wait 1 second to replenish 1 token
	time.Sleep(1100 * time.Millisecond)

	// 4th request -> 200
	rr4 := httptest.NewRecorder()
	handler.ServeHTTP(rr4, req)
	if rr4.Code != http.StatusOK {
		t.Errorf("req 4 after delay expected 200, got %d", rr4.Code)
	}
}

func TestGenerateID_ErrorPath(t *testing.T) {
	original := randReader
	defer func() { randReader = original }()

	randReader = func(b []byte) (int, error) {
		return 0, fmt.Errorf("simulated entropy failure")
	}

	id := generateID()
	if id != "req-unknown" {
		t.Errorf("expected 'req-unknown' on rand failure, got %q", id)
	}
}
