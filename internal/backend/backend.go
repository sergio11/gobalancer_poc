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
	Failures          int64
	Successes         int64
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
	b.mu.Lock()
	defer b.mu.Unlock()
	b.Successes++
	b.Failures = 0
	b.Latency = latency
	b.Status = StatusHealthy
	b.LastHealthCheck = time.Now()
}

func (b *Backend) RecordFailure(maxFailures int64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.Failures++
	if b.Failures >= maxFailures {
		b.Status = StatusUnhealthy
	}
	b.LastHealthCheck = time.Now()
}
