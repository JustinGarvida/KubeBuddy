// Package logging configures the agent's structured (slog) logger.
package logging

import (
	"log/slog"
	"os"
	"strings"
)

// New builds the agent's structured logger.
//
// Purpose: builds a JSON slog.Logger writing to stdout at the given
// level. Unrecognized levels fall back to info.
// Params:
//   - level: the minimum level to log ("debug", "info", "warn", or
//     "error", case-insensitive).
//
// Returns: a *slog.Logger writing JSON to stdout.
func New(level string) *slog.Logger {
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: parseLevel(level),
	})
	return slog.New(handler)
}

// parseLevel maps a level name to a slog.Level.
//
// Purpose: converts the configured log-level string into the
// slog.Level New needs.
// Params:
//   - level: the level name ("debug", "info", "warn", or "error",
//     case-insensitive).
//
// Returns: the matching slog.Level, or slog.LevelInfo if level is
// unrecognized.
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
