package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"gobalancer/internal/backend"
	"gobalancer/internal/config"
)

func TestAdminAPI_Endpoints(t *testing.T) {
	cfg := []config.BackendConfig{
		{URL: "http://localhost:8081", Weight: 1},
	}
	pool, _ := backend.NewBackendPool(cfg)

	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	_ = os.WriteFile(cfgPath, []byte(`
server:
  port: 8080
loadBalancer:
  algorithm: round-robin
backends:
  - url: http://localhost:8081
`), 0644)

	reloaded := false
	onReload := func(newCfg *config.Config) error {
		reloaded = true
		return nil
	}

	admin := NewAdminAPI(pool, cfgPath, onReload, "")
	mux := http.NewServeMux()
	admin.RegisterRoutes(mux)

	// Test GET /health
	reqHealth := httptest.NewRequest(http.MethodGet, "/health", nil)
	rrHealth := httptest.NewRecorder()
	mux.ServeHTTP(rrHealth, reqHealth)
	if rrHealth.Code != http.StatusOK {
		t.Errorf("expected 200 for /health, got %d", rrHealth.Code)
	}

	// Test GET /api/backends
	reqBackends := httptest.NewRequest(http.MethodGet, "/api/backends", nil)
	rrBackends := httptest.NewRecorder()
	mux.ServeHTTP(rrBackends, reqBackends)
	if rrBackends.Code != http.StatusOK {
		t.Errorf("expected 200 for /api/backends, got %d", rrBackends.Code)
	}

	var dtos []BackendDTO
	_ = json.Unmarshal(rrBackends.Body.Bytes(), &dtos)
	if len(dtos) != 1 {
		t.Errorf("expected 1 backend in JSON, got %d", len(dtos))
	}

	// Test POST /api/reload
	reqReload := httptest.NewRequest(http.MethodPost, "/api/reload", nil)
	rrReload := httptest.NewRecorder()
	mux.ServeHTTP(rrReload, reqReload)
	if rrReload.Code != http.StatusOK {
		t.Errorf("expected 200 for /api/reload, got %d", rrReload.Code)
	}
	if !reloaded {
		t.Errorf("expected onReload callback to be invoked")
	}
}
