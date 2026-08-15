package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"gobalancer/internal/backend"
	"gobalancer/internal/config"
)

func TestAdminAPI_GetStats(t *testing.T) {
	cfg := []config.BackendConfig{
		{URL: "http://localhost:8081", Weight: 1},
		{URL: "http://localhost:8082", Weight: 1},
	}
	pool, _ := backend.NewBackendPool(cfg)

	// Mark one unhealthy
	pool.GetBackends()[1].SetStatus(backend.StatusUnhealthy)

	admin := NewAdminAPI(pool, "", nil, "")
	mux := http.NewServeMux()
	admin.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/stats", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var stats StatsDTO
	if err := json.NewDecoder(rr.Body).Decode(&stats); err != nil {
		t.Fatalf("failed to decode stats: %v", err)
	}
	if stats.TotalBackends != 2 {
		t.Errorf("expected 2 total backends, got %d", stats.TotalBackends)
	}
	if stats.HealthyBackends != 1 {
		t.Errorf("expected 1 healthy, got %d", stats.HealthyBackends)
	}
	if stats.UnhealthyCount != 1 {
		t.Errorf("expected 1 unhealthy, got %d", stats.UnhealthyCount)
	}
}

func TestAdminAPI_ReloadConfig_BadYAML(t *testing.T) {
	cfg := []config.BackendConfig{
		{URL: "http://localhost:8081", Weight: 1},
	}
	pool, _ := backend.NewBackendPool(cfg)

	tmpDir := t.TempDir()
	badPath := filepath.Join(tmpDir, "bad.yaml")
	_ = os.WriteFile(badPath, []byte(`{invalid yaml:`), 0644)

	admin := NewAdminAPI(pool, badPath, nil, "")
	mux := http.NewServeMux()
	admin.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/reload", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for bad YAML, got %d", rr.Code)
	}
}

func TestAdminAPI_ReloadConfig_CallbackError(t *testing.T) {
	cfg := []config.BackendConfig{
		{URL: "http://localhost:8081", Weight: 1},
	}
	pool, _ := backend.NewBackendPool(cfg)

	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	_ = os.WriteFile(cfgPath, []byte(`
server:
  port: 8080
backends:
  - url: http://localhost:8081
`), 0644)

	admin := NewAdminAPI(pool, cfgPath, func(cfg *config.Config) error {
		return fmt.Errorf("simulated reload error")
	}, "")
	mux := http.NewServeMux()
	admin.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/reload", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for callback error, got %d", rr.Code)
	}
}

func TestAdminAPI_Auth_NoSecret_AllowsAll(t *testing.T) {
	cfg := []config.BackendConfig{
		{URL: "http://localhost:8081", Weight: 1},
	}
	pool, _ := backend.NewBackendPool(cfg)

	admin := NewAdminAPI(pool, "", nil, "")
	mux := http.NewServeMux()
	admin.RegisterRoutes(mux)

	endpoints := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/backends"},
		{http.MethodGet, "/api/stats"},
		{http.MethodGet, "/health"},
	}

	for _, ep := range endpoints {
		req := httptest.NewRequest(ep.method, ep.path, nil)
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("%s %s: expected 200 with no secret, got %d", ep.method, ep.path, rr.Code)
		}
	}
}

func TestAdminAPI_Auth_WithSecret_RejectsNoCredentials(t *testing.T) {
	cfg := []config.BackendConfig{
		{URL: "http://localhost:8081", Weight: 1},
	}
	pool, _ := backend.NewBackendPool(cfg)

	admin := NewAdminAPI(pool, "", nil, "my-secret")
	mux := http.NewServeMux()
	admin.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/backends", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 without credentials, got %d", rr.Code)
	}
}

func TestAdminAPI_Auth_WithSecret_RejectsWrongPassword(t *testing.T) {
	cfg := []config.BackendConfig{
		{URL: "http://localhost:8081", Weight: 1},
	}
	pool, _ := backend.NewBackendPool(cfg)

	admin := NewAdminAPI(pool, "", nil, "my-secret")
	mux := http.NewServeMux()
	admin.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/backends", nil)
	req.SetBasicAuth("admin", "wrong-password")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 with wrong password, got %d", rr.Code)
	}
}

func TestAdminAPI_Auth_WithSecret_AcceptsCorrectPassword(t *testing.T) {
	cfg := []config.BackendConfig{
		{URL: "http://localhost:8081", Weight: 1},
	}
	pool, _ := backend.NewBackendPool(cfg)

	admin := NewAdminAPI(pool, "", nil, "my-secret")
	mux := http.NewServeMux()
	admin.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/backends", nil)
	req.SetBasicAuth("admin", "my-secret")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 with correct password, got %d", rr.Code)
	}
}

func TestAdminAPI_Auth_HealthEndpoint_NoAuth(t *testing.T) {
	cfg := []config.BackendConfig{
		{URL: "http://localhost:8081", Weight: 1},
	}
	pool, _ := backend.NewBackendPool(cfg)

	admin := NewAdminAPI(pool, "", nil, "my-secret")
	mux := http.NewServeMux()
	admin.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("/health should not require auth, got %d", rr.Code)
	}
}
