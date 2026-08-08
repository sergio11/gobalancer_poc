package main

import (
	"fmt"
	"os"

	"gobalancer/internal/config"
	"gobalancer/internal/logger"
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

	log.Info("Configuration loaded successfully",
		"port", cfg.Server.Port,
		"algorithm", cfg.LoadBalancer.Algorithm,
		"backends_count", len(cfg.Backends),
	)

	fmt.Printf("GoBalancer running on port :%d with %s algorithm\n", cfg.Server.Port, cfg.LoadBalancer.Algorithm)
}
