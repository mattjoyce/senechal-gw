package main

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/mattjoyce/ductile/internal/config"
	"github.com/mattjoyce/ductile/internal/doctor"
	"github.com/mattjoyce/ductile/internal/plugin"
	"github.com/mattjoyce/ductile/internal/storage"
	"gopkg.in/yaml.v3"
)

var (
	version   = "dev"
	gitCommit = "unknown"
	buildDate = "unknown"
)

//go:embed templates/skills-core-mode.md
var skillsCoreMode string

//go:embed templates/skills-cli-commands.md
var skillsCLICommands string

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
	case "relay":
		return runRelayNoun(args)
	case "skills":
		return runSystemSkills(args)
	case "api":
		return runAPINoun(args)

	case "version":
		return runVersion(args)
	case "help", "--help", "-h":
		printUsage()
		return 0

	default:
		// #nosec G705 -- stderr output is plain text, not HTML.
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

func validateScheduledCommands(cfg *config.Config, registry *plugin.Registry) error {
	if cfg == nil || registry == nil {
		return nil
	}

	for pluginName, pluginConf := range cfg.Plugins {
		if !pluginConf.Enabled {
			continue
		}

		plug, ok := registry.Get(pluginName)
		if !ok {
			return fmt.Errorf("plugin %q is configured but not discoverable", pluginName)
		}

		for _, schedule := range pluginConf.NormalizedSchedules() {
			command := strings.TrimSpace(schedule.Command)
			if command == "" {
				command = "poll"
			}

			scheduleID := strings.TrimSpace(schedule.ID)
			if scheduleID == "" {
				scheduleID = "default"
			}

			if !plug.SupportsCommand(command) {
				return fmt.Errorf("plugin %q schedule %q references unsupported command %q", pluginName, scheduleID, command)
			}
		}
	}

	return nil
}

func printUsage() {
	fmt.Print(`ductile - Lightweight, open-source integration engine for the agentic era.

Usage:
  ductile <noun> <action> [flags]

Core Resources (Nouns):
  system    Gateway lifecycle and health
  config    System configuration and integrity
  job       Execution instances and lineage
  plugin    Capability discovery and management (Connectors)
  api       Directly call the gateway API

System Commands:
  system start      Start the integration gateway in foreground
  system status     Show global gateway health
  system plugin-facts Show recent append-only plugin facts
  system scheduler  Show scheduler-submitted polls currently in flight
  system reset      Reset a plugin/connector circuit breaker
  system reload     Reload configuration in a running gateway
  system skills     Export capability registry (Skills) as LLM-readable Markdown

Config Commands:
  config lock       Authorize current state (update integrity hashes)
  config check      Validate syntax, policy, and integrity
  config show       Show full resolved configuration or a filtered entity node
  config get        Read a single value from the resolved configuration
  config set        Set a configuration value with either preview or apply mode
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
  job logs          Query stored job logs for audit and troubleshooting

Plugin Commands:
  plugin list       Show discovered plugins/connectors
  plugin run <name> Manual execution

Relay Commands:
  relay send <instance> Send one authenticated remote relay event

API Commands:
  api <endpoint>    Directly call the gateway API (uses configured key)

Capability Discovery (Skills):
  skills            Export live capability registry as LLM-readable Markdown

General:
  --version         Show version information
  version           Show version information
  help              Show this help message

Use 'ductile <noun> help' for resource-specific flags.
`)
}

