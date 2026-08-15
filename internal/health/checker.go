package health

import (
	"context"
	"net/http"
	"sync"
	"time"

	"gobalancer/internal/backend"
	"gobalancer/internal/config"
	"gobalancer/internal/logger"
)

type HealthChecker struct {
	pool     *backend.BackendPool
	cfg      config.HealthCheckConfig
	client   *http.Client
	stopChan chan struct{}
	wg       sync.WaitGroup
}

func NewHealthChecker(pool *backend.BackendPool, cfg config.HealthCheckConfig) *HealthChecker {
	return &HealthChecker{
		pool: pool,
		cfg:  cfg,
		client: &http.Client{
			Timeout: cfg.Timeout,
		},
		stopChan: make(chan struct{}),
	}
}

func (hc *HealthChecker) Start(ctx context.Context) {
	ticker := time.NewTicker(hc.cfg.Interval)
	defer ticker.Stop()

	// Initial check
	hc.CheckAll(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-hc.stopChan:
			return
		case <-ticker.C:
			hc.CheckAll(ctx)
		}
	}
}

func (hc *HealthChecker) Stop() {
	close(hc.stopChan)
	hc.wg.Wait()
}

func (hc *HealthChecker) CheckAll(ctx context.Context) {
	backends := hc.pool.GetBackends()
	for _, b := range backends {
		hc.wg.Add(1)
		go func(b *backend.Backend) {
			defer hc.wg.Done()
			hc.CheckBackend(ctx, b)
		}(b)
	}
}

func (hc *HealthChecker) CheckBackend(ctx context.Context, b *backend.Backend) {
	healthURL := b.URL.String() + "/health"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
	if err != nil {
		b.RecordFailure(int64(hc.cfg.MaxFailures))
		return
	}

	start := time.Now()
	resp, err := hc.client.Do(req)
	latency := time.Since(start)

	log := logger.Get()

	if err != nil || resp.StatusCode != http.StatusOK {
		if resp != nil {
			resp.Body.Close()
		}
		b.RecordFailure(int64(hc.cfg.MaxFailures))
		log.Warn("Health check failed for backend",
			"backend_id", b.ID,
			"url", healthURL,
			"status", b.GetStatus(),
			"failures", b.Failures.Load(),
		)
		return
	}
	resp.Body.Close()

	wasUnhealthy := !b.IsHealthy()
	b.RecordSuccess(latency)

	if wasUnhealthy {
		log.Info("Backend recovered and marked HEALTHY",
			"backend_id", b.ID,
			"url", b.URL.String(),
			"latency_ms", latency.Milliseconds(),
		)
	}
}
