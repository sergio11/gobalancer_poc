package proxy

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"gobalancer/internal/backend"
	"gobalancer/internal/balancer"
	"gobalancer/internal/logger"
)

type ReverseProxy struct {
	balancer balancer.Balancer
}

func NewReverseProxy(b balancer.Balancer) *ReverseProxy {
	return &ReverseProxy{
		balancer: b,
	}
}

func (rp *ReverseProxy) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	targetBackend, err := rp.balancer.NextBackend(req)
	if err != nil {
		log := logger.Get()
		log.Error("No healthy backend available", "path", req.URL.Path, "error", err)
		http.Error(w, "Service Unavailable: No healthy backends", http.StatusServiceUnavailable)
		return
	}

	targetBackend.IncConnections()
	defer targetBackend.DecConnections()

	proxy := rp.createHttpProxy(targetBackend)
	proxy.ServeHTTP(w, req)
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
		b.RecordFailure(3)
		http.Error(w, fmt.Sprintf("Bad Gateway: %v", err), http.StatusBadGateway)
	}

	transport := &http.Transport{
		ResponseHeaderTimeout: 10 * time.Second,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
	}

	return &httputil.ReverseProxy{
		Director:      director,
		ErrorHandler:  errorHandler,
		Transport:     transport,
	}
}

func parseURL(raw string) *url.URL {
	u, _ := url.Parse(raw)
	return u
}
