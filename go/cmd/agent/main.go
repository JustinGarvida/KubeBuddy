// Command agent runs the PodSentinel Go agent: it will poll the
// Kubernetes Metrics API, write to Postgres, publish/consume RabbitMQ
// events, and serve the REST API the dashboard reads from. Today it
// wires up config, logging, and the REST API scaffold; polling,
// Postgres, and RabbitMQ land in follow-up work.
package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"podsentinel/internal/api"
	"podsentinel/internal/config"
	"podsentinel/internal/logging"
)

func main() {
	cfg := config.Load()
	logger := logging.New(cfg.LogLevel)

	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: api.NewRouter(logger),
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		logger.Info("starting server", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server failed", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	logger.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
		os.Exit(1)
	}

	logger.Info("shutdown complete")
}
