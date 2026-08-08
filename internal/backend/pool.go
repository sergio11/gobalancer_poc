package backend

import (
	"fmt"
	"sync"

	"gobalancer/internal/config"
)

type BackendPool struct {
	backends []*Backend
	mu       sync.RWMutex
}

func NewBackendPool(cfg []config.BackendConfig) (*BackendPool, error) {
	pool := &BackendPool{
		backends: make([]*Backend, 0, len(cfg)),
	}

	for i, bCfg := range cfg {
		id := fmt.Sprintf("backend-%d", i+1)
		b, err := NewBackend(id, bCfg.URL, bCfg.Weight)
		if err != nil {
			return nil, fmt.Errorf("failed to parse backend URL %s: %w", bCfg.URL, err)
		}
		pool.backends = append(pool.backends, b)
	}

	return pool, nil
}

func (p *BackendPool) AddBackend(b *Backend) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.backends = append(p.backends, b)
}

func (p *BackendPool) GetBackends() []*Backend {
	p.mu.RLock()
	defer p.mu.RUnlock()
	result := make([]*Backend, len(p.backends))
	copy(result, p.backends)
	return result
}

func (p *BackendPool) GetHealthyBackends() []*Backend {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var healthy []*Backend
	for _, b := range p.backends {
		if b.IsHealthy() {
			healthy = append(healthy, b)
		}
	}
	return healthy
}

func (p *BackendPool) GetBackendByURL(rawURL string) *Backend {
	p.mu.RLock()
	defer p.mu.RUnlock()

	for _, b := range p.backends {
		if b.URL.String() == rawURL {
			return b
		}
	}
	return nil
}
