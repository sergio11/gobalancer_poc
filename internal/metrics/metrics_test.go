package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gobalancer/internal/backend"
	"gobalancer/internal/config"
)

func TestMetricsHandler_CorrectOutput(t *testing.T) {
	cfg := []config.BackendConfig{
		{URL: "http://localhost:9001", Weight: 1},
		{URL: "http://localhost:9002", Weight: 1},
	}
	pool, _ := backend.NewBackendPool(cfg)

	svc := NewMetricsService(pool)

	// Simulate some counters
	svc.IncRequests()
	svc.IncRequests()
	svc.IncErrors()

	// Mark second backend unhealthy
	pool.GetBackends()[1].SetStatus(backend.StatusUnhealthy)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rr := httptest.NewRecorder()
	svc.Handler()(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", rr.Code)
	}

	body := rr.Body.String()

	for _, expected := range []string{
		"requests_total 2",
		"backend_errors_total 1",
		"healthy_backends 1",
		"unhealthy_backends 1",
	} {
		if !strings.Contains(body, expected) {
			t.Errorf("expected metrics output to contain %q, got:\n%s", expected, body)
		}
	}
}
