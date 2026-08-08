package backend

import (
	"net/url"
	"sync"
	"sync/atomic"
	"time"
)

type BackendStatus string

const (
	StatusHealthy   BackendStatus = "HEALTHY"
	StatusUnhealthy BackendStatus = "UNHEALTHY"
)

type Backend struct {
	ID                string
	URL               *url.URL
	Status            BackendStatus
	Weight            int
	ActiveConnections int64
	Latency           time.Duration
	Failures          atomic.Int64
	Successes         atomic.Int64
	LastHealthCheck   time.Time
	mu                sync.RWMutex
}

func NewBackend(id string, rawURL string, weight int) (*Backend, error) {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}

	if weight <= 0 {
		weight = 1
	}

	return &Backend{
		ID:              id,
		URL:             parsedURL,
		Status:          StatusHealthy,
		Weight:          weight,
		LastHealthCheck: time.Now(),
	}, nil
}

func (b *Backend) IncConnections() {
	atomic.AddInt64(&b.ActiveConnections, 1)
}

func (b *Backend) DecConnections() {
	atomic.AddInt64(&b.ActiveConnections, -1)
}

func (b *Backend) GetConnections() int64 {
	return atomic.LoadInt64(&b.ActiveConnections)
}

func (b *Backend) SetStatus(status BackendStatus) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.Status = status
}

func (b *Backend) GetStatus() BackendStatus {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.Status
}

func (b *Backend) IsHealthy() bool {
	return b.GetStatus() == StatusHealthy
}

func (b *Backend) RecordSuccess(latency time.Duration) {
	b.Successes.Add(1)
	b.Failures.Store(0)
	b.mu.Lock()
	defer b.mu.Unlock()
	b.Latency = latency
	b.Status = StatusHealthy
	b.LastHealthCheck = time.Now()
}

func (b *Backend) RecordFailure(maxFailures int64) {
	b.Failures.Add(1)
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.Failures.Load() >= maxFailures {
		b.Status = StatusUnhealthy
	}
	b.LastHealthCheck = time.Now()
}
