package balancer

import (
	"net/http"

	"gobalancer/internal/backend"
)

type LeastConnections struct {
	pool *backend.BackendPool
}

func NewLeastConnections(pool *backend.BackendPool) *LeastConnections {
	return &LeastConnections{
		pool: pool,
	}
}

func (lc *LeastConnections) NextBackend(req *http.Request) (*backend.Backend, error) {
	healthy := lc.pool.GetHealthyBackends()
	if len(healthy) == 0 {
		return nil, ErrNoHealthyBackends
	}

	best := healthy[0]
	minConns := best.GetConnections()

	for _, b := range healthy[1:] {
		conns := b.GetConnections()
		if conns < minConns {
			best = b
			minConns = conns
		}
	}

	return best, nil
}