// --- NOUN DISPATCHERS ---

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
	case "lock":
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
		// #nosec G705 -- stderr output is plain text, not HTML.
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
	remainingArgs := args
	for len(remainingArgs) > 0 {
		if err := fs.Parse(remainingArgs); err != nil {
			fmt.Fprintf(os.Stderr, "Flag error: %v\n", err)
			return 1
		}
		if fs.NArg() > 0 {
			arg0 := fs.Arg(0)
			if kvPair == "" && strings.Contains(arg0, "=") {
				kvPair = arg0
			}
			remainingArgs = fs.Args()[1:]
		} else {
			remainingArgs = nil
		}
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
	var configPath, configDir string
	var jsonOut bool

	fs := flag.NewFlagSet("show", flag.ExitOnError)
	fs.StringVar(&configPath, "config", "", "Path to configuration file or directory")
	fs.StringVar(&configDir, "config-dir", "", "Path to configuration directory")
	fs.BoolVar(&jsonOut, "json", false, "Output in structured JSON format")

	var entity string
	remainingArgs := args
	for len(remainingArgs) > 0 {
		if err := fs.Parse(remainingArgs); err != nil {
			fmt.Fprintf(os.Stderr, "Flag error: %v\n", err)
			return 1
		}
		if fs.NArg() > 0 {
			if entity == "" {
				entity = fs.Arg(0)
			}
			remainingArgs = fs.Args()[1:]
		} else {
			remainingArgs = nil
		}
	}

	cfg, err := loadConfigForToolWithDir(configPath, configDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Load error: %v\n", err)
		return 1
	}

	var result any = cfg
	if entity != "" {
		res, err := cfg.GetPath(entity)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return 1
		}
		result = res
	}

	if jsonOut {
		data, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(data))
	} else {
		data, _ := yaml.Marshal(result)
		fmt.Print(string(data))
	}
	return 0
}

