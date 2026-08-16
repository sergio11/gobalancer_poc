package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/sergio11/gobalancer_poc/internal/backend"
	"github.com/sergio11/gobalancer_poc/internal/config"
	"github.com/sergio11/gobalancer_poc/internal/logger"
)

type AdminAPI struct {
	pool       *backend.BackendPool
	configPath string
	onReload   func(cfg *config.Config) error
	apiSecret  string
}

type BackendDTO struct {
	ID                string                `json:"id"`
	URL               string                `json:"url"`
	Status            backend.BackendStatus `json:"status"`
	Weight            int                   `json:"weight"`
	ActiveConnections int64                 `json:"active_connections"`
	LatencyMs         int64                 `json:"latency_ms"`
	Failures          int64                 `json:"failures"`
	Successes         int64                 `json:"successes"`
	LastHealthCheck   time.Time             `json:"last_health_check"`
}

type StatsDTO struct {
	TotalBackends   int `json:"total_backends"`
	HealthyBackends int `json:"healthy_backends"`
	UnhealthyCount  int `json:"unhealthy_backends"`
}

func NewAdminAPI(pool *backend.BackendPool, configPath string, onReload func(cfg *config.Config) error, apiSecret string) *AdminAPI {
	return &AdminAPI{
		pool:       pool,
		configPath: configPath,
		onReload:   onReload,
		apiSecret:  apiSecret,
	}
}

func (api *AdminAPI) withAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if api.apiSecret == "" {
			next(w, r)
			return
		}
		_, password, ok := r.BasicAuth()
		if !ok || password != api.apiSecret {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func (api *AdminAPI) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/backends", api.withAuth(api.HandleGetBackends))
	mux.HandleFunc("GET /api/stats", api.withAuth(api.HandleGetStats))
	mux.HandleFunc("POST /api/reload", api.withAuth(api.HandleReloadConfig))
	mux.HandleFunc("GET /health", api.HandleHealth)
}

func (api *AdminAPI) HandleGetBackends(w http.ResponseWriter, r *http.Request) {
	backends := api.pool.GetBackends()
	dtos := make([]BackendDTO, 0, len(backends))

	for _, b := range backends {
		dtos = append(dtos, BackendDTO{
			ID:                b.ID,
			URL:               b.URL.String(),
			Status:            b.GetStatus(),
			Weight:            b.Weight,
			ActiveConnections: b.GetConnections(),
			LatencyMs:         b.GetLatency().Milliseconds(),
			Failures:          b.Failures.Load(),
			Successes:         b.Successes.Load(),
			LastHealthCheck:   b.LastHealthCheck,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(dtos)
}

func (api *AdminAPI) HandleGetStats(w http.ResponseWriter, r *http.Request) {
	backends := api.pool.GetBackends()
	healthy := 0
	unhealthy := 0

	for _, b := range backends {
		if b.IsHealthy() {
			healthy++
		} else {
			unhealthy++
		}
	}

	stats := StatsDTO{
		TotalBackends:   len(backends),
		HealthyBackends: healthy,
		UnhealthyCount:  unhealthy,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(stats)
}

func (api *AdminAPI) HandleReloadConfig(w http.ResponseWriter, r *http.Request) {
	log := logger.Get()
	log.Info("Dynamic configuration reload requested", "config_path", api.configPath)

	newCfg, err := config.LoadConfig(api.configPath)
	if err != nil {
		log.Error("Failed to reload configuration", "error", err)
		http.Error(w, "Failed to parse updated YAML config: "+err.Error(), http.StatusBadRequest)
		return
	}

	if api.onReload != nil {
		if err := api.onReload(newCfg); err != nil {
			log.Error("Failed to apply reloaded configuration", "error", err)
			http.Error(w, "Failed to apply config: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"message": "Configuration reloaded successfully",
		"status":  "ok",
	})
}

func (api *AdminAPI) HandleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status":  "UP",
		"service": "GoBalancer",
	})
}
