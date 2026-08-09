// Package logging configures the agent's structured (slog) logger.
package logging

import (
	"log/slog"
	"os"
	"strings"
)

// New builds a JSON slog.Logger writing to stdout at the given level
// (debug, info, warn, error). Unrecognized levels fall back to info.
func New(level string) *slog.Logger {
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: parseLevel(level),
	})
	return slog.New(handler)
}

func parseLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
