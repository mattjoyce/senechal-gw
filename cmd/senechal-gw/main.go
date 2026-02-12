package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/mattjoyce/senechal-gw/internal/api"
	"github.com/mattjoyce/senechal-gw/internal/auth"
	"github.com/mattjoyce/senechal-gw/internal/config"
	"github.com/mattjoyce/senechal-gw/internal/dispatch"
	"github.com/mattjoyce/senechal-gw/internal/doctor"
	"github.com/mattjoyce/senechal-gw/internal/inspect"
	"github.com/mattjoyce/senechal-gw/internal/lock"
	"github.com/mattjoyce/senechal-gw/internal/log"
	"github.com/mattjoyce/senechal-gw/internal/plugin"
	"github.com/mattjoyce/senechal-gw/internal/queue"
	"github.com/mattjoyce/senechal-gw/internal/router"
	"github.com/mattjoyce/senechal-gw/internal/scheduler"
	"github.com/mattjoyce/senechal-gw/internal/state"
	"github.com/mattjoyce/senechal-gw/internal/storage"
	"github.com/mattjoyce/senechal-gw/internal/webhook"
	"github.com/mattjoyce/senechal-gw/internal/workspace"
	"gopkg.in/yaml.v3"
)

const version = "0.1.0-mvp"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	switch cmd {
	// --- NOUNS ---
	case "system":
		os.Exit(runSystemNoun(args))
	case "config":
		os.Exit(runConfigNoun(args))
	case "job":
		os.Exit(runJobNoun(args))
	case "plugin":
		os.Exit(runPluginNoun(args))

	// --- ROOT ALIASES (Backward Compatibility) ---
	case "start":
		os.Exit(runStart(args))
	case "inspect":
		os.Exit(runInspect(args))
	case "doctor": // Alias for backward compat with Claude's branch
		os.Exit(runConfigCheck(args))
	case "version":
		fmt.Printf("senechal-gw version %s\n", version)
		os.Exit(0)
	case "help", "--help", "-h":
		printUsage()
		os.Exit(0)

	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", cmd)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Print(`senechal-gw - Lightweight YAML-configured integration gateway

Usage:
  senechal-gw <noun> <action> [flags]

Core Resources (Nouns):
  system    Gateway lifecycle and health
  config    System configuration and integrity
  job       Execution instances and lineage
  plugin    Capability discovery and management

System Commands:
  system start      Start the gateway service in foreground
  system status     Show global gateway health (planned)

Config Commands:
  config lock       Authorize current state (update integrity hashes)
  config check      Validate syntax, policy, and integrity

Job Commands:
  job inspect <id>  Show lineage, baggage, and workspace artifacts

Plugin Commands:
  plugin list       Show discovered plugins (planned)
  plugin run <name> Manual execution (planned)

General:
  version           Show version information
  help              Show this help message

Use 'senechal-gw <noun> help' for resource-specific flags.
`)
}

// --- NOUN DISPATCHERS ---

func runSystemNoun(args []string) int {
	if len(args) < 1 {
		printSystemNounHelp(os.Stderr)
		return 1
	}
	if isHelpToken(args[0]) {
		printSystemNounHelp(os.Stdout)
		return 0
	}

	action := args[0]
	actionArgs := args[1:]

	switch action {
	case "start":
		if hasHelpFlag(actionArgs) {
			printSystemStartHelp()
			return 0
		}
		return runStart(actionArgs)
	case "status":
		if hasHelpFlag(actionArgs) {
			printSystemStatusHelp()
			return 0
		}
		fmt.Println("system status is not yet implemented")
		return 0
	case "help":
		printSystemNounHelp(os.Stdout)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "Unknown system action: %s\n", action)
		return 1
	}
}

func runConfigNoun(args []string) int {
	if len(args) < 1 {
		printConfigNounHelp(os.Stderr)
		return 1
	}

	if isHelpToken(args[0]) {
		printConfigNounHelp(os.Stdout)
		return 0
	}

	action := args[0]
	actionArgs := args[1:]

	switch action {
	case "lock", "hash-update": // Alias for backward compat
		if hasHelpFlag(actionArgs) {
			printConfigLockHelp()
			return 0
		}
		return runConfigHashUpdate(actionArgs)
	case "check":
		if hasHelpFlag(actionArgs) {
			printConfigCheckHelp()
			return 0
		}
		return runConfigCheck(actionArgs)
	case "show":
		if hasHelpFlag(actionArgs) {
			printConfigShowHelp()
			return 0
		}
		return runConfigShow(actionArgs)
	case "get":
		if hasHelpFlag(actionArgs) {
			printConfigGetHelp()
			return 0
		}
		return runConfigGet(actionArgs)
	case "set":
		if hasHelpFlag(actionArgs) {
			printConfigSetHelp()
			return 0
		}
		return runConfigSet(actionArgs)
	case "help":
		printConfigNounHelp(os.Stdout)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "Unknown config action: %s\n", action)
		return 1
	}
}

// ... (skipping to action implementations)

func runConfigSet(args []string) int {
	var configPath string
	var dryRun, apply bool

	fs := flag.NewFlagSet("set", flag.ContinueOnError)
	fs.StringVar(&configPath, "config", "", "Path to configuration")
	fs.BoolVar(&dryRun, "dry-run", false, "Preview changes")
	fs.BoolVar(&apply, "apply", false, "Apply changes")

	var kvPair string
	var remainingArgs []string
	for _, arg := range args {
		if !strings.HasPrefix(arg, "-") && strings.Contains(arg, "=") && kvPair == "" {
			kvPair = arg
		} else {
			remainingArgs = append(remainingArgs, arg)
		}
	}

	if err := fs.Parse(remainingArgs); err != nil {
		fmt.Fprintf(os.Stderr, "Flag error: %v\n", err)
		return 1
	}

	if kvPair == "" {
		fmt.Fprintf(os.Stderr, "Usage: senechal-gw config set <path>=<value> [--dry-run | --apply]\n")
		return 1
	}

	if !dryRun && !apply {
		fmt.Println("Error: either --dry-run or --apply must be specified for 'config set'.")
		return 1
	}

	parts := strings.SplitN(kvPair, "=", 2)
	path, value := parts[0], parts[1]

	cfg, err := loadConfigForTool(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Load error: %v\n", err)
		return 1
	}

	if dryRun {
		// In-memory test without persistence
		err := cfg.SetPath(path, value, false)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Dry-run validation failed: %v\n", err)
			return 1
		}
		fmt.Printf("Dry-run: would set %q to %q\n", path, value)
		fmt.Println("Status: Configuration check PASSED.")
		return 0
	}

	// Real application
	if err := cfg.SetPath(path, value, true); err != nil {
		fmt.Fprintf(os.Stderr, "Apply failed: %v\n", err)
		return 1
	}

	fmt.Printf("Successfully set %q to %q\n", path, value)
	return 0
}

// ... (skipping to action implementations)

func runConfigShow(args []string) int {
	fs := flag.NewFlagSet("show", flag.ExitOnError)
	configPath := fs.String("config", "", "Path to configuration file or directory")
	jsonOut := fs.Bool("json", false, "Output in structured JSON format")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to parse flags: %v\n", err)
		return 1
	}

	cfg, err := loadConfigForTool(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Load error: %v\n", err)
		return 1
	}

	var result any = cfg
	if fs.NArg() > 0 {
		entity := fs.Arg(0)
		res, err := cfg.GetPath(entity)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return 1
		}
		result = res
	}

	if *jsonOut {
		data, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(data))
	} else {
		data, _ := yaml.Marshal(result)
		fmt.Print(string(data))
	}
	return 0
}

func runConfigGet(args []string) int {
	fs := flag.NewFlagSet("get", flag.ExitOnError)
	configPath := fs.String("config", "", "Path to configuration file or directory")
	jsonOut := fs.Bool("json", false, "Output in structured JSON format")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to parse flags: %v\n", err)
		return 1
	}

	if fs.NArg() != 1 {
		fmt.Fprintf(os.Stderr, "Usage: senechal-gw config get <path> [--json]\n")
		return 1
	}
	path := fs.Arg(0)

	cfg, err := loadConfigForTool(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Load error: %v\n", err)
		return 1
	}

	val, err := cfg.GetPath(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	if *jsonOut {
		data, _ := json.MarshalIndent(val, "", "  ")
		fmt.Println(string(data))
	} else {
		fmt.Printf("%v\n", val)
	}
	return 0
}

func loadConfigForTool(configPath string) (*config.Config, error) {
	if configPath == "" {
		discovered, err := config.DiscoverConfigDir()
		if err != nil {
			return nil, err
		}
		configPath = discovered
	}
	return config.Load(configPath)
}

func runJobNoun(args []string) int {
	if len(args) < 1 {
		printJobNounHelp(os.Stderr)
		return 1
	}

	if isHelpToken(args[0]) {
		printJobNounHelp(os.Stdout)
		return 0
	}

	action := args[0]
	actionArgs := args[1:]

	switch action {
	case "inspect":
		if hasHelpFlag(actionArgs) {
			printJobInspectHelp()
			return 0
		}
		return runInspect(actionArgs)
	case "help":
		printJobNounHelp(os.Stdout)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "Unknown job action: %s\n", action)
		return 1
	}
}

func runPluginNoun(args []string) int {
	if len(args) < 1 {
		printPluginNounHelp(os.Stderr)
		return 1
	}

	if isHelpToken(args[0]) {
		printPluginNounHelp(os.Stdout)
		return 0
	}

	action := args[0]
	actionArgs := args[1:]

	switch action {
	case "list":
		if hasHelpFlag(actionArgs) {
			printPluginListHelp()
			return 0
		}
		fmt.Println("plugin list is not yet implemented")
		return 0
	case "run":
		if hasHelpFlag(actionArgs) {
			printPluginRunHelp()
			return 0
		}
		fmt.Println("plugin run is not yet implemented")
		return 0
	case "help":
		printPluginNounHelp(os.Stdout)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "Unknown plugin action: %s\n", action)
		return 1
	}
}

func isHelpToken(token string) bool {
	return token == "help" || token == "--help" || token == "-h"
}

func hasHelpFlag(args []string) bool {
	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			return true
		}
	}
	return false
}

func printSystemNounHelp(w *os.File) {
	fmt.Fprintln(w, "Usage: senechal-gw system <action>")
	fmt.Fprintln(w, "Actions: start, status")
}

func printConfigNounHelp(w *os.File) {
	fmt.Fprintln(w, "Usage: senechal-gw config <action> [flags]")
	fmt.Fprintln(w, "Actions: lock, check, show, get, set")
}

func printJobNounHelp(w *os.File) {
	fmt.Fprintln(w, "Usage: senechal-gw job <action>")
	fmt.Fprintln(w, "Actions: inspect")
}

func printPluginNounHelp(w *os.File) {
	fmt.Fprintln(w, "Usage: senechal-gw plugin <action>")
	fmt.Fprintln(w, "Actions: list, run")
}

func printSystemStartHelp() {
	fmt.Println("Usage: senechal-gw system start [--config PATH]")
	fmt.Println("Start the gateway service in the foreground.")
}

func printSystemStatusHelp() {
	fmt.Println("Usage: senechal-gw system status")
	fmt.Println("Show global gateway health (planned).")
}

func printConfigLockHelp() {
	fmt.Println("Usage: senechal-gw config lock [--config PATH | --config-dir PATH] [-v|--verbose] [--dry-run]")
	fmt.Println("Authorize current configuration state by regenerating scope file integrity hashes.")
}

func printConfigCheckHelp() {
	fmt.Println("Usage: senechal-gw config check [--config PATH] [--format human|json] [--strict] [--json]")
	fmt.Println("Validate configuration syntax, policy, and integrity.")
}

func printConfigShowHelp() {
	fmt.Println("Usage: senechal-gw config show [entity] [--config PATH] [--json]")
	fmt.Println("Show full resolved configuration or a filtered entity node.")
}

func printConfigGetHelp() {
	fmt.Println("Usage: senechal-gw config get <path> [--config PATH] [--json]")
	fmt.Println("Read a single value from the resolved configuration.")
}

func printConfigSetHelp() {
	fmt.Println("Usage: senechal-gw config set <path>=<value> [--config PATH] [--dry-run | --apply]")
	fmt.Println("Set a configuration value with either preview or apply mode.")
}

func printJobInspectHelp() {
	fmt.Println("Usage: senechal-gw job inspect <job_id> [--config PATH] [--json]")
	fmt.Println("Inspect job lineage, baggage, and workspace artifacts.")
}

func printPluginListHelp() {
	fmt.Println("Usage: senechal-gw plugin list")
	fmt.Println("Show discovered plugins (planned).")
}

func printPluginRunHelp() {
	fmt.Println("Usage: senechal-gw plugin run <name>")
	fmt.Println("Execute a plugin manually (planned).")
}

// --- ACTION IMPLEMENTATIONS ---

func runStart(args []string) int {
	fs := flag.NewFlagSet("start", flag.ExitOnError)
	configPath := fs.String("config", "", "Path to configuration file or directory")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to parse flags: %v\n", err)
		return 1
	}

	if *configPath == "" {
		discovered, err := config.DiscoverConfigDir()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to discover config: %v\n", err)
			return 1
		}
		*configPath = discovered
		fmt.Fprintf(os.Stderr, "Using discovered config: %s\n", *configPath)
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		return 1
	}

	log.Setup(cfg.Service.LogLevel)
	logger := log.WithComponent("main")
	logger.Info("senechal-gw starting", "version", version, "config", *configPath)

	pidLockPath := getPIDLockPath(cfg)
	pidLock, err := lock.AcquirePIDLock(pidLockPath)
	if err != nil {
		logger.Error("failed to acquire PID lock (another instance may be running)", "path", pidLockPath, "error", err)
		return 1
	}
	defer pidLock.Release()
	logger.Info("acquired PID lock", "path", pidLockPath)

	ctx := context.Background()
	db, err := storage.OpenSQLite(ctx, cfg.State.Path)
	if err != nil {
		logger.Error("failed to open database", "path", cfg.State.Path, "error", err)
		return 1
	}
	defer db.Close()
	logger.Info("database opened", "path", cfg.State.Path)

	q := queue.New(db)
	st := state.NewStore(db)
	contextStore := state.NewContextStore(db)

	registry, err := plugin.Discover(cfg.PluginsDir, func(level, msg string, args ...interface{}) {
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

	configDir := *configPath
	if stat, err := os.Stat(configDir); err != nil || !stat.IsDir() {
		configDir = filepath.Dir(*configPath)
	}

	wsBaseDir := filepath.Join(filepath.Dir(cfg.State.Path), "workspaces")
	wsManager, err := workspace.NewFSManager(wsBaseDir)
	if err != nil {
		logger.Error("failed to initialize workspace manager", "base_dir", wsBaseDir, "error", err)
		return 1
	}

	routerEngine, err := router.LoadFromConfigDir(configDir, registry)
	if err != nil {
		logger.Error("failed to load router pipelines", "config_dir", configDir, "error", err)
		return 1
	}

	pipelines := routerEngine.PipelineSummary()
	logger.Info("pipeline discovery complete", "config_dir", configDir, "pipelines_loaded", len(pipelines))
	for _, p := range pipelines {
		logger.Info("pipeline registered", "name", p.Name, "trigger", p.Trigger)
	}

	sched := scheduler.New(cfg, q, logger)
	disp := dispatch.New(q, st, contextStore, wsManager, routerEngine, registry, cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

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

	if cfg.API.Enabled {
		tokens := make([]auth.TokenConfig, 0, len(cfg.API.Auth.Tokens))
		for _, t := range cfg.API.Auth.Tokens {
			tokens = append(tokens, auth.TokenConfig{
				Token:  t.Token,
				Scopes: t.Scopes,
			})
		}
		apiConfig := api.Config{
			Listen: cfg.API.Listen,
			APIKey: cfg.API.Auth.APIKey,
			Tokens: tokens,
		}
		apiServer := api.New(apiConfig, q, registry, log.WithComponent("api"))
		go func() {
			if err := apiServer.Start(ctx); err != nil && err != context.Canceled {
				errCh <- fmt.Errorf("api: %w", err)
			}
		}()
		logger.Info("API server enabled", "listen", cfg.API.Listen)
	}

	if cfg.Webhooks != nil && len(cfg.Webhooks.Endpoints) > 0 {
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
	}

	logger.Info("senechal-gw running (press Ctrl+C to stop)")

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

func runInspect(args []string) int {
	// Custom flag parsing because we want to support flags AFTER the job ID
	// like 'senechal-gw job inspect <id> --json'
	var configPath string
	var jsonOut bool

	// Create a new flag set but don't parse everything at once
	fs := flag.NewFlagSet("inspect", flag.ContinueOnError)
	fs.StringVar(&configPath, "config", "", "Path to configuration")
	fs.BoolVar(&jsonOut, "json", false, "Output report in JSON")

	// Filter out positional jobID and then parse remaining flags
	var jobID string
	var remainingArgs []string
	for _, arg := range args {
		if !strings.HasPrefix(arg, "-") && jobID == "" {
			jobID = arg
		} else {
			remainingArgs = append(remainingArgs, arg)
		}
	}

	if err := fs.Parse(remainingArgs); err != nil {
		fmt.Fprintf(os.Stderr, "Flag error: %v\n", err)
		return 1
	}

	if jobID == "" {
		fmt.Fprintf(os.Stderr, "Usage: senechal-gw job inspect <job_id> [--config PATH] [--json]\n")
		return 1
	}

	if configPath == "" {
		discovered, err := config.DiscoverConfigDir()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to discover config: %v\n", err)
			return 1
		}
		configPath = discovered
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		return 1
	}

	db, err := storage.OpenSQLite(context.Background(), cfg.State.Path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open database: %v\n", err)
		return 1
	}
	defer db.Close()

	var report string
	if jsonOut {
		report, err = inspect.BuildJSONReport(context.Background(), db, cfg.State.Path, jobID)
	} else {
		report, err = inspect.BuildReport(context.Background(), db, cfg.State.Path, jobID)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Inspect failed: %v\n", err)
		return 1
	}

	fmt.Print(report)
	return 0
}

func runConfigCheck(args []string) int {
	var configPath string
	var strict, jsonOut bool
	var format string

	fs := flag.NewFlagSet("check", flag.ExitOnError)
	fs.StringVar(&configPath, "config", "", "Path to configuration")
	fs.BoolVar(&strict, "strict", false, "Treat warnings as errors")
	fs.StringVar(&format, "format", "human", "Output format (human, json)")
	// Handle -json alias for format=json
	fs.BoolVar(&jsonOut, "json", false, "Output in JSON")

	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "Flag error: %v\n", err)
		return 1
	}

	if jsonOut {
		format = "json"
	}

	if configPath == "" {
		discovered, err := config.DiscoverConfigDir()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to discover config: %v\n", err)
			return 1
		}
		configPath = discovered
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Config load error: %v\n", err)
		return 1
	}

	registry, err := plugin.Discover(cfg.PluginsDir, func(level, msg string, args ...interface{}) {})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Plugin discovery error: %v\n", err)
		return 1
	}

	doc := doctor.New(cfg, registry)
	result := doc.Validate()

	switch format {
	case "json":
		out, err := doctor.FormatJSON(result)
		if err != nil {
			fmt.Fprintf(os.Stderr, "JSON format error: %v\n", err)
			return 1
		}
		fmt.Println(out)
	default:
		fmt.Print(doctor.FormatHuman(result))
	}

	if !result.Valid {
		return 1
	}
	if strict && len(result.Warnings) > 0 {
		return 2
	}
	return 0
}

func runConfigHashUpdate(args []string) int {
	var configPath, configDir string
	var verbose, verboseShort, dryRun bool

	fs := flag.NewFlagSet("lock", flag.ExitOnError)
	fs.StringVar(&configPath, "config", "", "Path to configuration")
	fs.StringVar(&configDir, "config-dir", "", "Path to config directory")
	fs.BoolVar(&verbose, "verbose", false, "Verbose output")
	fs.BoolVar(&verboseShort, "v", false, "Verbose output")
	fs.BoolVar(&dryRun, "dry-run", false, "Dry run")

	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "Flag error: %v\n", err)
		return 1
	}
	isVerbose := verbose || verboseShort

	if configPath != "" && configDir != "" {
		fmt.Fprintf(os.Stderr, "Error: use only one of --config or --config-dir\n")
		return 1
	}

	var targetDirs []string
	if configDir != "" {
		targetDirs = []string{configDir}
	} else {
		resolvedConfigPath := configPath
		if resolvedConfigPath == "" {
			discovered, err := config.DiscoverConfigDir()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to discover config: %v\n", err)
				return 1
			}
			resolvedConfigPath = discovered
		}

		dirs, err := config.DiscoverScopeDirs(resolvedConfigPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to resolve scope directories: %v\n", err)
			return 1
		}
		targetDirs = dirs
	}

	for _, dir := range targetDirs {
		// Check if this is a CONFIG_SPEC directory
		if config.IsConfigSpecDir(dir) {
			files, err := config.DiscoverConfigFiles(dir)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to discover config files in %s: %v\n", dir, err)
				return 1
			}

			if isVerbose {
				fmt.Printf("Processing directory (v2 manifest): %s\n", dir)
				for _, f := range files.AllFiles() {
					tier := "operational"
					if files.FileTier(f) == config.TierHighSecurity {
						tier = "high-security"
					}
					fmt.Printf("  DISCOVER [%s] %s\n", tier, f)
				}
			}

			if err := config.GenerateChecksumsFromDiscovery(files, dryRun); err != nil {
				fmt.Fprintf(os.Stderr, "Failed to lock config in %s: %v\n", dir, err)
				return 1
			}

			if isVerbose {
				if dryRun {
					fmt.Printf("  DRY-RUN .checksums: %s (not written)\n", filepath.Join(dir, ".checksums"))
				} else {
					fmt.Printf("  WROTE .checksums: %s\n", filepath.Join(dir, ".checksums"))
				}
			}
		} else {
			// Legacy include-based mode
			scopeFiles := []string{"tokens.yaml", "webhooks.yaml"}
			report, err := config.GenerateChecksumsWithReport(dir, scopeFiles, dryRun)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to lock config in %s: %v\n", dir, err)
				return 1
			}
			if isVerbose {
				fmt.Printf("Processing directory (v1 manifest): %s\n", dir)
				for _, file := range report.Files {
					if file.Exists {
						fmt.Printf("  HASH %s: %s\n", file.Filename, file.Hash)
						continue
					}
					fmt.Printf("  SKIP %s: not found (optional)\n", file.Filename)
				}
				if dryRun {
					fmt.Printf("  DRY-RUN .checksums: %s (not written)\n", report.ChecksumPath)
				} else {
					fmt.Printf("  WROTE .checksums: %s\n", report.ChecksumPath)
				}
			}
		}
	}

	if dryRun {
		fmt.Printf("Dry run completed for %d directory/ies (no files written):\n", len(targetDirs))
	} else {
		fmt.Printf("Successfully locked configuration in %d directory/ies:\n", len(targetDirs))
	}
	for _, dir := range targetDirs {
		fmt.Printf("  - %s\n", dir)
	}

	return 0
}
func getPIDLockPath(cfg *config.Config) string {
	dbPath := cfg.State.Path
	dbDir := filepath.Dir(dbPath)
	dbBase := filepath.Base(dbPath)
	ext := filepath.Ext(dbBase)
	nameWithoutExt := dbBase[:len(dbBase)-len(ext)]
	return filepath.Join(dbDir, nameWithoutExt+".pid")
}
