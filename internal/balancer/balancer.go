package balancer

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/sergio11/gobalancer_poc/internal/backend"
)

var ErrNoHealthyBackends = errors.New("no healthy backends available")

type Balancer interface {
	NextBackend(req *http.Request) (*backend.Backend, error)
}

func NewBalancer(algorithm string, pool *backend.BackendPool) (Balancer, error) {
	switch strings.ToLower(algorithm) {
	case "round-robin", "roundrobin", "":
		return NewRoundRobin(pool), nil
	case "least-connections", "leastconnections", "least_conn":
		return NewLeastConnections(pool), nil
	case "weighted-round-robin", "weighted", "wrr":
		return NewWeightedRoundRobin(pool), nil
	case "random":
		return NewRandom(pool), nil
	case "ip-hash", "iphash":
		return NewIPHash(pool), nil
	default:
		return nil, fmt.Errorf("unsupported load balancing algorithm: %s", algorithm)
	}
}
