package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gobalancer/internal/api"
	"gobalancer/internal/backend"
	"gobalancer/internal/balancer"
	"gobalancer/internal/config"
	"gobalancer/internal/health"
	"gobalancer/internal/logger"
	"gobalancer/internal/metrics"
	"gobalancer/internal/middleware"
	"gobalancer/internal/proxy"
)

func main() {
	log := logger.Init("info")
	log.Info("Starting GoBalancer L7 Load Balancer...")

	configPath := "configs/config.yaml"
	if len(os.Args) > 1 {
		configPath = os.Args[1]
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		log.Error("Failed to load configuration", "error", err)
		os.Exit(1)
	}

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

	revProxy := proxy.NewReverseProxy(balEngine)
	metricsSvc := metrics.NewMetricsService(pool)

	onReload := func(newCfg *config.Config) error {
		newPool, err := backend.NewBackendPool(newCfg.Backends)
		if err != nil {
			return err
		}
		newBal, err := balancer.NewBalancer(newCfg.LoadBalancer.Algorithm, newPool)
		if err != nil {
			return err
		}
		*pool = *newPool
		balEngine = newBal
		log.Info("Configuration reloaded successfully")
		return nil
	}

	adminAPI := api.NewAdminAPI(pool, configPath, onReload)

	// Routing setup
	mux := http.NewServeMux()
	adminAPI.RegisterRoutes(mux)
	mux.HandleFunc("GET /metrics", metricsSvc.Handler())
	mux.Handle("/", revProxy)

	// Rate Limiter
	limiter := middleware.NewRateLimiter(5000, 10000, true)

	handler := middleware.Chain(
		mux,
		middleware.Recovery,
		middleware.RequestID,
		middleware.Logger,
		limiter.Middleware,
	)

	serverAddr := fmt.Sprintf(":%d", cfg.Server.Port)
	srv := &http.Server{
		Addr:         serverAddr,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
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
