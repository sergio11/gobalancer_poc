package balancer

import (
	"net/http"
	"sync"

	"github.com/sergio11/gobalancer_poc/internal/backend"
)

type WeightedRoundRobin struct {
	pool           *backend.BackendPool
	currentWeights map[string]int
	mu             sync.Mutex
}

func NewWeightedRoundRobin(pool *backend.BackendPool) *WeightedRoundRobin {
	return &WeightedRoundRobin{
		pool:           pool,
		currentWeights: make(map[string]int),
	}
}

func (wrr *WeightedRoundRobin) NextBackend(req *http.Request) (*backend.Backend, error) {
	wrr.mu.Lock()
	defer wrr.mu.Unlock()

	healthy := wrr.pool.GetHealthyBackends()
	if len(healthy) == 0 {
		return nil, ErrNoHealthyBackends
	}

	validIDs := make(map[string]bool, len(healthy))
	for _, b := range healthy {
		validIDs[b.ID] = true
	}
	for id := range wrr.currentWeights {
		if !validIDs[id] {
			delete(wrr.currentWeights, id)
		}
	}

	totalWeight := 0
	var maxBackend *backend.Backend
	maxCurrentWeight := -1 << 31

	for _, b := range healthy {
		weight := b.Weight
		if weight <= 0 {
			weight = 1
		}
		totalWeight += weight

		wrr.currentWeights[b.ID] += weight
		if wrr.currentWeights[b.ID] > maxCurrentWeight {
			maxCurrentWeight = wrr.currentWeights[b.ID]
			maxBackend = b
		}
	}

	if maxBackend != nil {
		wrr.currentWeights[maxBackend.ID] -= totalWeight
	}

	return maxBackend, nil
}
