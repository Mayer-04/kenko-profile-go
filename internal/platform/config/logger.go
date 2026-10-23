package config

import (
	"log/slog"
	"os"
	"strings"
)

// NewLogger crea el logger centralizado (11.3): un único punto de
// construcción, Structured logs JSON, nivel por LOG_LEVEL.
// Se inyecta por DI en worker, relay, store y handlers.
func NewLogger() *slog.Logger {
	level := slog.LevelInfo
	switch strings.ToLower(strings.TrimSpace(os.Getenv("LOG_LEVEL"))) {
	case "debug":
		level = slog.LevelDebug
	case "warn", "warning":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	h := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	return slog.New(h)
}
