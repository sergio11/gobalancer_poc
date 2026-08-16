package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/sergio11/gobalancer_poc/internal/api"
	"github.com/sergio11/gobalancer_poc/internal/backend"
	"github.com/sergio11/gobalancer_poc/internal/balancer"
	"github.com/sergio11/gobalancer_poc/internal/config"
	"github.com/sergio11/gobalancer_poc/internal/health"
	"github.com/sergio11/gobalancer_poc/internal/logger"
	"github.com/sergio11/gobalancer_poc/internal/metrics"
	"github.com/sergio11/gobalancer_poc/internal/middleware"
	"github.com/sergio11/gobalancer_poc/internal/proxy"
)

type safeBalancer struct {
	mu    sync.RWMutex
	inner balancer.Balancer
}

func (s *safeBalancer) NextBackend(req *http.Request) (*backend.Backend, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.inner.NextBackend(req)
}

func main() {
	configPath := "configs/config.yaml"
	if len(os.Args) > 1 {
		configPath = os.Args[1]
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	log := logger.Init(cfg.Server.LogLevel)
	log.Info("Starting GoBalancer L7 Load Balancer...")

	pool, err := backend.NewBackendPool(cfg.Backends)
	if err != nil {
		log.Error("Failed to initialize backend pool", "error", err)
		os.Exit(1)
	}

	checker := health.NewHealthChecker(pool, cfg.HealthCheck)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go checker.Start(ctx)

	balEngine, err := balancer.NewBalancer(cfg.LoadBalancer.Algorithm, pool)
	if err != nil {
		log.Error("Failed to initialize load balancer engine", "error", err)
		os.Exit(1)
	}

	sb := &safeBalancer{inner: balEngine}
	metricsSvc := metrics.NewMetricsService(pool)
	revProxy := proxy.NewReverseProxy(sb, metricsSvc, cfg.HealthCheck.MaxFailures)

	onReload := func(newCfg *config.Config) error {
		newPool, err := backend.NewBackendPool(newCfg.Backends)
		if err != nil {
			return err
		}
		newBal, err := balancer.NewBalancer(newCfg.LoadBalancer.Algorithm, newPool)
		if err != nil {
			return err
		}
		pool.ReplaceBackends(newPool.GetBackends())
		sb.mu.Lock()
		sb.inner = newBal
		sb.mu.Unlock()

		checker.Stop()
		checker = health.NewHealthChecker(pool, newCfg.HealthCheck)
		go checker.Start(ctx)

		log.Info("Configuration reloaded successfully")
		return nil
	}

	adminAPI := api.NewAdminAPI(pool, configPath, onReload, cfg.Admin.Secret)

	// Routing setup
	mux := http.NewServeMux()
	adminAPI.RegisterRoutes(mux)
	mux.HandleFunc("GET /metrics", metricsSvc.Handler())
	mux.Handle("/", revProxy)

	// Rate Limiter
	limiter := middleware.NewRateLimiter(cfg.RateLimit.Rate, cfg.RateLimit.Capacity, cfg.RateLimit.Enabled, cfg.RateLimit.TrustForwardedHeaders)
	defer limiter.Stop()

	handler := middleware.Chain(
		mux,
		middleware.Recovery,
		middleware.RequestID,
		middleware.Logger,
		limiter.Middleware,
	)

	serverAddr := fmt.Sprintf(":%d", cfg.Server.Port)
	srv := &http.Server{
		Addr:              serverAddr,
		Handler:           handler,
		ReadTimeout:       15 * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	go func() {
		log.Info("GoBalancer listening for traffic",
			"addr", serverAddr,
			"algorithm", cfg.LoadBalancer.Algorithm,
			"backends", len(cfg.Backends),
		)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("HTTP Server error", "error", err)
		}
	}()

	// Graceful shutdown handling
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	<-stop
	log.Info("Shutting down GoBalancer gracefully...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	checker.Stop()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("Forced shutdown error", "error", err)
	} else {
		log.Info("GoBalancer stopped cleanly")
	}
}
