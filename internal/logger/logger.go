package logger

import (
	"context"
	"log/slog"
	"os"
)

// Logger defaults to the standard library's default logger so code paths that
// log before Init() runs (e.g. unit tests) never dereference a nil pointer.
// Init() replaces it with the structured JSON logger used in production.
var Logger = slog.Default()

func Init(service string) {
	Logger = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})).With("service", service)
}

// With returns the logger, initializing with defaults if needed.
func With(ctx context.Context) *slog.Logger {
	if Logger == nil {
		Init("resume-customizer")
	}
	return Logger
}
