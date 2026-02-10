package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/mattjoyce/senechal-gw/internal/api"
	"github.com/mattjoyce/senechal-gw/internal/config"
	"github.com/mattjoyce/senechal-gw/internal/dispatch"
	"github.com/mattjoyce/senechal-gw/internal/lock"
	"github.com/mattjoyce/senechal-gw/internal/log"
	"github.com/mattjoyce/senechal-gw/internal/plugin"
	"github.com/mattjoyce/senechal-gw/internal/queue"
	"github.com/mattjoyce/senechal-gw/internal/scheduler"
	"github.com/mattjoyce/senechal-gw/internal/state"
	"github.com/mattjoyce/senechal-gw/internal/storage"
	"github.com/mattjoyce/senechal-gw/internal/webhook"
)

const version = "0.1.0-mvp"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]
	switch command {
	case "start":
		os.Exit(runStart(os.Args[2:]))
	case "config":
		os.Exit(runConfig(os.Args[2:]))
	case "version":
		fmt.Printf("senechal-gw version %s\n", version)
		os.Exit(0)
	case "help", "--help", "-h":
		printUsage()
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Print(`senechal-gw - Lightweight YAML-configured integration gateway

Usage:
  senechal-gw start [flags]         Start the service in foreground
  senechal-gw config hash-update    Regenerate .checksums for scope files
  senechal-gw version               Show version information
  senechal-gw help                  Show this help message

Flags for 'start':
  --config PATH                     Path to config file or directory
                                    (default: auto-discover from standard locations)

Config file discovery order:
  1. --config flag (if provided)
  2. $SENECHAL_CONFIG_DIR environment variable
  3. ~/.config/senechal-gw/ (multi-file mode)
  4. /etc/senechal-gw/ (multi-file mode)
  5. ./config.yaml (legacy single-file mode)

Examples:
  senechal-gw start
  senechal-gw start --config ~/.config/senechal-gw
  senechal-gw start --config /etc/senechal/config.yaml  # legacy single-file
  senechal-gw config hash-update --config-dir ~/.config/senechal-gw

`)
}

func runStart(args []string) int {
	// Parse flags
	fs := flag.NewFlagSet("start", flag.ExitOnError)
	configPath := fs.String("config", "", "Path to configuration file or directory")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to parse flags: %v\n", err)
		return 1
	}

	// Discover config if not specified
	if *configPath == "" {
		discovered, err := config.DiscoverConfigDir()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to discover config: %v\n", err)
			return 1
		}
		*configPath = discovered
		fmt.Fprintf(os.Stderr, "Using discovered config: %s\n", *configPath)
	}

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		return 1
	}

	// Setup logging
	log.Setup(cfg.Service.LogLevel)
	logger := log.WithComponent("main")
	logger.Info("senechal-gw starting", "version", version, "config", *configPath)

	// Acquire PID lock for single-instance enforcement
	pidLockPath := getPIDLockPath(cfg)
	pidLock, err := lock.AcquirePIDLock(pidLockPath)
	if err != nil {
		logger.Error("failed to acquire PID lock (another instance may be running)", "path", pidLockPath, "error", err)
		return 1
	}
	defer pidLock.Release()
	logger.Info("acquired PID lock", "path", pidLockPath)

	// Open SQLite database
	ctx := context.Background()
	db, err := storage.OpenSQLite(ctx, cfg.State.Path)
	if err != nil {
		logger.Error("failed to open database", "path", cfg.State.Path, "error", err)
		return 1
	}
	defer db.Close()
	logger.Info("database opened", "path", cfg.State.Path)

	// Initialize queue and state store
	q := queue.New(db)
	st := state.NewStore(db)

	// Discover plugins
	registry, err := plugin.Discover(cfg.PluginsDir, func(level, msg string, args ...interface{}) {
		// Simple logger wrapper for plugin discovery
		switch level {
		case "debug":
			logger.Debug(msg, args...)
		case "info":
			logger.Info(msg, args...)
		case "warn":
			logger.Warn(msg, args...)
		case "error":
			logger.Error(msg, args...)
		}
	})
	if err != nil {
		logger.Error("plugin discovery failed", "plugins_dir", cfg.PluginsDir, "error", err)
		return 1
	}
	logger.Info("plugin discovery complete", "count", len(registry.All()))

	// Create scheduler and dispatcher
	sched := scheduler.New(cfg, q, logger)
	disp := dispatch.New(q, st, registry, cfg)

	// Setup graceful shutdown on SIGINT/SIGTERM
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Start scheduler and dispatcher in goroutines
	errCh := make(chan error, 3)

	go func() {
		if err := sched.Start(ctx); err != nil && err != context.Canceled {
			errCh <- fmt.Errorf("scheduler: %w", err)
		}
	}()

	go func() {
		if err := disp.Start(ctx); err != nil && err != context.Canceled {
			errCh <- fmt.Errorf("dispatcher: %w", err)
		}
	}()

	// Start API server if enabled
	if cfg.API.Enabled {
		if cfg.API.Auth.APIKey == "" {
			logger.Warn("API server enabled but api_key is empty - this is insecure!")
		}
		apiConfig := api.Config{
			Listen: cfg.API.Listen,
			APIKey: cfg.API.Auth.APIKey,
		}
		apiServer := api.New(apiConfig, q, registry, log.WithComponent("api"))
		go func() {
			if err := apiServer.Start(ctx); err != nil && err != context.Canceled {
				errCh <- fmt.Errorf("api: %w", err)
			}
		}()
		logger.Info("API server enabled", "listen", cfg.API.Listen)
	} else {
		logger.Info("API server disabled")
	}

	// Start webhook server if configured
	if cfg.Webhooks != nil && len(cfg.Webhooks.Endpoints) > 0 {
		// Convert config and resolve secrets
		webhookConfig, err := webhook.FromGlobalConfig(cfg.Webhooks, make(map[string]string))
		if err != nil {
			logger.Error("failed to configure webhooks", "error", err)
			return 1
		}

		webhookServer := webhook.New(webhookConfig, q, log.WithComponent("webhook"))
		go func() {
			if err := webhookServer.Start(ctx); err != nil && err != context.Canceled {
				errCh <- fmt.Errorf("webhook: %w", err)
			}
		}()
		logger.Info("webhook server enabled", "listen", webhookConfig.Listen, "endpoints", len(webhookConfig.Endpoints))
	} else {
		logger.Info("webhook server disabled")
	}

	logger.Info("senechal-gw running (press Ctrl+C to stop)")

	// Wait for shutdown signal or error
	select {
	case sig := <-sigCh:
		logger.Info("received shutdown signal", "signal", sig)
		cancel()
	case err := <-errCh:
		logger.Error("component failed", "error", err)
		cancel()
		return 1
	}

	logger.Info("senechal-gw stopped")
	return 0
}

