// Package logging define el logger del servicio en un único punto y lo
// inyecta por dependencias (sección 11.3).
package logging

import (
	"log/slog"
	"os"
	"strings"
	"time"
)

// New crea el logger estructurado del servicio. El nivel se toma de la
// configuración (info por defecto) y los instantes se registran en UTC.
func New(level string) *slog.Logger {
	var slogLevel slog.Level
	switch strings.ToLower(level) {
	case "debug":
		slogLevel = slog.LevelDebug
	case "warn":
		slogLevel = slog.LevelWarn
	case "error":
		slogLevel = slog.LevelError
	default:
		slogLevel = slog.LevelInfo
	}

	handler := &slog.HandlerOptions{
		Level: slogLevel,
		ReplaceAttr: func(_ []string, attr slog.Attr) slog.Attr {
			if attr.Key == slog.TimeKey {
				if instant, ok := attr.Value.Any().(time.Time); ok {
					attr.Value = slog.TimeValue(instant.UTC())
				}
			}
			return attr
		},
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, handler))
}
