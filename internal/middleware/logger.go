package middleware

import (
	"net/http"
	"time"

	"gobalancer/internal/logger"
)

type responseWriterWrapper struct {
	http.ResponseWriter
	statusCode int
	bytesWritten int64
}

func (rw *responseWriterWrapper) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriterWrapper) Write(b []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(b)
	rw.bytesWritten += int64(n)
	return n, err
}

func Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &responseWriterWrapper{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(rw, r)

		latency := time.Since(start)
		log := logger.Get()
		reqID := GetRequestID(r.Context())

		log.Info("HTTP Request",
			"request_id", reqID,
			"method", r.Method,
			"path", r.URL.Path,
			"status", rw.statusCode,
			"latency_ms", latency.Milliseconds(),
			"bytes", rw.bytesWritten,
			"client_ip", r.RemoteAddr,
		)
	})
}
