package logger

import (
	"context"
	"log/slog"
	"os"
)

var Logger *slog.Logger

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