func runConfigGet(args []string) int {
	var configPath, configDir string
	var jsonOut bool

	fs := flag.NewFlagSet("get", flag.ExitOnError)
	fs.StringVar(&configPath, "config", "", "Path to configuration file or directory")
	fs.StringVar(&configDir, "config-dir", "", "Path to configuration directory")
	fs.BoolVar(&jsonOut, "json", false, "Output in structured JSON format")

	var path string
	remainingArgs := args
	for len(remainingArgs) > 0 {
		if err := fs.Parse(remainingArgs); err != nil {
			fmt.Fprintf(os.Stderr, "Flag error: %v\n", err)
			return 1
		}
		if fs.NArg() > 0 {
			if path == "" {
				path = fs.Arg(0)
			}
			remainingArgs = fs.Args()[1:]
		} else {
			remainingArgs = nil
		}
	}

	if path == "" {
		fmt.Fprintf(os.Stderr, "Usage: ductile config get <path> [--json]\n")
		return 1
	}

	cfg, err := loadConfigForToolWithDir(configPath, configDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Load error: %v\n", err)
		return 1
	}

	val, err := cfg.GetPath(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	if jsonOut {
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

	// #nosec G304 -- config paths are operator-controlled local inputs.
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
	if err := writeFileAtomicWithBackup(configFile, updated, 0o600); err != nil {
		return err
	}

	if _, err := config.Load(configTarget); err != nil {
		backupPath := configFile + ".bak"
		// #nosec G304 -- config paths are operator-controlled local inputs.
		if backup, readErr := os.ReadFile(backupPath); readErr == nil {
			// #nosec G703 -- config paths are operator-controlled local inputs.
			_ = os.WriteFile(configFile, backup, 0o600)
		}
		return fmt.Errorf("validation failed: %w", err)
	}
	return nil
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

func printConfigNounHelp(w *os.File) {
	_, _ = fmt.Fprintln(w, "Usage: ductile config <action> [flags]")
	_, _ = fmt.Fprintln(w, "Actions: lock, check, show, get, set, token, scope, plugin, route, webhook, init, backup, restore")
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

type systemStatusCheck struct {
	Name      string `json:"name"`
	OK        bool   `json:"ok"`
	Path      string `json:"path,omitempty"`
	Detail    string `json:"detail,omitempty"`
	ActivePID int    `json:"active_pid,omitempty"`
}

type systemStatusReport struct {
	Healthy bool                `json:"healthy"`
	Checks  []systemStatusCheck `json:"checks"`
}

func runSystemStatus(args []string) int {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	configPath := fs.String("config", "", "Path to configuration file or directory")
	jsonOut := fs.Bool("json", false, "Output status as JSON")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "Flag error: %v\n", err)
		return 1
	}

	report := collectSystemStatus(*configPath)

	if *jsonOut {
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to render JSON status: %v\n", err)
			return 1
		}
		fmt.Println(string(data))
	} else {
		renderSystemStatusHuman(report)
	}

	if report.Healthy {
		return 0
	}
	return 1
}

func collectSystemStatus(configPath string) systemStatusReport {
	report := systemStatusReport{
		Healthy: true,
		Checks:  make([]systemStatusCheck, 0, 4),
	}

	resolvedConfigPath := configPath
	if resolvedConfigPath == "" {
		discovered, err := config.DiscoverConfigDir()
		if err != nil {
			report.Checks = append(report.Checks,
				systemStatusCheck{
					Name:   "config_discovery",
					OK:     false,
					Detail: err.Error(),
				},
				systemStatusCheck{
					Name:   "config_load",
					OK:     false,
					Detail: "skipped: config discovery failed",
				},
				systemStatusCheck{
					Name:   "state_db",
					OK:     false,
					Detail: "skipped: config discovery failed",
				},
				systemStatusCheck{
					Name:   "pid_lock",
					OK:     false,
					Detail: "skipped: config discovery failed",
				},
			)
			report.Healthy = false
			return report
		}
		resolvedConfigPath = discovered
	}

	report.Checks = append(report.Checks, systemStatusCheck{
		Name:   "config_discovery",
		OK:     true,
		Path:   resolvedConfigPath,
		Detail: "config path resolved",
	})

	cfg, err := config.Load(resolvedConfigPath)
	if err != nil {
		report.Checks = append(report.Checks,
			systemStatusCheck{
				Name:   "config_load",
				OK:     false,
				Path:   resolvedConfigPath,
				Detail: err.Error(),
			},
			systemStatusCheck{
				Name:   "state_db",
				OK:     false,
				Detail: "skipped: config load failed",
			},
			systemStatusCheck{
				Name:   "pid_lock",
				OK:     false,
				Detail: "skipped: config load failed",
			},
		)
		report.Healthy = false
		return report
	}

	report.Checks = append(report.Checks, systemStatusCheck{
		Name:   "config_load",
		OK:     true,
		Path:   resolvedConfigPath,
		Detail: "configuration loaded",
	})

	stateDBCheck := checkStateDBReadiness(cfg.State.Path)
	if !stateDBCheck.OK {
		report.Healthy = false
	}
	report.Checks = append(report.Checks, stateDBCheck)

	pidLockCheck := checkPIDLockState(getPIDLockPath(cfg))
	if !pidLockCheck.OK {
		report.Healthy = false
	}
	report.Checks = append(report.Checks, pidLockCheck)

	return report
}

func checkStateDBReadiness(statePath string) systemStatusCheck {
	check := systemStatusCheck{
		Name: "state_db",
		Path: statePath,
	}

	if statePath == "" {
		check.OK = false
		check.Detail = "state path is empty"
		return check
	}

	if _, err := os.Stat(statePath); os.IsNotExist(err) {
		check.OK = true
		check.Detail = "database file does not exist yet (will be created on start)"
		return check
	}

	dsn := fmt.Sprintf("file:%s?mode=ro", filepath.ToSlash(statePath))
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		check.OK = false
		check.Detail = fmt.Sprintf("open failed: %v", err)
		return check
	}
	defer func() { _ = db.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		check.OK = false
		check.Detail = fmt.Sprintf("ping failed: %v", err)
		return check
	}

	check.OK = true
	check.Detail = "database is readable"
	return check
}

func checkPIDLockState(lockPath string) systemStatusCheck {
	check := systemStatusCheck{
		Name: "pid_lock",
		Path: lockPath,
	}

	// #nosec G304 -- lock path is operator-controlled local input.
	data, err := os.ReadFile(lockPath)
	if err != nil {
		if os.IsNotExist(err) {
			check.OK = true
			check.Detail = "no active PID lock file"
			return check
		}
		check.OK = false
		check.Detail = fmt.Sprintf("failed to read lock file: %v", err)
		return check
	}

	line := strings.TrimSpace(string(data))
	if line == "" {
		check.OK = true
		check.Detail = "lock file present but empty (not active)"
		return check
	}

	pid, err := strconv.Atoi(line)
	if err != nil || pid <= 0 {
		check.OK = false
		check.Detail = "lock file contains invalid pid"
		return check
	}

	if processExists(pid) {
		check.OK = false
		check.ActivePID = pid
		check.Detail = fmt.Sprintf("another instance appears active (pid %d)", pid)
		return check
	}

	check.OK = true
	check.Detail = fmt.Sprintf("stale lock file detected (pid %d not running)", pid)
	return check
}

