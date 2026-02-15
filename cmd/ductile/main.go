package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattjoyce/ductile/internal/api"
	"github.com/mattjoyce/ductile/internal/auth"
	"github.com/mattjoyce/ductile/internal/config"
	"github.com/mattjoyce/ductile/internal/dispatch"
	"github.com/mattjoyce/ductile/internal/doctor"
	"github.com/mattjoyce/ductile/internal/events"
	"github.com/mattjoyce/ductile/internal/inspect"
	"github.com/mattjoyce/ductile/internal/lock"
	"github.com/mattjoyce/ductile/internal/log"
	"github.com/mattjoyce/ductile/internal/plugin"
	"github.com/mattjoyce/ductile/internal/queue"
	"github.com/mattjoyce/ductile/internal/router"
	"github.com/mattjoyce/ductile/internal/scheduler"
	"github.com/mattjoyce/ductile/internal/state"
	"github.com/mattjoyce/ductile/internal/storage"
	"github.com/mattjoyce/ductile/internal/tui"
	"github.com/mattjoyce/ductile/internal/tui/watch"
	"github.com/mattjoyce/ductile/internal/webhook"
	"github.com/mattjoyce/ductile/internal/workspace"
	"gopkg.in/yaml.v3"
)

var (
	version   = "0.1.0-dev"
	gitCommit = "unknown"
	buildDate = "unknown"
)

func main() {
	os.Exit(runCLI(os.Args[1:]))
}

func runCLI(cliArgs []string) int {
	if len(cliArgs) < 1 {
		printUsage()
		return 1
	}

	cmd := cliArgs[0]
	args := cliArgs[1:]

	if cmd == "--version" {
		return runVersion(args)
	}

	switch cmd {
	// --- NOUNS ---
	case "system":
		return runSystemNoun(args)
	case "config":
		return runConfigNoun(args)
	case "job":
		return runJobNoun(args)
	case "plugin":
		return runPluginNoun(args)
	case "trigger":
		printTriggerHelp()
		return 0

	// --- ROOT ALIASES (Backward Compatibility) ---
	case "start":
		return runStart(args)
	case "reset":
		if hasHelpFlag(args) {
			printSystemResetHelp()
			return 0
		}
		return runSystemReset(args)
	case "inspect":
		return runInspect(args)
	case "doctor": // Alias for backward compat with Claude's branch
		return runConfigCheck(args)
	case "version":
		return runVersion(args)
	case "help", "--help", "-h":
		printUsage()
		return 0

	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", cmd)
		printUsage()
		return 1
	}
}

type versionInfo struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildTime string `json:"build_time"`
}

func runVersion(args []string) int {
	fs := flag.NewFlagSet("version", flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "Output version metadata as JSON")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "Flag error: %v\n", err)
		return 1
	}
	if fs.NArg() > 0 {
		fmt.Fprintln(os.Stderr, "Usage: ductile version [--json]")
		return 1
	}

	info := currentVersionInfo()

	if *jsonOut {
		data, err := json.MarshalIndent(info, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to render version JSON: %v\n", err)
			return 1
		}
		fmt.Println(string(data))
		return 0
	}

	fmt.Printf("ductile %s\n", info.Version)
	fmt.Printf("commit: %s\n", info.Commit)
	fmt.Printf("built_at: %s\n", info.BuildTime)
	return 0
}

func currentVersionInfo() versionInfo {
	info := versionInfo{
		Version:   strings.TrimSpace(version),
		Commit:    "unknown",
		BuildTime: "unknown",
	}

	if info.Version == "" {
		info.Version = "0.0.0-dev"
	}

	resolvedCommit := strings.TrimSpace(gitCommit)
	if resolvedCommit == "" || resolvedCommit == "unknown" {
		resolvedCommit = strings.TrimSpace(readBuildSetting("vcs.revision"))
	}
	if resolvedCommit != "" {
		info.Commit = shortenCommit(resolvedCommit)
	}

	resolvedBuildTime := strings.TrimSpace(buildDate)
	if resolvedBuildTime == "" || resolvedBuildTime == "unknown" {
		resolvedBuildTime = strings.TrimSpace(readBuildSetting("vcs.time"))
	}
	if normalizedBuildTime, ok := normalizeBuildTimeUTC(resolvedBuildTime); ok {
		info.BuildTime = normalizedBuildTime
	}

	return info
}

