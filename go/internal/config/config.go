// Package config loads the Go agent's runtime configuration from
// environment variables.
package config

import (
	"os"
	"strings"
	"time"
)

// Config holds the agent's runtime settings.
type Config struct {
	// Port is the TCP port the REST API listens on.
	Port string
	// LogLevel is the minimum level logged (debug, info, warn, error).
	LogLevel string
	// PostgresDSN is the connection string for the Postgres/TimescaleDB
	// instance the agent writes metrics and anomalies to. Required —
	// left empty if unset, which surfaces as a connection error at
	// startup rather than being validated here.
	PostgresDSN string
	// WatchNamespaces restricts polling to these namespaces. Empty
	// means watch all namespaces.
	WatchNamespaces []string
	// PollInterval is how often the agent polls the Kubernetes APIs.
	PollInterval time.Duration
}

// Load reads configuration from environment variables, falling back to
// defaults for anything unset.
func Load() Config {
	return Config{
		Port:            getEnv("PORT", "8080"),
		LogLevel:        getEnv("LOG_LEVEL", "info"),
		PostgresDSN:     getEnv("POSTGRES_DSN", ""),
		WatchNamespaces: parseNamespaces(getEnv("WATCH_NAMESPACES", "")),
		PollInterval:    parseDuration(getEnv("POLL_INTERVAL", "15s"), 15*time.Second),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// parseNamespaces splits a comma-separated namespace list, trimming
// whitespace and dropping empty entries. An empty input yields a nil
// slice, meaning "watch all namespaces".
func parseNamespaces(raw string) []string {
	if raw == "" {
		return nil
	}
	var namespaces []string
	for _, ns := range strings.Split(raw, ",") {
		ns = strings.TrimSpace(ns)
		if ns != "" {
			namespaces = append(namespaces, ns)
		}
	}
	return namespaces
}

// parseDuration parses a duration string, falling back to the given
// default if it's empty or invalid.
func parseDuration(raw string, fallback time.Duration) time.Duration {
	d, err := time.ParseDuration(raw)
	if err != nil {
		return fallback
	}
	return d
}