func processExists(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

func renderSystemStatusHuman(report systemStatusReport) {
	state := "HEALTHY"
	if !report.Healthy {
		state = "DEGRADED"
	}
	fmt.Printf("System Status: %s\n", state)
	for _, c := range report.Checks {
		status := "OK"
		if !c.OK {
			status = "FAIL"
		}
		fmt.Printf("- %s: %s", c.Name, status)
		if c.Path != "" {
			fmt.Printf(" (path=%s)", c.Path)
		}
		if c.Detail != "" {
			fmt.Printf(" - %s", c.Detail)
		}
		fmt.Println()
	}
}

// runSystemSelfcheck runs pre-deploy/post-migration health checks. Stricter
// than `system status`: it opens the database directly, runs PRAGMA
// integrity_check, validates the embedded schema shape, and reports invariants
// operators care about during a migration window.
func runSystemSelfcheck(args []string) int {
	fs := flag.NewFlagSet("selfcheck", flag.ContinueOnError)
	configPath := fs.String("config", "", "Path to configuration file or directory")
	jsonOut := fs.Bool("json", false, "Output report as JSON")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "Flag error: %v\n", err)
		return 1
	}

	report := collectSystemSelfcheck(*configPath)

	if *jsonOut {
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to render JSON selfcheck: %v\n", err)
			return 1
		}
		fmt.Println(string(data))
	} else {
		renderSystemSelfcheckHuman(report)
	}

	if report.Healthy {
		return 0
	}
	return 1
}

