package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/mattjoyce/senechal-gw/internal/auth"
	"github.com/mattjoyce/senechal-gw/internal/plugin"
	"github.com/mattjoyce/senechal-gw/internal/queue"
	"github.com/mattjoyce/senechal-gw/internal/router"
)

// JobQueuer defines the interface for job queue operations
type JobQueuer interface {
	Enqueue(ctx context.Context, req queue.EnqueueRequest) (string, error)
	GetJobByID(ctx context.Context, jobID string) (*queue.JobResult, error)
	GetJobTree(ctx context.Context, rootJobID string) ([]*queue.JobResult, error)
	Depth(ctx context.Context) (int, error)
}

// TreeWaiter defines the interface for waiting on job tree completion
type TreeWaiter interface {
	WaitForJobTree(ctx context.Context, rootJobID string, timeout time.Duration) ([]*queue.JobResult, error)
}

// PipelineRouter defines the interface for looking up pipelines
type PipelineRouter interface {
	GetPipelineByTrigger(trigger string) *router.PipelineInfo
}

// PluginRegistry defines the interface for plugin operations
type PluginRegistry interface {
	Get(name string) (*plugin.Plugin, bool)
	All() map[string]*plugin.Plugin
}

// Config holds API server configuration
type Config struct {
	Listen string
	// APIKey is the legacy single bearer token (admin/full access).
	APIKey string
	// Tokens is an optional list of scoped bearer tokens.
	Tokens            []auth.TokenConfig
	MaxConcurrentSync int
	MaxSyncTimeout    time.Duration
}

// Server represents the HTTP API server
type Server struct {
	config        Config
	queue         JobQueuer
	registry      PluginRegistry
	router        PipelineRouter
	waiter        TreeWaiter
	logger        *slog.Logger
	server        *http.Server
	startedAt     time.Time
	events        *EventHub
	syncSemaphore chan struct{}
}

// New creates a new API server instance
func New(config Config, queue JobQueuer, registry PluginRegistry, router PipelineRouter, waiter TreeWaiter, logger *slog.Logger) *Server {
	if config.MaxConcurrentSync <= 0 {
		config.MaxConcurrentSync = 10
	}
	return &Server{
		config:        config,
		queue:         queue,
		registry:      registry,
		router:        router,
		waiter:        waiter,
		logger:        logger,
		startedAt:     time.Now(),
		events:        NewEventHub(256),
		syncSemaphore: make(chan struct{}, config.MaxConcurrentSync),
	}
}

// Start starts the HTTP server (blocking)
func (s *Server) Start(ctx context.Context) error {
	router := s.setupRoutes()

	s.server = &http.Server{
		Addr:         s.config.Listen,
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Minute, // Increased to support synchronous pipelines
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

	// Routes
	// Unauthenticated ops endpoint.
	r.Get("/healthz", s.handleHealthz)

	// Protected API.
	r.Group(func(r chi.Router) {
		r.Use(s.authMiddleware)
		r.With(s.requireScopes("plugin:ro", "plugin:rw", "*")).Post("/trigger/{plugin}/{command}", s.handleTrigger)
		r.With(s.requireScopes("jobs:ro", "jobs:rw", "*")).Get("/job/{jobID}", s.handleGetJob)
		r.With(s.requireScopes("events:ro", "events:rw", "*")).Get("/events", s.handleEvents)
	})

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
