// Package config loads the Go agent's runtime configuration from
// environment variables.
package config

import "os"

// Config holds the agent's runtime settings.
type Config struct {
	// Port is the TCP port the REST API listens on.
	Port string
	// LogLevel is the minimum level logged (debug, info, warn, error).
	LogLevel string
}

// Load reads configuration from environment variables, falling back to
// defaults for anything unset.
func Load() Config {
	return Config{
		Port:     getEnv("PORT", "8080"),
		LogLevel: getEnv("LOG_LEVEL", "info"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
