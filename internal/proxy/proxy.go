package proxy

import (
	"net/http"
	"net/http/httputil"
	"sync"
	"time"

	"github.com/sergio11/gobalancer_poc/internal/backend"
	"github.com/sergio11/gobalancer_poc/internal/balancer"
	"github.com/sergio11/gobalancer_poc/internal/logger"
	"github.com/sergio11/gobalancer_poc/internal/metrics"
)

type ReverseProxy struct {
	balancer    balancer.Balancer
	metrics     *metrics.MetricsService
	maxFailures int
	transport   *http.Transport
	proxyCache  sync.Map
}

func NewReverseProxy(b balancer.Balancer, m *metrics.MetricsService, maxFailures int) *ReverseProxy {
	return &ReverseProxy{
		balancer:    b,
		metrics:     m,
		maxFailures: maxFailures,
		transport: &http.Transport{
			ResponseHeaderTimeout: 10 * time.Second,
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
		},
	}
}

func (rp *ReverseProxy) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	targetBackend, err := rp.balancer.NextBackend(req)
	if err != nil {
		log := logger.Get()
		log.Error("No healthy backend available", "path", req.URL.Path, "error", err)
		if rp.metrics != nil {
			rp.metrics.IncErrors()
		}
		http.Error(w, "Service Unavailable: No healthy backends", http.StatusServiceUnavailable)
		return
	}

	if rp.metrics != nil {
		rp.metrics.IncRequests()
	}

	targetBackend.IncConnections()
	defer targetBackend.DecConnections()

	proxy := rp.getOrCreateProxy(targetBackend)
	proxy.ServeHTTP(w, req)
}

func (rp *ReverseProxy) getOrCreateProxy(b *backend.Backend) *httputil.ReverseProxy {
	if cached, ok := rp.proxyCache.Load(b.ID); ok {
		return cached.(*httputil.ReverseProxy)
	}
	proxy := rp.createHttpProxy(b)
	rp.proxyCache.Store(b.ID, proxy)
	return proxy
}

func (rp *ReverseProxy) createHttpProxy(b *backend.Backend) *httputil.ReverseProxy {
	targetURL := b.URL

	director := func(req *http.Request) {
		req.URL.Scheme = targetURL.Scheme
		req.URL.Host = targetURL.Host
		req.Host = targetURL.Host

		// Set X-Forwarded headers
		if clientIP := req.RemoteAddr; clientIP != "" {
			if prior := req.Header.Get("X-Forwarded-For"); prior != "" {
				req.Header.Set("X-Forwarded-For", prior+", "+clientIP)
			} else {
				req.Header.Set("X-Forwarded-For", clientIP)
			}
		}
		req.Header.Set("X-Forwarded-Host", req.Host)
		req.Header.Set("X-Forwarded-Proto", targetURL.Scheme)
	}

	errorHandler := func(w http.ResponseWriter, req *http.Request, err error) {
		log := logger.Get()
		log.Error("Backend proxy error",
			"backend_id", b.ID,
			"url", b.URL.String(),
			"error", err,
		)
		b.RecordFailure(int64(rp.maxFailures))
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
	}

	return &httputil.ReverseProxy{
		Director:      director,
		ErrorHandler:  errorHandler,
		Transport:     rp.transport,
	}
}
