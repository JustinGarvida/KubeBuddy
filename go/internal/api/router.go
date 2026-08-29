// Package api exposes the Go agent's REST API: a chi router serving
// the endpoints the dashboard reads from, backed by the Postgres
// store (see docs/architecture.md for the underlying schema).
package api

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"podsentinel/internal/store"
)

// NewRouter builds the agent's HTTP handler tree.
//
// Purpose: wires up the chi router, middleware, and route table for
// the REST API the dashboard reads from.
// Params:
//   - logger: structured logger used by request logging and handlers.
//   - st: the Postgres-backed store handlers read from.
//
// Returns: an http.Handler ready to be served.
func NewRouter(logger *slog.Logger, st *store.Store) http.Handler {
	h := &handlers{logger: logger, store: st}

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

// requestLogger builds a logging middleware.
//
// Purpose: logs each request's method, path, status, and duration
// through the agent's structured logger.
// Params:
//   - logger: the structured logger to write request log lines to.
//
// Returns: middleware that wraps an http.Handler with request logging.
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