func shortenCommit(commit string) string {
	if len(commit) <= 12 {
		return commit
	}
	return commit[:12]
}

func normalizeBuildTimeUTC(raw string) (string, bool) {
	if raw == "" || raw == "unknown" {
		return "", false
	}

	t, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return "", false
	}

	return t.UTC().Format(time.RFC3339), true
}

func readBuildSetting(key string) string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	for _, setting := range info.Settings {
		if setting.Key == key {
			return setting.Value
		}
	}
	return ""
}

func printUsage() {
	fmt.Print(`ductile - Lightweight YAML-configured integration gateway

Usage:
  ductile <noun> <action> [flags]

Core Resources (Nouns):
  system    Gateway lifecycle and health
  config    System configuration and integrity
  job       Execution instances and lineage
  plugin    Capability discovery and management

System Commands:
  system start      Start the gateway service in foreground
  system status     Show global gateway health
  system reset      Reset a plugin poll circuit breaker
  system watch      Real-time diagnostic monitoring TUI

Config Commands:
  config lock       Authorize current state (update integrity hashes)
  config check      Validate syntax, policy, and integrity
  config token      Manage scoped tokens
  config scope      Manage token scopes
  config plugin     Manage plugin configuration
  config route      Manage event routes
  config webhook    Manage webhooks
  config init       Initialize config directory
  config backup     Create backup archive
  config restore    Restore config backup

Job Commands:
  job inspect <id>  Show lineage, baggage, and workspace artifacts

Plugin Commands:
  plugin list       Show discovered plugins (planned)
  plugin run <name> Manual execution (planned)

Manual Triggering:
  trigger           Show instructions for triggering plugins via API

General:
  --version         Show version information
  version           Show version information
  help              Show this help message

Use 'ductile <noun> help' for resource-specific flags.
`)
}

