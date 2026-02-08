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

// WithPlugin returns a logger with the plugin field set.
func WithPlugin(name string) *slog.Logger {
	return Get().With(slog.String("plugin", name))
}

// WithJob returns a logger with the job_id field set.
func WithJob(id string) *slog.Logger {
	return Get().With(slog.String("job_id", id))
}

// Info logs at INFO level.
func Info(msg string, args ...any) {
	Get().Info(msg, args...)
}

// Debug logs at DEBUG level.
func Debug(msg string, args ...any) {
	Get().Debug(msg, args...)
}

// Warn logs at WARN level.
func Warn(msg string, args ...any) {
	Get().Warn(msg, args...)
}

// Error logs at ERROR level.
func Error(msg string, args ...any) {
	Get().Error(msg, args...)
}
