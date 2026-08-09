// Package api scaffolds the Go agent's REST API: a chi router exposing
// the endpoints the dashboard reads from. Handlers currently return
// placeholder data -- see docs/architecture.md for the data they'll
// eventually be backed by.
package api

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// NewRouter builds the agent's HTTP handler tree.
func NewRouter(logger *slog.Logger) http.Handler {
	h := &handlers{logger: logger}

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)
	r.Use(requestLogger(logger))

	r.Get("/health", h.health)

	r.Route("/api/v1/pods", func(r chi.Router) {
		r.Get("/", h.listPods)
		r.Get("/{namespace}/{pod}", h.getPod)
		r.Get("/{namespace}/{pod}/anomalies", h.listPodAnomalies)
	})

	return r
}

// requestLogger logs each request's method, path, status, and duration
// through the agent's structured logger.
func requestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			next.ServeHTTP(ww, r)

			logger.Info("http request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", ww.Status(),
				"duration", time.Since(start).String(),
			)
		})
	}
}
