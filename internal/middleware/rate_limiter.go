package middleware

import (
	"net/http"
	"sync"
	"time"

	"gobalancer/internal/httputil"
)

type clientBucket struct {
	tokens     float64
	lastUpdate time.Time
}

type RateLimiter struct {
	rate           float64 // tokens per second
	capacity       float64 // max bucket capacity
	clients        map[string]*clientBucket
	mu             sync.Mutex
	enabled        bool
	done           chan struct{}
	cleanupTicker  time.Duration
}

func NewRateLimiter(rate float64, capacity float64, enabled bool) *RateLimiter {
	return NewRateLimiterWithCleanup(rate, capacity, enabled, 5*time.Minute)
}

func NewRateLimiterWithCleanup(rate float64, capacity float64, enabled bool, cleanupInterval time.Duration) *RateLimiter {
	rl := &RateLimiter{
		rate:          rate,
		capacity:      capacity,
		clients:       make(map[string]*clientBucket),
		enabled:       enabled,
		done:          make(chan struct{}),
		cleanupTicker: cleanupInterval,
	}

	// Periodically cleanup inactive clients
	go rl.cleanupLoop()

	return rl
}

func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !rl.enabled {
			next.ServeHTTP(w, r)
			return
		}

		clientIP := httputil.GetClientIP(r)

		rl.mu.Lock()
		b, exists := rl.clients[clientIP]
		now := time.Now()

		if !exists {
			b = &clientBucket{
				tokens:     rl.capacity - 1, // consume 1 token for current req
				lastUpdate: now,
			}
			rl.clients[clientIP] = b
			rl.mu.Unlock()
			next.ServeHTTP(w, r)
			return
		}

		// Refill tokens
		elapsed := now.Sub(b.lastUpdate).Seconds()
		b.tokens += elapsed * rl.rate
		if b.tokens > rl.capacity {
			b.tokens = rl.capacity
		}
		b.lastUpdate = now

		if b.tokens >= 1 {
			b.tokens -= 1
			rl.mu.Unlock()
			next.ServeHTTP(w, r)
		} else {
			rl.mu.Unlock()
			w.Header().Set("Retry-After", "1")
			http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
		}
	})
}

func (rl *RateLimiter) cleanup() {
	rl.mu.Lock()
	now := time.Now()
	for ip, b := range rl.clients {
		if now.Sub(b.lastUpdate) > 10*time.Minute {
			delete(rl.clients, ip)
		}
	}
	rl.mu.Unlock()
}

func (rl *RateLimiter) cleanupLoop() {
	ticker := time.NewTicker(rl.cleanupTicker)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			rl.cleanup()
		case <-rl.done:
			return
		}
	}
}

func (rl *RateLimiter) Stop() {
	close(rl.done)
}
