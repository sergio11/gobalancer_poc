package balancer

import (
	"hash/fnv"
	"net/http"

	"gobalancer/internal/backend"
	"gobalancer/internal/httputil"
)

type IPHash struct {
	pool *backend.BackendPool
}

func NewIPHash(pool *backend.BackendPool) *IPHash {
	return &IPHash{
		pool: pool,
	}
}

func (iph *IPHash) NextBackend(req *http.Request) (*backend.Backend, error) {
	healthy := iph.pool.GetHealthyBackends()
	if len(healthy) == 0 {
		return nil, ErrNoHealthyBackends
	}

	clientIP := httputil.GetClientIP(req, true)
	h := fnv.New32a()
	_, _ = h.Write([]byte(clientIP))
	hashVal := h.Sum32()

	idx := int(hashVal % uint32(len(healthy)))
	return healthy[idx], nil
}
