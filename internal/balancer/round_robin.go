package balancer

import (
	"net/http"
	"sync/atomic"

	"gobalancer/internal/backend"
)

type RoundRobin struct {
	pool    *backend.BackendPool
	counter uint64
}

func NewRoundRobin(pool *backend.BackendPool) *RoundRobin {
	return &RoundRobin{
		pool: pool,
	}
}

func (rr *RoundRobin) NextBackend(req *http.Request) (*backend.Backend, error) {
	healthy := rr.pool.GetHealthyBackends()
	if len(healthy) == 0 {
		return nil, ErrNoHealthyBackends
	}

	idx := atomic.AddUint64(&rr.counter, 1) - 1
	selected := healthy[idx%uint64(len(healthy))]
	return selected, nil
}