func collectSystemSelfcheck(configPath string) systemStatusReport {
	report := systemStatusReport{
		Healthy: true,
		Checks:  make([]systemStatusCheck, 0, 6),
	}

	resolvedConfigPath := configPath
	if resolvedConfigPath == "" {
		discovered, err := config.DiscoverConfigDir()
		if err != nil {
			report.Healthy = false
			report.Checks = append(report.Checks, systemStatusCheck{
				Name:   "config_discovery",
				OK:     false,
				Detail: err.Error(),
			})
			return report
		}
		resolvedConfigPath = discovered
	}
	report.Checks = append(report.Checks, systemStatusCheck{
		Name:   "config_discovery",
		OK:     true,
		Path:   resolvedConfigPath,
		Detail: "config path resolved",
	})

	cfg, err := config.Load(resolvedConfigPath)
	if err != nil {
		report.Healthy = false
		report.Checks = append(report.Checks, systemStatusCheck{
			Name:   "config_load",
			OK:     false,
			Path:   resolvedConfigPath,
			Detail: err.Error(),
		})
		return report
	}
	report.Checks = append(report.Checks, systemStatusCheck{
		Name:   "config_load",
		OK:     true,
		Path:   resolvedConfigPath,
		Detail: "configuration loaded",
	})

	pidLockCheck := checkPIDLockState(getPIDLockPath(cfg))
	if !pidLockCheck.OK {
		report.Healthy = false
	}
	report.Checks = append(report.Checks, pidLockCheck)

	statePath := cfg.State.Path
	if statePath == "" {
		report.Healthy = false
		report.Checks = append(report.Checks, systemStatusCheck{
			Name:   "db_open",
			OK:     false,
			Detail: "state path is empty",
		})
		return report
	}

	// PRAGMA integrity_check on a live WAL is unsafe — refuse if a gateway
	// is holding the PID lock with an active process.
	if !pidLockCheck.OK && pidLockCheck.ActivePID != 0 {
		report.Checks = append(report.Checks,
			systemStatusCheck{
				Name:   "db_integrity",
				OK:     false,
				Detail: "skipped: active gateway holds PID lock — quiesce before selfcheck",
			},
			systemStatusCheck{
				Name:   "db_schema",
				OK:     false,
				Detail: "skipped: active gateway holds PID lock",
			},
			systemStatusCheck{
				Name:   "queue_terminal_freshness",
				OK:     false,
				Detail: "skipped: active gateway holds PID lock",
			},
		)
		return report
	}

	if _, err := os.Stat(statePath); os.IsNotExist(err) {
		report.Healthy = false
		report.Checks = append(report.Checks, systemStatusCheck{
			Name:   "db_open",
			OK:     false,
			Path:   statePath,
			Detail: "database file does not exist; start the gateway once before selfcheck",
		})
		return report
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	db, err := sql.Open("sqlite", statePath)
	if err != nil {
		report.Healthy = false
		report.Checks = append(report.Checks, systemStatusCheck{
			Name:   "db_open",
			OK:     false,
			Path:   statePath,
			Detail: fmt.Sprintf("open failed: %v", err),
		})
		return report
	}
	defer func() { _ = db.Close() }()

	integrityCheck := checkDBIntegrity(ctx, db, statePath)
	if !integrityCheck.OK {
		report.Healthy = false
	}
	report.Checks = append(report.Checks, integrityCheck)

	schemaCheck := checkDBSchema(ctx, db, statePath)
	if !schemaCheck.OK {
		report.Healthy = false
	}
	report.Checks = append(report.Checks, schemaCheck)

	freshness := checkQueueTerminalFreshness(ctx, db, statePath)
	if !freshness.OK {
		report.Healthy = false
	}
	report.Checks = append(report.Checks, freshness)

	return report
}

func checkDBIntegrity(ctx context.Context, db *sql.DB, statePath string) systemStatusCheck {
	check := systemStatusCheck{
		Name: "db_integrity",
		Path: statePath,
	}
	rows, err := db.QueryContext(ctx, "PRAGMA integrity_check;")
	if err != nil {
		check.OK = false
		check.Detail = fmt.Sprintf("integrity_check failed: %v", err)
		return check
	}
	defer func() { _ = rows.Close() }()

	var lines []string
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			check.OK = false
			check.Detail = fmt.Sprintf("scan integrity_check row: %v", err)
			return check
		}
		lines = append(lines, line)
	}
	if err := rows.Err(); err != nil {
		check.OK = false
		check.Detail = fmt.Sprintf("integrity_check rows: %v", err)
		return check
	}
	if len(lines) == 1 && lines[0] == "ok" {
		check.OK = true
		check.Detail = "PRAGMA integrity_check returned ok"
		return check
	}
	check.OK = false
	check.Detail = fmt.Sprintf("integrity_check reported issues: %s", strings.Join(lines, "; "))
	return check
}

func checkDBSchema(ctx context.Context, db *sql.DB, statePath string) systemStatusCheck {
	check := systemStatusCheck{
		Name: "db_schema",
		Path: statePath,
	}
	if err := storage.ValidateSQLiteSchema(ctx, db); err != nil {
		check.OK = false
		check.Detail = err.Error()
		return check
	}
	check.OK = true
	check.Detail = "all required tables, columns, and indexes present"
	return check
}

// checkQueueTerminalFreshness reports whether terminal-state rows are accumulating
// in job_queue. Wave-1 (pre-pruneJobQueue): tolerates up to 5000 stale rows so
// existing dirty databases don't fail selfcheck before the pruner ships.
// Wave-2 (post-pruneJobQueue): tighten to fail on any non-zero count.
const queueTerminalFreshnessFailThreshold = 5000

