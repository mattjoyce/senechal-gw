package log

import (
	"log/slog"
	"os"
	"strings"
	"sync"
)

var (
	once   sync.Once
	logger *slog.Logger
)

// Setup initializes the global logger.
// logic: default to INFO. If level is invalid, fallback to INFO.
func Setup(level string) {
	once.Do(func() {
		var l slog.Level
		switch strings.ToUpper(level) {
		case "DEBUG":
			l = slog.LevelDebug
		case "WARN":
			l = slog.LevelWarn
		case "ERROR":
			l = slog.LevelError
		default:
			l = slog.LevelInfo
		}

		opts := &slog.HandlerOptions{
			Level: l,
		}
		handler := slog.NewJSONHandler(os.Stdout, opts)
		logger = slog.New(handler)
		slog.SetDefault(logger)
	})
}

// Get returns the configured logger, or a default one if Setup hasn't been called.
func Get() *slog.Logger {
	if logger == nil {
		Setup("INFO")
	}
	return logger
}

// WithComponent returns a logger with the component field set.
func WithComponent(name string) *slog.Logger {
	return Get().With(slog.String("component", name))
}