func printTriggerHelp() {
	fmt.Print(`Manual Plugin Triggering (via API)

Plugins are triggered via the REST API. This allows for programmatic control 
from LLMs, scripts, and external services.

Endpoint:
  POST /trigger/{plugin}/{command}

Headers:
  Authorization: Bearer <token>
  Content-Type: application/json

Body:
  {
    "payload": {
      "key": "value"
    }
  }

Example (curl):
  curl -X POST http://localhost:8080/trigger/echo/poll \
    -H "Authorization: Bearer my-token" \
    -H "Content-Type: application/json" \
    -d '{"payload": {"message": "Hello"}}'

For more details, see docs/API_REFERENCE.md.
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
		return runSystemStatus(actionArgs)
	case "monitor":
		if hasHelpFlag(actionArgs) {
			printSystemMonitorHelp()
			return 0
		}
		return runMonitor(actionArgs)
	case "reset":
		if hasHelpFlag(actionArgs) {
			printSystemResetHelp()
			return 0
		}
		return runSystemReset(actionArgs)
	case "watch":
		if hasHelpFlag(actionArgs) {
			printSystemWatchHelp()
			return 0
		}
		return runWatch(actionArgs)
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
	case "token":
		return runConfigToken(actionArgs)
	case "scope":
		return runConfigScope(actionArgs)
	case "plugin":
		return runConfigPlugin(actionArgs)
	case "route":
		return runConfigRoute(actionArgs)
	case "webhook":
		return runConfigWebhook(actionArgs)
	case "init":
		return runConfigInit(actionArgs)
	case "backup":
		return runConfigBackup(actionArgs)
	case "restore":
		return runConfigRestore(actionArgs)
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
	var configPath, configDir string
	var dryRun, apply bool

	fs := flag.NewFlagSet("set", flag.ContinueOnError)
	fs.StringVar(&configPath, "config", "", "Path to configuration")
	fs.StringVar(&configDir, "config-dir", "", "Path to configuration directory")
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
		fmt.Fprintf(os.Stderr, "Usage: ductile config set <path>=<value> [--dry-run | --apply]\n")
		return 1
	}

	if !dryRun && !apply {
		fmt.Println("Error: either --dry-run or --apply must be specified for 'config set'.")
		return 1
	}

	parts := strings.SplitN(kvPair, "=", 2)
	path, value := parts[0], parts[1]

	cfg, err := loadConfigForToolWithDir(configPath, configDir)
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
		if !strings.Contains(err.Error(), "no valid configuration source found") {
			fmt.Fprintf(os.Stderr, "Apply failed: %v\n", err)
			return 1
		}
		resolvedTarget, _, resolveErr := resolveConfigTarget(configPath, configDir)
		if resolveErr != nil {
			fmt.Fprintf(os.Stderr, "Apply failed: %v\n", err)
			return 1
		}
		if fallbackErr := applyConfigSetFallback(resolvedTarget, path, value); fallbackErr != nil {
			fmt.Fprintf(os.Stderr, "Apply failed: %v\n", fallbackErr)
			return 1
		}
	}

	fmt.Printf("Successfully set %q to %q\n", path, value)
	resolvedTarget, _, err := resolveConfigTarget(configPath, configDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Validation skipped: %v\n", err)
		return 0
	}
	validation, code, err := validateConfigAtPath(resolvedTarget)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Validation failed to run: %v\n", err)
		return 1
	}
	printValidationSummary(validation)
	return code
}

// ... (skipping to action implementations)

func runConfigShow(args []string) int {
	fs := flag.NewFlagSet("show", flag.ExitOnError)
	configPath := fs.String("config", "", "Path to configuration file or directory")
	configDir := fs.String("config-dir", "", "Path to configuration directory")
	jsonOut := fs.Bool("json", false, "Output in structured JSON format")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to parse flags: %v\n", err)
		return 1
	}

	cfg, err := loadConfigForToolWithDir(*configPath, *configDir)
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
	configDir := fs.String("config-dir", "", "Path to configuration directory")
	jsonOut := fs.Bool("json", false, "Output in structured JSON format")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to parse flags: %v\n", err)
		return 1
	}

	if fs.NArg() != 1 {
		fmt.Fprintf(os.Stderr, "Usage: ductile config get <path> [--json]\n")
		return 1
	}
	path := fs.Arg(0)

	cfg, err := loadConfigForToolWithDir(*configPath, *configDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
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
	return loadConfigForToolWithDir(configPath, "")
}

func loadConfigForToolWithDir(configPath, configDir string) (*config.Config, error) {
	if configPath != "" && configDir != "" {
		return nil, fmt.Errorf("use only one of --config or --config-dir")
	}
	if configDir != "" {
		configPath = configDir
	}
	if configPath == "" {
		discovered, err := config.DiscoverConfigDir()
		if err != nil {
			return nil, err
		}
		configPath = discovered
	}
	return config.Load(configPath)
}

func applyConfigSetFallback(configTarget, path, value string) error {
	configFile := configTarget
	info, err := os.Stat(configTarget)
	if err != nil {
		return err
	}
	if info.IsDir() {
		configFile = filepath.Join(configTarget, "config.yaml")
	}

	original, err := os.ReadFile(configFile)
	if err != nil {
		return err
	}

	var doc map[string]any
	if err := yaml.Unmarshal(original, &doc); err != nil {
		return err
	}
	if doc == nil {
		doc = map[string]any{}
	}

	setNestedMapValue(doc, strings.Split(path, "."), parseScalarValue(value))

	updated, err := yaml.Marshal(doc)
	if err != nil {
		return err
	}
	if err := writeFileAtomicWithBackup(configFile, updated, 0o644); err != nil {
		return err
	}

	if _, err := config.Load(configTarget); err != nil {
		backupPath := configFile + ".bak"
		if backup, readErr := os.ReadFile(backupPath); readErr == nil {
			_ = os.WriteFile(configFile, backup, 0o644)
		}
		return fmt.Errorf("validation failed: %w", err)
	}
	return nil
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
	fmt.Fprintln(w, "Usage: ductile system <action>")
	fmt.Fprintln(w, "Actions: start, status, monitor, reset, watch")
}

func printConfigNounHelp(w *os.File) {
	fmt.Fprintln(w, "Usage: ductile config <action> [flags]")
	fmt.Fprintln(w, "Actions: lock, check, show, get, set, token, scope, plugin, route, webhook, init, backup, restore")
}

func printJobNounHelp(w *os.File) {
	fmt.Fprintln(w, "Usage: ductile job <action>")
	fmt.Fprintln(w, "Actions: inspect")
}

func printPluginNounHelp(w *os.File) {
	fmt.Fprintln(w, "Usage: ductile plugin <action>")
	fmt.Fprintln(w, "Actions: list, run")
}

func printSystemStartHelp() {
	fmt.Println("Usage: ductile system start [--config PATH]")
	fmt.Println("Start the gateway service in the foreground.")
}

func printSystemStatusHelp() {
	fmt.Println("Usage: ductile system status [--config PATH] [--json]")
	fmt.Println("Show global gateway health (config, database readiness, and PID lock state).")
	fmt.Println("")
	fmt.Println("Exit codes:")
	fmt.Println("  0  All required checks passed")
	fmt.Println("  1  One or more checks failed")
}

func printSystemMonitorHelp() {
	fmt.Println("Usage: ductile system monitor [--api-url URL] [--api-key KEY]")
	fmt.Println("Launch the real-time TUI dashboard.")
}

func printSystemResetHelp() {
	fmt.Println("Usage: ductile system reset <plugin> [--config PATH]")
	fmt.Println("Reset scheduler poll circuit breaker state for a plugin.")
}

func runMonitor(args []string) int {
	fs := flag.NewFlagSet("monitor", flag.ExitOnError)
	apiURL := fs.String("api-url", "http://localhost:8080", "Gateway API URL")
	apiKey := fs.String("api-key", os.Getenv("DUCTILE_API_KEY"), "API Bearer Token")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "Flag error: %v\n", err)
		return 1
	}

	if *apiKey == "" {
		fmt.Fprintln(os.Stderr, "Error: API key required. Use --api-key or DUCTILE_API_KEY env var.")
		return 1
	}

	m := tui.NewMonitor(*apiURL, *apiKey)
	p := tea.NewProgram(m)
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "TUI error: %v\n", err)
		return 1
	}
	return 0
}

func runSystemReset(actionArgs []string) int {
	fs := flag.NewFlagSet("reset", flag.ContinueOnError)
	configPath := fs.String("config", "", "Path to configuration file or directory")
	if err := fs.Parse(actionArgs); err != nil {
		fmt.Fprintf(os.Stderr, "Flag error: %v\n", err)
		return 1
	}

	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "Usage: ductile system reset <plugin> [--config PATH]")
		return 1
	}
	pluginName := strings.TrimSpace(fs.Arg(0))
	if pluginName == "" {
		fmt.Fprintln(os.Stderr, "plugin name is required")
		return 1
	}

	if *configPath == "" {
		discovered, err := config.DiscoverConfigDir()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to discover config: %v\n", err)
			return 1
		}
		*configPath = discovered
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		return 1
	}
	if _, ok := cfg.Plugins[pluginName]; !ok {
		fmt.Fprintf(os.Stderr, "Unknown plugin: %s\n", pluginName)
		return 1
	}

	db, err := storage.OpenSQLite(context.Background(), cfg.State.Path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open database: %v\n", err)
		return 1
	}
	defer db.Close()

	q := queue.New(db)
	if err := q.ResetCircuitBreaker(context.Background(), pluginName, "poll"); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to reset circuit breaker: %v\n", err)
		return 1
	}

	fmt.Printf("Reset circuit breaker for %s (poll)\n", pluginName)
	return 0
}

func runWatch(args []string) int {
	fs := flag.NewFlagSet("watch", flag.ExitOnError)
	apiURL := fs.String("api-url", "http://localhost:8080", "Gateway API URL")
	apiKey := fs.String("api-key", os.Getenv("DUCTILE_API_KEY"), "API Bearer Token")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "Flag error: %v\n", err)
		return 1
	}

	if *apiKey == "" {
		fmt.Fprintln(os.Stderr, "Error: API key required. Use --api-key or DUCTILE_API_KEY env var.")
		return 1
	}

	m := watch.New(*apiURL, *apiKey)
	p := tea.NewProgram(m)
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "TUI error: %v\n", err)
		return 1
	}
	return 0
}

func printSystemWatchHelp() {
	fmt.Println("Usage: ductile system watch [flags]")
	fmt.Println()
	fmt.Println("Real-time diagnostic monitoring TUI (Overwatch).")
	fmt.Println("Shows gateway health, active pipelines, and event stream.")
	fmt.Println()
	fmt.Println("Flags:")
	fmt.Println("  --api-url URL    Gateway API URL (default: http://localhost:8080)")
	fmt.Println("  --api-key KEY    API Bearer Token (or DUCTILE_API_KEY env var)")
	fmt.Println()
	fmt.Println("Keybindings:")
	fmt.Println("  q, Ctrl+C        Quit")
	fmt.Println("  ↑/↓, k/j         Navigate pipelines")
}

func printConfigLockHelp() {
	fmt.Println("Usage: ductile config lock [--config PATH | --config-dir PATH] [-v|--verbose] [--dry-run]")
	fmt.Println("Authorize current configuration state by regenerating scope file integrity hashes.")
}

func printConfigCheckHelp() {
	fmt.Println("Usage: ductile config check [--config PATH | --config-dir PATH] [--format human|json] [--strict] [--json]")
	fmt.Println("Validate configuration syntax, policy, and integrity.")
}

func printConfigShowHelp() {
	fmt.Println("Usage: ductile config show [entity] [--config PATH | --config-dir PATH] [--json]")
	fmt.Println("Show full resolved configuration or a filtered entity node.")
}

func printConfigGetHelp() {
	fmt.Println("Usage: ductile config get <path> [--config PATH | --config-dir PATH] [--json]")
	fmt.Println("Read a single value from the resolved configuration.")
}

func printConfigSetHelp() {
	fmt.Println("Usage: ductile config set <path>=<value> [--config PATH | --config-dir PATH] [--dry-run | --apply]")
	fmt.Println("Set a configuration value with either preview or apply mode.")
}

func printJobInspectHelp() {
	fmt.Println("Usage: ductile job inspect <job_id> [--config PATH] [--json]")
	fmt.Println("Inspect job lineage, baggage, and workspace artifacts.")
}

func printPluginListHelp() {
	fmt.Println("Usage: ductile plugin list")
	fmt.Println("Show discovered plugins (planned).")
}

func printPluginRunHelp() {
	fmt.Println("Usage: ductile plugin run <name>")
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
	logger.Info("ductile starting", "version", version, "config", *configPath)

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

	q := queue.New(
		db,
		queue.WithLogger(logger),
		queue.WithDedupeTTL(cfg.Service.DedupeTTL),
	)
	st := state.NewStore(db)
	contextStore := state.NewContextStore(db)
	hub := events.NewHub(256)

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

	routerEngine, err := router.LoadFromConfigDir(configDir, registry, logger)
	if err != nil {
		logger.Error("failed to load router pipelines", "config_dir", configDir, "error", err)
		return 1
	}
	if r, ok := routerEngine.(*router.Router); ok {
		pipelines := r.PipelineSummary()
		logger.Info("pipeline discovery complete", "config_dir", configDir, "pipelines_loaded", len(pipelines))
		for _, p := range pipelines {
			logger.Info("pipeline registered", "name", p.Name, "trigger", p.Trigger)
		}
	}

	sched := scheduler.New(cfg, q, hub, logger)
	disp := dispatch.New(q, st, contextStore, wsManager, routerEngine, registry, hub, cfg)

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
			Listen:            cfg.API.Listen,
			APIKey:            cfg.API.Auth.APIKey,
			Tokens:            tokens,
			MaxConcurrentSync: cfg.API.MaxConcurrentSync,
			MaxSyncTimeout:    cfg.API.MaxSyncTimeout,
		}
		apiServer := api.New(apiConfig, q, registry, routerEngine, disp, hub, log.WithComponent("api"))
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

	logger.Info("ductile running (press Ctrl+C to stop)")

	select {
	case sig := <-sigCh:
		logger.Info("received shutdown signal", "signal", sig)
		cancel()
	case err := <-errCh:
		logger.Error("component failed", "error", err)
		cancel()
		return 1
	}

	logger.Info("ductile stopped")
	return 0
}