func checkQueueTerminalFreshness(ctx context.Context, db *sql.DB, statePath string) systemStatusCheck {
	check := systemStatusCheck{
		Name: "queue_terminal_freshness",
		Path: statePath,
	}
	// Predicate must match queue.PruneJobQueue's terminal-status set so the
	// selfcheck invariant and the pruner agree on what "stale terminal" means.
	const q = `
SELECT COUNT(*) FROM job_queue
WHERE status IN ('succeeded','skipped','failed','timed_out','dead')
  AND completed_at IS NOT NULL
  AND completed_at < datetime('now','-2 days');
`
	var count int
	if err := db.QueryRowContext(ctx, q).Scan(&count); err != nil {
		check.OK = false
		check.Detail = fmt.Sprintf("query failed: %v", err)
		return check
	}
	switch {
	case count == 0:
		check.OK = true
		check.Detail = "no stale terminal-state rows in job_queue"
	case count <= queueTerminalFreshnessFailThreshold:
		check.OK = true
		check.Detail = fmt.Sprintf("%d stale terminal-state rows in job_queue (within Wave-1 tolerance; ship pruneJobQueue to clear)", count)
	default:
		check.OK = false
		check.Detail = fmt.Sprintf("%d stale terminal-state rows exceeds threshold %d; pruneJobQueue likely missing or stuck", count, queueTerminalFreshnessFailThreshold)
	}
	return check
}

func renderSystemSelfcheckHuman(report systemStatusReport) {
	state := "PASS"
	if !report.Healthy {
		state = "FAIL"
	}
	fmt.Printf("Selfcheck: %s\n", state)
	for _, c := range report.Checks {
		status := "OK"
		if !c.OK {
			status = "FAIL"
		}
		fmt.Printf("- %s: %s", c.Name, status)
		if c.Path != "" {
			fmt.Printf(" (path=%s)", c.Path)
		}
		if c.Detail != "" {
			fmt.Printf(" - %s", c.Detail)
		}
		fmt.Println()
	}
}

func runConfigCheck(args []string) int {
	var configPath, configDir string
	var strict, jsonOut bool
	var format string

	fs := flag.NewFlagSet("check", flag.ExitOnError)
	fs.StringVar(&configPath, "config", "", "Path to configuration")
	fs.StringVar(&configDir, "config-dir", "", "Path to configuration directory")
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

	if configPath != "" && configDir != "" {
		fmt.Fprintln(os.Stderr, "Error: use only one of --config or --config-dir")
		return 1
	}
	if configDir != "" {
		configPath = configDir
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

	registry, err := discoverRegistry(cfg, configPath)
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
		configPath := filepath.Join(dir, "config.yaml")
		if _, err := os.Stat(configPath); err == nil {
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

			cfg, err := config.LoadForLock(configPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to load config for plugin locking in %s: %v\n", dir, err)
				return 1
			}
			resolved, err := resolveConfiguredPluginFingerprints(cfg, configPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to resolve plugin fingerprints in %s: %v\n", dir, err)
				return 1
			}
			if isVerbose {
				for _, rp := range resolved {
					fmt.Printf("  DISCOVER [plugin] %s manifest=%s entrypoint=%s enabled=%t\n",
						rp.Name, rp.ManifestPath, rp.EntrypointPath, rp.Enabled)
				}
			}
			if err := config.GenerateChecksumsWithPlugins(files, resolved, dryRun); err != nil {
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
			continue
		} else if err != nil && !os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "Failed to access %s: %v\n", configPath, err)
			return 1
		}

		scopeFiles := []string{"tokens.yaml", "webhooks.yaml"}
		report, err := config.GenerateChecksumsWithReport(dir, scopeFiles, dryRun)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to lock config in %s: %v\n", dir, err)
			return 1
		}
		if isVerbose {
			fmt.Printf("Processing directory: %s\n", dir)
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

func readPIDFromFile(path string) (int, error) {
	// #nosec G304 -- pid lock path is operator-controlled local input.
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf("failed to read pid file %s: %w", path, err)
	}
	raw := strings.TrimSpace(string(data))
	pid, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid pid in %s", path)
	}
	return pid, nil
}

type stringSlice []string

func (s *stringSlice) String() string {
	return strings.Join(*s, ",")
}

func (s *stringSlice) Set(value string) error {
	*s = append(*s, value)
	return nil
}
