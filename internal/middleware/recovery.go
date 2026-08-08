package middleware

import (
	"fmt"
	"net/http"
	"runtime/debug"

	"gobalancer/internal/logger"
)

func Recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				log := logger.Get()
				log.Error("Panic recovered in HTTP handler",
					"error", err,
					"stack", string(debug.Stack()),
					"path", r.URL.Path,
				)
				http.Error(w, fmt.Sprintf("Internal Server Error: panic recovered (%v)", err), http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
