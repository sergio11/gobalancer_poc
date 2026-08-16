package metrics

import (
	"fmt"
	"net/http"
	"sync/atomic"

	"github.com/sergio11/gobalancer_poc/internal/backend"
)

type MetricsService struct {
	pool          *backend.BackendPool
	totalRequests uint64
	totalErrors   uint64
}

func NewMetricsService(pool *backend.BackendPool) *MetricsService {
	return &MetricsService{
		pool: pool,
	}
}

func (m *MetricsService) IncRequests() {
	atomic.AddUint64(&m.totalRequests, 1)
}

func (m *MetricsService) IncErrors() {
	atomic.AddUint64(&m.totalErrors, 1)
}

func (m *MetricsService) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		backends := m.pool.GetBackends()
		healthyCount := 0
		unhealthyCount := 0
		var totalActiveConns int64

		for _, b := range backends {
			if b.IsHealthy() {
				healthyCount++
			} else {
				unhealthyCount++
			}
			totalActiveConns += b.GetConnections()
		}

		w.Header().Set("Content-Type", "text/plain; version=0.0.4")

		fmt.Fprintf(w, "# HELP requests_total Total HTTP requests handled\n")
		fmt.Fprintf(w, "# TYPE requests_total counter\n")
		fmt.Fprintf(w, "requests_total %d\n\n", atomic.LoadUint64(&m.totalRequests))

		fmt.Fprintf(w, "# HELP backend_errors_total Total backend errors\n")
		fmt.Fprintf(w, "# TYPE backend_errors_total counter\n")
		fmt.Fprintf(w, "backend_errors_total %d\n\n", atomic.LoadUint64(&m.totalErrors))

		fmt.Fprintf(w, "# HELP active_connections Active connections count\n")
		fmt.Fprintf(w, "# TYPE active_connections gauge\n")
		fmt.Fprintf(w, "active_connections %d\n\n", totalActiveConns)

		fmt.Fprintf(w, "# HELP healthy_backends Healthy backends count\n")
		fmt.Fprintf(w, "# TYPE healthy_backends gauge\n")
		fmt.Fprintf(w, "healthy_backends %d\n\n", healthyCount)

		fmt.Fprintf(w, "# HELP unhealthy_backends Unhealthy backends count\n")
		fmt.Fprintf(w, "# TYPE unhealthy_backends gauge\n")
		fmt.Fprintf(w, "unhealthy_backends %d\n\n", unhealthyCount)

		for _, b := range backends {
			fmt.Fprintf(w, "backend_latency_ms{backend=\"%s\",url=\"%s\"} %d\n", b.ID, b.URL.String(), b.GetLatency().Milliseconds())
			fmt.Fprintf(w, "backend_active_connections{backend=\"%s\",url=\"%s\"} %d\n", b.ID, b.URL.String(), b.GetConnections())
		}
	}
}