// runConfig handles config subcommands.
func runConfig(args []string) int {
	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "Usage: senechal-gw config hash-update --config-dir PATH\n")
		return 1
	}

	subcommand := args[0]
	switch subcommand {
	case "hash-update":
		return runConfigHashUpdate(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "Unknown config subcommand: %s\n", subcommand)
		return 1
	}
}

// runConfigHashUpdate regenerates .checksums for scope files.
func runConfigHashUpdate(args []string) int {
	fs := flag.NewFlagSet("hash-update", flag.ExitOnError)
	configDir := fs.String("config-dir", "", "Path to config directory")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to parse flags: %v\n", err)
		return 1
	}

	// Discover config dir if not specified
	if *configDir == "" {
		discovered, err := config.DiscoverConfigDir()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to discover config directory: %v\n", err)
			return 1
		}

		// Check if discovered path is a directory
		info, err := os.Stat(discovered)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to stat config path: %v\n", err)
			return 1
		}

		if !info.IsDir() {
			fmt.Fprintf(os.Stderr, "Error: hash-update requires multi-file config mode (directory), but found single-file config: %s\n", discovered)
			fmt.Fprintf(os.Stderr, "Hint: Migrate to multi-file config first.\n")
			return 1
		}

		*configDir = discovered
	}

	// Generate checksums for scope files
	scopeFiles := []string{"tokens.yaml", "webhooks.yaml"}
	if err := config.GenerateChecksums(*configDir, scopeFiles); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to generate checksums: %v\n", err)
		return 1
	}

	fmt.Printf("Successfully generated .checksums for scope files in %s\n", *configDir)
	return 0
}

// getPIDLockPath returns the PID lock file path.
// Derives it from the database path if not explicitly configured.
func getPIDLockPath(cfg *config.Config) string {
	// Use the same directory as the state database, with .pid extension
	dbPath := cfg.State.Path
	dbDir := filepath.Dir(dbPath)
	dbBase := filepath.Base(dbPath)

	// Remove extension and add .pid
	ext := filepath.Ext(dbBase)
	nameWithoutExt := dbBase[:len(dbBase)-len(ext)]

	return filepath.Join(dbDir, nameWithoutExt+".pid")
}
