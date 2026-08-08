package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type ServerConfig struct {
	Port int `yaml:"port"`
}

type LoadBalancerConfig struct {
	Algorithm string `yaml:"algorithm"`
}

type HealthCheckConfig struct {
	Interval    time.Duration `yaml:"interval"`
	Timeout     time.Duration `yaml:"timeout"`
	MaxFailures int           `yaml:"maxFailures"`
}

type BackendConfig struct {
	URL    string `yaml:"url"`
	Weight int    `yaml:"weight"`
}

type Config struct {
	Server       ServerConfig       `yaml:"server"`
	LoadBalancer LoadBalancerConfig `yaml:"loadBalancer"`
	HealthCheck  HealthCheckConfig  `yaml:"healthCheck"`
	Backends     []BackendConfig    `yaml:"backends"`
}

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse YAML config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return &cfg, nil
}

func (c *Config) Validate() error {
	if c.Server.Port <= 0 || c.Server.Port > 65535 {
		return fmt.Errorf("invalid server port: %d", c.Server.Port)
	}

	if c.LoadBalancer.Algorithm == "" {
		c.LoadBalancer.Algorithm = "round-robin"
	}

	if c.HealthCheck.Interval <= 0 {
		c.HealthCheck.Interval = 5 * time.Second
	}

	if c.HealthCheck.Timeout <= 0 {
		c.HealthCheck.Timeout = 2 * time.Second
	}

	if c.HealthCheck.MaxFailures <= 0 {
		c.HealthCheck.MaxFailures = 3
	}

	if len(c.Backends) == 0 {
		return fmt.Errorf("at least one backend must be specified")
	}

	for i, b := range c.Backends {
		if b.URL == "" {
			return fmt.Errorf("backend at index %d has empty URL", i)
		}
		if b.Weight <= 0 {
			c.Backends[i].Weight = 1
		}
	}

	return nil
}
