package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/mattjoyce/senechal-gw/internal/plugin"
	"github.com/mattjoyce/senechal-gw/internal/queue"
)

// JobQueuer defines the interface for job queue operations
type JobQueuer interface {
	Enqueue(ctx context.Context, req queue.EnqueueRequest) (string, error)
	GetJobByID(ctx context.Context, jobID string) (*queue.JobResult, error)
}

// PluginRegistry defines the interface for plugin operations
type PluginRegistry interface {
	Get(name string) (*plugin.Plugin, bool)
}

// Config holds API server configuration
type Config struct {
	Listen string
	APIKey string
}

// Server represents the HTTP API server
type Server struct {
	config   Config
	queue    JobQueuer
	registry PluginRegistry
	logger   *slog.Logger
	server   *http.Server
}

// New creates a new API server instance
func New(config Config, queue JobQueuer, registry PluginRegistry, logger *slog.Logger) *Server {
	return &Server{
		config:   config,
		queue:    queue,
		registry: registry,
		logger:   logger,
	}
}

// Start starts the HTTP server (blocking)
func (s *Server) Start(ctx context.Context) error {
	router := s.setupRoutes()

	s.server = &http.Server{
		Addr:         s.config.Listen,
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	s.logger.Info("API server starting", "listen", s.config.Listen)

	// Run server in a goroutine
	errCh := make(chan error, 1)
	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	// Wait for context cancellation or server error
	select {
	case <-ctx.Done():
		s.logger.Info("API server shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("server shutdown failed: %w", err)
		}
		return ctx.Err()
	case err := <-errCh:
		return fmt.Errorf("server error: %w", err)
	}
}

// setupRoutes configures the HTTP router
func (s *Server) setupRoutes() *chi.Mux {
	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(s.loggingMiddleware)
	r.Use(middleware.Recoverer)

	// Auth middleware for all routes
	r.Use(s.authMiddleware)

	// Routes
	r.Post("/trigger/{plugin}/{command}", s.handleTrigger)
	r.Get("/job/{jobID}", s.handleGetJob)

	return r
}

// loggingMiddleware logs HTTP requests
func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		s.logger.Info("http request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", ww.Status(),
			"duration_ms", time.Since(start).Milliseconds(),
			"request_id", middleware.GetReqID(r.Context()),
		)
	})
}
