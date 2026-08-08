package balancer

import (
	"hash/fnv"
	"net"
	"net/http"
	"strings"

	"gobalancer/internal/backend"
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

	clientIP := getClientIP(req)
	h := fnv.New32a()
	_, _ = h.Write([]byte(clientIP))
	hashVal := h.Sum32()

	idx := int(hashVal % uint32(len(healthy)))
	return healthy[idx], nil
}

func getClientIP(req *http.Request) string {
	if req == nil {
		return "127.0.0.1"
	}
	if xff := req.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	if xri := req.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}
	ip, _, err := net.SplitHostPort(req.RemoteAddr)
	if err != nil {
		return req.RemoteAddr
	}
	return ip
}
