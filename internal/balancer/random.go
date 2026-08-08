package balancer

import (
	"math/rand"
	"net/http"
	"sync"

	"gobalancer/internal/backend"
)

type Random struct {
	pool *backend.BackendPool
	rnd  *rand.Rand
	mu   sync.Mutex
}

func NewRandom(pool *backend.BackendPool) *Random {
	return &Random{
		pool: pool,
		rnd:  rand.New(rand.NewSource(rand.Int63())),
	}
}

func (r *Random) NextBackend(req *http.Request) (*backend.Backend, error) {
	healthy := r.pool.GetHealthyBackends()
	if len(healthy) == 0 {
		return nil, ErrNoHealthyBackends
	}

	r.mu.Lock()
	idx := r.rnd.Intn(len(healthy))
	r.mu.Unlock()

	return healthy[idx], nil
}
