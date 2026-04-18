package main

import (
	"bytes"
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattjoyce/ductile/internal/api"
	"github.com/mattjoyce/ductile/internal/auth"
	"github.com/mattjoyce/ductile/internal/config"
	"github.com/mattjoyce/ductile/internal/configsnapshot"
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
	"github.com/mattjoyce/ductile/internal/tui/watch"
	"github.com/mattjoyce/ductile/internal/webhook"
	"github.com/mattjoyce/ductile/internal/workspace"
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
  system reset      Reset a plugin/connector circuit breaker
  system reload     Reload configuration in a running gateway
  system watch      Real-time diagnostic monitoring TUI
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
	case "reset":
		if hasHelpFlag(actionArgs) {
			printSystemResetHelp()
			return 0
		}
		return runSystemReset(actionArgs)
	case "reload":
		if hasHelpFlag(actionArgs) {
			printSystemReloadHelp()
			return 0
		}
		return runSystemReload(actionArgs)
	case "skills":
		return runSystemSkills(actionArgs)
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
		// #nosec G705 -- stderr output is plain text, not HTML.
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
	case "logs":
		if hasHelpFlag(actionArgs) {
			printJobLogsHelp()
			return 0
		}
		return runJobLogs(actionArgs)
	case "help":
		printJobNounHelp(os.Stdout)
		return 0
	default:
		// #nosec G705 -- stderr output is plain text, not HTML.
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
		return runPluginList(actionArgs)
	case "run":
		if hasHelpFlag(actionArgs) {
			printPluginRunHelp()
			return 0
		}
		return runPluginRun(actionArgs)
	case "help":
		printPluginNounHelp(os.Stdout)
		return 0
	default:
		// #nosec G705 -- stderr output is plain text, not HTML.
		fmt.Fprintf(os.Stderr, "Unknown plugin action: %s\n", action)
		return 1
	}
}

func runAPINoun(args []string) int {
	if len(args) < 1 || isHelpToken(args[0]) {
		printAPIHelp()
		return 0
	}

	endpoint := args[0]
	if !strings.HasPrefix(endpoint, "/") {
		endpoint = "/" + endpoint
	}
	actionArgs := args[1:]

	var method, apiURL, apiKey, bodyStr, configPath string
	var headers, fields stringSlice

	fs := flag.NewFlagSet("api", flag.ContinueOnError)
	fs.StringVar(&method, "method", "", "HTTP method")
	fs.StringVar(&method, "X", "", "HTTP method (alias)")
	fs.StringVar(&apiURL, "api-url", "", "Base API URL")
	fs.StringVar(&apiKey, "api-key", "", "API Key")
	fs.StringVar(&bodyStr, "body", "", "Request body (or - for stdin)")
	fs.StringVar(&bodyStr, "b", "", "Request body (alias)")
	fs.Var(&headers, "header", "HTTP header (Header: value)")
	fs.Var(&headers, "H", "HTTP header (alias)")
	fs.Var(&fields, "field", "JSON field (key=value)")
	fs.Var(&fields, "f", "JSON field (alias)")
	fs.StringVar(&configPath, "config", "", "Path to configuration")
	fs.StringVar(&configPath, "c", "", "Path to configuration (alias)")

	if err := fs.Parse(actionArgs); err != nil {
		return 1
	}

	if method == "" {
		if len(fields) > 0 || bodyStr != "" {
			method = "POST"
		} else {
			method = "GET"
		}
	}

	// Load config to find defaults
	if apiURL == "" || apiKey == "" {
		resolvedPath := configPath
		if resolvedPath == "" {
			// Try to discover config directory
			if dir, err := config.DiscoverConfigDir(); err == nil {
				resolvedPath = dir
			} else {
				resolvedPath = "."
			}
		}
		cfg, _ := config.Load(resolvedPath) // ignore error, might not have config
		if cfg != nil {
			if apiURL == "" && cfg.API.Listen != "" {
				apiURL = cfg.API.Listen
				if !strings.HasPrefix(apiURL, "http://") && !strings.HasPrefix(apiURL, "https://") {
					apiURL = "http://" + apiURL
				}
			}
			if apiKey == "" && len(cfg.API.Auth.Tokens) > 0 {
				apiKey = cfg.API.Auth.Tokens[0].Token
			}
		}
	}

	// Check env vars as fallback
	if apiKey == "" {
		apiKey = os.Getenv("DUCTILE_API_KEY")
	}
	if apiURL == "" {
		apiURL = "http://localhost:8080"
	}

	var bodyReader io.Reader
	if bodyStr == "-" {
		bodyReader = os.Stdin
	} else if bodyStr != "" {
		bodyReader = strings.NewReader(bodyStr)
	} else if len(fields) > 0 {
		fieldMap := make(map[string]any)
		for _, f := range fields {
			parts := strings.SplitN(f, "=", 2)
			if len(parts) != 2 {
				fmt.Fprintf(os.Stderr, "Invalid field format %q, expected key=value\n", f)
				return 1
			}
			val := parts[1]
			// Try to parse as JSON types (bool, int)
			if b, err := strconv.ParseBool(val); err == nil {
				fieldMap[parts[0]] = b
			} else if i, err := strconv.Atoi(val); err == nil {
				fieldMap[parts[0]] = i
			} else {
				fieldMap[parts[0]] = val
			}
		}
		data, err := json.Marshal(fieldMap)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to marshal fields: %v\n", err)
			return 1
		}
		bodyReader = bytes.NewReader(data)
	}

	fullURL, err := buildValidatedGatewayAPIURL(apiURL, endpoint)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid API URL: %v\n", err)
		return 1
	}
	req, err := http.NewRequest(method, fullURL, bodyReader)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create request: %v\n", err)
		return 1
	}

	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	if bodyReader != nil && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}

	for _, h := range headers {
		parts := strings.SplitN(h, ":", 2)
		if len(parts) != 2 {
			fmt.Fprintf(os.Stderr, "Invalid header format %q, expected Header: value\n", h)
			return 1
		}
		req.Header.Add(strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]))
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Request failed: %v\n", err)
		return 1
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to read response: %v\n", err)
		return 1
	}

	if len(body) > 0 {
		var pretty bytes.Buffer
		if err := json.Indent(&pretty, body, "", "  "); err == nil {
			fmt.Println(pretty.String())
		} else {
			fmt.Println(string(body))
		}
	}

	if resp.StatusCode >= 400 {
		fmt.Fprintf(os.Stderr, "Response Status: %s\n", resp.Status)
		return 1
	}

	return 0
}

func printAPIHelp() {
	fmt.Print(`Usage: ductile api <endpoint> [flags]

Directly call the gateway API using your current configuration for URL and token.

Arguments:
  endpoint      The API path (e.g., /jobs, /plugin/echo)

Flags:
  -X, --method  HTTP method (default: GET, or POST if fields/body provided)
  -f, --field   Add a JSON field (key=value). May be used multiple times.
  -H, --header  Add an HTTP header (Header: value). May be used multiple times.
  -b, --body    Raw request body (or - for stdin)
  -c, --config  Path to configuration to load defaults from
  --api-url     Override base API URL
  --api-key     Override API key (defaults to first token in config or DUCTILE_API_KEY)

Examples:
  ductile api /jobs
  ductile api /plugin/echo/poll -f message="hello"
  ductile api /system/reload -X POST
`)
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
	_, _ = fmt.Fprintln(w, "Usage: ductile system <action>")
	_, _ = fmt.Fprintln(w, "Actions: start, status, reset, reload, watch, skills")
}

func printConfigNounHelp(w *os.File) {
	_, _ = fmt.Fprintln(w, "Usage: ductile config <action> [flags]")
	_, _ = fmt.Fprintln(w, "Actions: lock, check, show, get, set, token, scope, plugin, route, webhook, init, backup, restore")
}

func printJobNounHelp(w *os.File) {
	_, _ = fmt.Fprintln(w, "Usage: ductile job <action>")
	_, _ = fmt.Fprintln(w, "Actions: inspect, logs")
}

func printPluginNounHelp(w *os.File) {
	_, _ = fmt.Fprintln(w, "Usage: ductile plugin <action>")
	_, _ = fmt.Fprintln(w, "Actions: list, run")
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

func printSystemResetHelp() {
	fmt.Println("Usage: ductile system reset <plugin> [--config PATH]")
	fmt.Println("Reset scheduler poll circuit breaker state for a plugin.")
}

func printSystemReloadHelp() {
	fmt.Println("Usage: ductile system reload [--config PATH] [--api-url URL] [--api-key TOKEN] [--json]")
	fmt.Println("Reload configuration in a running gateway (SIGHUP or API).")
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
	defer func() { _ = db.Close() }()

	q := queue.New(db)
	if err := q.ResetCircuitBreaker(context.Background(), pluginName, "poll"); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to reset circuit breaker: %v\n", err)
		return 1
	}

	fmt.Printf("Reset circuit breaker for %s (poll)\n", pluginName)
	return 0
}

func runSystemReload(actionArgs []string) int {
	fs := flag.NewFlagSet("reload", flag.ContinueOnError)
	configPath := fs.String("config", "", "Path to configuration file or directory")
	apiURL := fs.String("api-url", "", "API base URL (optional)")
	apiKey := fs.String("api-key", "", "API key (optional)")
	jsonOut := fs.Bool("json", false, "Output JSON response")
	if err := fs.Parse(actionArgs); err != nil {
		fmt.Fprintf(os.Stderr, "Flag error: %v\n", err)
		return 1
	}

	if *apiURL != "" {
		key := strings.TrimSpace(*apiKey)
		if key == "" {
			key = strings.TrimSpace(os.Getenv("DUCTILE_API_KEY"))
		}
		if key == "" {
			fmt.Fprintln(os.Stderr, "API key required for reload (set --api-key or DUCTILE_API_KEY)")
			return 1
		}
		endpoint := strings.TrimRight(*apiURL, "/") + "/system/reload"
		req, err := http.NewRequest(http.MethodPost, endpoint, nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to build request: %v\n", err)
			return 1
		}
		req.Header.Set("Authorization", "Bearer "+key)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Reload request failed: %v\n", err)
			return 1
		}
		defer func() { _ = resp.Body.Close() }()
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK {
			fmt.Fprintf(os.Stderr, "Reload failed (%d): %s\n", resp.StatusCode, strings.TrimSpace(string(body)))
			return 1
		}
		if *jsonOut {
			fmt.Println(string(body))
			return 0
		}
		var result api.ReloadResponse
		if err := json.Unmarshal(body, &result); err == nil && result.Status != "" {
			fmt.Printf("Reloaded at %s\n", result.ReloadedAt)
			if result.Message != "" {
				fmt.Printf("%s\n", result.Message)
			}
			return 0
		}
		fmt.Println(string(body))
		return 0
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
	pidPath := getPIDLockPath(cfg)
	pid, err := readPIDFromFile(pidPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to read PID: %v\n", err)
		return 1
	}
	if err := syscall.Kill(pid, syscall.SIGHUP); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to signal PID %d: %v\n", pid, err)
		return 1
	}
	if *jsonOut {
		resp := api.ReloadResponse{Status: "ok", Message: fmt.Sprintf("SIGHUP sent to %d", pid)}
		raw, _ := json.MarshalIndent(resp, "", "  ")
		fmt.Println(string(raw))
		return 0
	}
	fmt.Printf("Reload signal sent to PID %d\n", pid)
	return 0
}

func buildValidatedGatewayAPIURL(base, endpoint string) (string, error) {
	if strings.TrimSpace(base) == "" {
		return "", fmt.Errorf("base URL is required")
	}
	if endpoint == "" || !strings.HasPrefix(endpoint, "/") || strings.HasPrefix(endpoint, "//") {
		return "", fmt.Errorf("endpoint must be an absolute path")
	}

	baseURL, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("parse base URL: %w", err)
	}
	if baseURL.Scheme != "http" && baseURL.Scheme != "https" {
		return "", fmt.Errorf("scheme must be http or https")
	}
	if baseURL.Hostname() == "" {
		return "", fmt.Errorf("host is required")
	}
	if err := validateGatewayAPIHost(baseURL.Hostname()); err != nil {
		return "", err
	}

	relative, err := url.Parse(endpoint)
	if err != nil {
		return "", fmt.Errorf("parse endpoint: %w", err)
	}
	if relative.IsAbs() || relative.Host != "" {
		return "", fmt.Errorf("endpoint must not include a host")
	}

	baseURL.Path = path.Join(baseURL.Path, relative.Path)
	if strings.HasSuffix(relative.Path, "/") && !strings.HasSuffix(baseURL.Path, "/") {
		baseURL.Path += "/"
	}
	baseURL.RawQuery = relative.RawQuery
	baseURL.Fragment = ""
	return baseURL.String(), nil
}

func validateGatewayAPIHost(host string) error {
	if strings.EqualFold(host, "localhost") {
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return fmt.Errorf("host must be localhost or an IP address")
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() {
		return nil
	}
	return fmt.Errorf("host must be loopback, private, or link-local")
}

func runSystemSkills(args []string) int {
	fs := flag.NewFlagSet("skills", flag.ContinueOnError)
	configPath := fs.String("config", "", "Path to configuration file or directory")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "Flag error: %v\n", err)
		return 1
	}

	explicitConfig := *configPath != ""

	// Try to auto-discover config if not specified; failure is non-fatal.
	if *configPath == "" {
		if discovered, err := config.DiscoverConfigDir(); err == nil {
			*configPath = discovered
		}
	}

	// Attempt to load config and registry.
	var registry *plugin.Registry
	var loadedConfig *config.Config
	hasConfig := false
	if *configPath != "" {
		cfg, err := config.Load(*configPath)
		if err != nil {
			if explicitConfig {
				fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
				return 1
			}
			// Auto-discovered config failed to load — fall through to core mode.
		} else {
			if r, err := discoverRegistry(cfg, *configPath); err == nil {
				registry = r
				loadedConfig = cfg
				hasConfig = true
			}
		}
	}

	if !hasConfig {
		fmt.Print(skillsCoreMode)
		return 0
	}

	// Full manifest output.
	fmt.Println("# Ductile Gateway: LLM Operator Skill Manifest")
	fmt.Println()
	fmt.Print(skillsCLICommands)
	fmt.Println()

	// --- Plugin Skills ---
	plugins := registry.All()
	var pNames []string
	for name := range plugins {
		pNames = append(pNames, name)
	}
	sort.Strings(pNames)

	fmt.Println("## Plugins")
	fmt.Println()
	fmt.Println("Format: `<plugin>.<command> m=<HTTP> p=<path> tier=<READ|WRITE> mut=<0|1> idem=<0|1> retry=<0|1> [in=<schema>] [out=<schema>] [d=<desc>]`")
	fmt.Println()

	for _, name := range pNames {
		p := plugins[name]
		fmt.Printf("### %s\n", p.Name)
		if p.Description != "" {
			fmt.Println()
			fmt.Println(p.Description)
		}
		fmt.Println()
		for _, cmd := range p.Commands {
			mutatesState := cmd.Type == plugin.CommandTypeWrite
			idempotent := !mutatesState
			if cmd.Idempotent != nil {
				idempotent = *cmd.Idempotent
			}
			retrySafe := !mutatesState
			if cmd.RetrySafe != nil {
				retrySafe = *cmd.RetrySafe
			}
			tier := "READ"
			if mutatesState {
				tier = "WRITE"
			}
			fmt.Printf("- %s.%s m=POST p=/plugin/%s/%s tier=%s mut=%d idem=%d retry=%d",
				p.Name, cmd.Name, p.Name, cmd.Name, tier, boolToInt(mutatesState), boolToInt(idempotent), boolToInt(retrySafe))
			if s := renderSchema(cmd.InputSchema); s != "" {
				fmt.Printf(" in=%q", s)
			}
			if s := renderSchema(cmd.OutputSchema); s != "" {
				fmt.Printf(" out=%q", s)
			}
			if cmd.Description != "" {
				fmt.Printf(" d=%q", cmd.Description)
			}
			fmt.Println()
		}
		fmt.Println()
	}

	// --- Pipeline Skills ---
	if loadedConfig != nil {
		pipelineFiles := make([]string, 0, len(loadedConfig.SourceFiles))
		for f := range loadedConfig.SourceFiles {
			pipelineFiles = append(pipelineFiles, f)
		}
		sort.Strings(pipelineFiles)

		routerEngine, err := router.LoadFromConfigFiles(pipelineFiles, registry, log.WithComponent("skills-export"))
		if err == nil {
			if r, ok := routerEngine.(*router.Router); ok {
				pipelines := r.PipelineSummary()
				if len(pipelines) > 0 {
					sort.Slice(pipelines, func(i, j int) bool {
						return pipelines[i].Name < pipelines[j].Name
					})
					fmt.Println("## Pipelines")
					fmt.Println()
					fmt.Println("Format: `<pipeline> m=<HTTP> p=<path> trigger=<trigger> mode=<sync|async> [timeout=<duration>]`")
					fmt.Println()
					for _, p := range pipelines {
						mode := "async"
						if p.ExecutionMode == "synchronous" {
							mode = "sync"
						}
						fmt.Printf("- %s m=POST p=/pipeline/%s trigger=%q mode=%s", p.Name, p.Name, p.Trigger, mode)
						if p.Timeout > 0 {
							fmt.Printf(" timeout=%s", p.Timeout)
						}
						fmt.Println()
					}
				}
			}
		}
	}

	fmt.Println("---")
	fmt.Println()
	fmt.Println("**Next steps:** Use `job inspect <id>` to trace any execution. Use `system status` to verify health.")

	return 0
}

// renderSchema formats a plugin command's raw schema for manifest display.
// Compact map {prop: "type"} → "{key: type, ...}" (sorted keys).
// Full JSON schema (has "type" key) → compact JSON.
// nil → "" (omit field).
func renderSchema(schema any) string {
	if schema == nil {
		return ""
	}
	m, ok := schema.(map[string]any)
	if !ok {
		b, err := json.Marshal(schema)
		if err != nil {
			return ""
		}
		return string(b)
	}
	// Full JSON schema: has a "type" key at the top level.
	if _, hasType := m["type"]; hasType {
		b, err := json.Marshal(schema)
		if err != nil {
			return ""
		}
		return string(b)
	}
	// Compact map {prop: "type"} — render sorted.
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s: %v", k, m[k]))
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func runWatch(args []string) int {
	fs := flag.NewFlagSet("watch", flag.ExitOnError)
	apiURL := fs.String("api-url", "http://localhost:8080", "Gateway API URL")
	apiKey := fs.String("api-key", os.Getenv("DUCTILE_API_KEY"), "API Bearer Token")
	configPath := fs.String("config", "", "Path to configuration file or directory")
	configDir := fs.String("config-dir", "", "Path to configuration directory")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "Flag error: %v\n", err)
		return 1
	}

	if *apiKey == "" {
		fmt.Fprintln(os.Stderr, "Error: API key required. Use --api-key or DUCTILE_API_KEY env var.")
		return 1
	}

	resolvedConfigPath := strings.TrimSpace(*configPath)
	if strings.TrimSpace(*configDir) != "" {
		resolvedConfigPath = strings.TrimSpace(*configDir)
	}

	var cfg *config.Config
	if resolvedConfigPath == "" {
		if discovered, err := config.DiscoverConfigDir(); err == nil {
			resolvedConfigPath = discovered
		}
	}

	if resolvedConfigPath != "" {
		loaded, err := config.Load(resolvedConfigPath)
		if err != nil {
			if *configPath != "" || *configDir != "" {
				fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
				return 1
			}
			fmt.Fprintf(os.Stderr, "Warning: unable to load config from %s: %v\n", resolvedConfigPath, err)
		} else {
			cfg = loaded
		}
	}

	m := watch.New(*apiURL, *apiKey, cfg)
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
	fmt.Println("  --api-url URL      Gateway API URL (default: http://localhost:8080)")
	fmt.Println("  --api-key KEY      API Bearer Token (or DUCTILE_API_KEY env var)")
	fmt.Println("  --config PATH      Path to configuration file or directory")
	fmt.Println("  --config-dir PATH  Path to configuration directory")
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

func printJobLogsHelp() {
	fmt.Println("Usage: ductile job logs [--config PATH] [--json] [--plugin NAME] [--command CMD] [--status STATUS] [--submitted-by NAME] [--from TIME] [--to TIME] [--query TEXT] [--limit N] [--include-result]")
	fmt.Println("Query stored job logs for audit and troubleshooting.")
	fmt.Println("Time values must be RFC3339 (e.g. 2025-01-02T15:04:05Z).")
}

func printPluginListHelp() {
	fmt.Println("Usage: ductile plugin list [--api-url URL] [--json]")
	fmt.Println("Show discovered plugins via the API /plugins endpoint.")
}

func printPluginRunHelp() {
	fmt.Println("Usage: ductile plugin run <name> [--command CMD] [--payload JSON] [--payload-file PATH] [--api-url URL] [--api-key KEY] [--json]")
	fmt.Println("Execute a plugin command via the API /plugin/{name}/{command} endpoint.")
}

// --- ACTION IMPLEMENTATIONS ---

type pluginListResponse struct {
	Plugins []struct {
		Name        string   `json:"name"`
		Version     string   `json:"version"`
		Description string   `json:"description"`
		Commands    []string `json:"commands"`
	} `json:"plugins"`
}

type triggerRequest struct {
	Payload json.RawMessage `json:"payload,omitempty"`
}

type triggerResponse struct {
	JobID   string `json:"job_id"`
	Status  string `json:"status"`
	Plugin  string `json:"plugin"`
	Command string `json:"command"`
}

func buildAPIURL(base, path string) string {
	return strings.TrimRight(base, "/") + path
}

func runPluginList(args []string) int {
	fs := flag.NewFlagSet("plugin list", flag.ContinueOnError)
	apiURL := fs.String("api-url", "http://localhost:8080", "Gateway API URL")
	jsonOut := fs.Bool("json", false, "Output JSON")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "Flag error: %v\n", err)
		return 1
	}
	if fs.NArg() > 0 {
		printPluginListHelp()
		return 1
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(buildAPIURL(*apiURL, "/plugins"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "API request failed: %v\n", err)
		return 1
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to read response: %v\n", err)
		return 1
	}
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "API error (%d): %s\n", resp.StatusCode, strings.TrimSpace(string(body)))
		return 1
	}

	if *jsonOut {
		fmt.Println(string(body))
		return 0
	}

	var list pluginListResponse
	if err := json.Unmarshal(body, &list); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to parse response: %v\n", err)
		return 1
	}

	nameWidth := len("NAME")
	for _, p := range list.Plugins {
		if len(p.Name) > nameWidth {
			nameWidth = len(p.Name)
		}
	}

	fmt.Printf("%-*s  %-8s  %s\n", nameWidth, "NAME", "VERSION", "COMMANDS")
	for _, p := range list.Plugins {
		commands := strings.Join(p.Commands, ",")
		fmt.Printf("%-*s  %-8s  %s\n", nameWidth, p.Name, p.Version, commands)
		if strings.TrimSpace(p.Description) != "" {
			fmt.Printf("%*s  %s\n", nameWidth, "", p.Description)
		}
	}
	return 0
}

func runPluginRun(args []string) int {
	fs := flag.NewFlagSet("plugin run", flag.ContinueOnError)
	command := fs.String("command", "poll", "Plugin command to run")
	payloadRaw := fs.String("payload", "", "JSON payload to send")
	payloadFile := fs.String("payload-file", "", "Path to JSON payload file")
	apiURL := fs.String("api-url", "http://localhost:8080", "Gateway API URL")
	apiKey := fs.String("api-key", os.Getenv("DUCTILE_API_KEY"), "API Bearer Token")
	jsonOut := fs.Bool("json", false, "Output JSON response")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "Flag error: %v\n", err)
		return 1
	}
	if fs.NArg() != 1 {
		printPluginRunHelp()
		return 1
	}
	if *apiKey == "" {
		fmt.Fprintln(os.Stderr, "Error: API key required. Use --api-key or DUCTILE_API_KEY env var.")
		return 1
	}
	if *payloadRaw != "" && *payloadFile != "" {
		fmt.Fprintln(os.Stderr, "Error: use only one of --payload or --payload-file")
		return 1
	}

	pluginName := fs.Arg(0)
	cmd := strings.TrimSpace(*command)
	if cmd == "" {
		fmt.Fprintln(os.Stderr, "Error: --command is required")
		return 1
	}

	var payload json.RawMessage
	if *payloadFile != "" {
		data, err := os.ReadFile(*payloadFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to read payload file: %v\n", err)
			return 1
		}
		payload = json.RawMessage(bytes.TrimSpace(data))
	} else if *payloadRaw != "" {
		payload = json.RawMessage(strings.TrimSpace(*payloadRaw))
	}

	var body io.Reader
	if len(payload) > 0 {
		if !json.Valid(payload) {
			fmt.Fprintln(os.Stderr, "Error: payload must be valid JSON")
			return 1
		}
		var payloadObj any
		if err := json.Unmarshal(payload, &payloadObj); err != nil {
			fmt.Fprintf(os.Stderr, "Error: invalid JSON payload: %v\n", err)
			return 1
		}
		if payloadObj != nil {
			if _, ok := payloadObj.(map[string]any); !ok {
				fmt.Fprintln(os.Stderr, "Error: payload must be a JSON object")
				return 1
			}
		}
		reqBody, err := json.Marshal(triggerRequest{Payload: payload})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to build request: %v\n", err)
			return 1
		}
		body = bytes.NewBuffer(reqBody)
	}

	endpoint := fmt.Sprintf("/plugin/%s/%s", pluginName, cmd)
	req, err := http.NewRequest("POST", buildAPIURL(*apiURL, endpoint), body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create request: %v\n", err)
		return 1
	}
	req.Header.Set("Authorization", "Bearer "+*apiKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "API request failed: %v\n", err)
		return 1
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to read response: %v\n", err)
		return 1
	}
	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "API error (%d): %s\n", resp.StatusCode, strings.TrimSpace(string(respBody)))
		return 1
	}

	if *jsonOut {
		fmt.Println(string(respBody))
		return 0
	}

	var result triggerResponse
	if err := json.Unmarshal(respBody, &result); err != nil || result.JobID == "" {
		fmt.Println(string(respBody))
		return 0
	}
	fmt.Printf("Queued job %s (%s %s)\n", result.JobID, result.Plugin, result.Command)
	return 0
}

type runtimeState struct {
	cfg                    *config.Config
	configPath             string
	logger                 *slog.Logger
	registry               *plugin.Registry
	router                 router.Engine
	scheduler              *scheduler.Scheduler
	dispatcher             *dispatch.Dispatcher
	apiServer              *api.Server
	webhook                *webhook.Server
	ctx                    context.Context
	cancel                 context.CancelFunc
	wg                     sync.WaitGroup
	stopOnce               sync.Once
	stopDone               chan struct{}
	errCh                  chan error
	db                     *sql.DB
	configSource           string
	activeConfigSnapshotID string
}

type reloadManager struct {
	mu           sync.Mutex
	configPath   string
	configSource string
	runtime      *runtimeState
	errCh        chan error
	reloadFunc   func(context.Context) (api.ReloadResponse, error)
}

type runtimeBuildOptions struct {
	snapshotReason     string
	existingSnapshotID string
}

func (rt *runtimeState) Stop() {
	if rt == nil {
		return
	}
	if rt.stopDone == nil {
		rt.stopDone = make(chan struct{})
	}
	rt.stopOnce.Do(func() {
		defer close(rt.stopDone)
		if rt.cancel != nil {
			rt.cancel()
		}
		if rt.scheduler != nil {
			rt.scheduler.Stop()
		}
		rt.wg.Wait()
		if rt.db != nil {
			_ = rt.db.Close()
		}
	})
	<-rt.stopDone
}

func (rt *runtimeState) WaitListenersStopped(ctx context.Context) error {
	if rt == nil {
		return nil
	}
	if rt.apiServer != nil {
		if err := rt.apiServer.WaitServeStopped(ctx); err != nil {
			return fmt.Errorf("api listener stopped: %w", err)
		}
	}
	if rt.webhook != nil {
		if err := rt.webhook.WaitServeStopped(ctx); err != nil {
			return fmt.Errorf("webhook listener stopped: %w", err)
		}
	}
	return nil
}

func newRuntimeContext() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancelCause(context.Background())
	return ctx, func() {
		cancel(nil)
	}
}

func (rm *reloadManager) Stop() {
	rm.mu.Lock()
	rt := rm.runtime
	rm.runtime = nil
	rm.mu.Unlock()
	if rt == nil {
		return
	}
	rt.Stop()
}

func (rm *reloadManager) Reload(ctx context.Context) (api.ReloadResponse, error) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	oldRuntime := rm.runtime
	if oldRuntime == nil {
		return api.ReloadResponse{Status: "error", Message: "runtime not available"}, fmt.Errorf("runtime not available")
	}
	oldCfg := oldRuntime.cfg

	newCfg, err := config.Load(rm.configPath)
	if err != nil {
		return api.ReloadResponse{Status: "error", Message: err.Error()}, err
	}
	if err := verifyReloadIntegrity(rm.configPath); err != nil {
		return api.ReloadResponse{Status: "error", Message: err.Error()}, err
	}
	if err := validateReloadableFields(oldCfg, newCfg); err != nil {
		return api.ReloadResponse{Status: "error", Message: err.Error()}, err
	}

	oldRuntime.logger.Info("reloading config")

	if ctx == nil {
		ctx = context.Background()
	}

	go oldRuntime.Stop()

	listenerCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := oldRuntime.WaitListenersStopped(listenerCtx); err != nil {
		rm.runtime = nil
		return api.ReloadResponse{Status: "error", Message: err.Error()}, err
	}

	runtime, err := buildRuntime(newCfg, rm.configPath, rm.configSource, rm.reloadFunc, rm.errCh, runtimeBuildOptions{
		snapshotReason: configsnapshot.ReasonReload,
	})
	if err != nil {
		oldRuntime.logger.Error("reload failed; attempting to restore previous runtime", "error", err)
		restored, restoreErr := buildRuntime(oldCfg, rm.configPath, rm.configSource, rm.reloadFunc, rm.errCh, runtimeBuildOptions{
			existingSnapshotID: oldRuntime.activeConfigSnapshotID,
		})
		if restoreErr == nil {
			rm.runtime = restored
		} else {
			rm.runtime = nil
			err = fmt.Errorf("reload failed: %w; restore previous runtime: %v", err, restoreErr)
		}
		return api.ReloadResponse{Status: "error", Message: err.Error()}, err
	}

	rm.runtime = runtime

	return api.ReloadResponse{
		Status:     "ok",
		ReloadedAt: time.Now().UTC().Format(time.RFC3339),
		Message:    "configuration reloaded",
	}, nil
}

func validateReloadableFields(oldCfg, newCfg *config.Config) error {
	if oldCfg.State.Path != newCfg.State.Path {
		return fmt.Errorf("config reload rejected: changes to state.path require a full restart")
	}
	if oldCfg.API.Listen != newCfg.API.Listen {
		return fmt.Errorf("config reload rejected: changes to api.listen require a full restart")
	}
	oldWebhookListen := ""
	newWebhookListen := ""
	if oldCfg.Webhooks != nil {
		oldWebhookListen = oldCfg.Webhooks.Listen
	}
	if newCfg.Webhooks != nil {
		newWebhookListen = newCfg.Webhooks.Listen
	}
	if oldWebhookListen != newWebhookListen {
		return fmt.Errorf("config reload rejected: changes to webhooks.listen require a full restart")
	}
	return nil
}

func resolveConfigDir(configPath string) string {
	configDir := configPath
	if stat, err := os.Stat(configPath); err == nil && !stat.IsDir() {
		configDir = filepath.Dir(configPath)
	}
	return configDir
}

func verifyReloadIntegrity(configPath string) error {
	configDir := resolveConfigDir(configPath)
	files, err := config.DiscoverConfigFiles(configDir)
	if err != nil {
		return fmt.Errorf("config reload rejected: unlocked changes detected")
	}
	result, err := config.VerifyIntegrity(configDir, files)
	if err != nil || !result.Passed {
		return fmt.Errorf("config reload rejected: unlocked changes detected")
	}
	if err := verifyPluginFingerprintsForConfig(configPath); err != nil {
		return fmt.Errorf("config reload rejected: %v", err)
	}
	return nil
}

func loadLockedPluginFingerprints(configPath string) []config.PluginFingerprint {
	manifest, err := config.LoadChecksums(resolveConfigDir(configPath))
	if err != nil || len(manifest.PluginFingerprints) == 0 {
		return nil
	}
	fingerprints := append([]config.PluginFingerprint(nil), manifest.PluginFingerprints...)
	sort.Slice(fingerprints, func(i, j int) bool {
		return fingerprints[i].Name < fingerprints[j].Name
	})
	return fingerprints
}

func snapshotVersion() string {
	v := strings.TrimSpace(version)
	commit := strings.TrimSpace(gitCommit)
	if commit != "" && commit != "unknown" {
		return v + "+commit." + commit
	}
	return v
}

func binaryPath() string {
	path, err := os.Executable()
	if err != nil {
		return ""
	}
	return path
}

func buildRuntime(cfg *config.Config, configPath string, configSource string, reloadFunc func(context.Context) (api.ReloadResponse, error), errCh chan error, opts runtimeBuildOptions) (*runtimeState, error) {
	log.Setup(cfg.Service.LogLevel)
	logger := log.WithComponent("main")

	configPaths, err := config.CollectConfigPaths(configPath, cfg)
	if err != nil {
		logger.Error("config symlink scan failed", "error", err)
		return nil, err
	}
	symlinkWarnings, err := config.DetectSymlinks(configPaths)
	if err != nil {
		logger.Error("config symlink scan failed", "error", err)
		return nil, err
	}
	for _, warning := range symlinkWarnings {
		logger.Warn("symlink detected", "path", warning.Path, "resolved", warning.Resolved)
	}
	if len(symlinkWarnings) > 0 && !cfg.Service.AllowSymlinks {
		return nil, fmt.Errorf("symlinks detected in config paths but not allowed")
	}

	pluginRoots, err := resolvePluginRoots(cfg, configPath)
	if err != nil {
		logger.Error("plugin root resolution failed", "error", err)
		return nil, err
	}
	registry, err := plugin.DiscoverManyWithOptions(pluginRoots, func(level, msg string, args ...any) {
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
	}, plugin.DiscoverOptions{AllowSymlinks: cfg.Service.AllowSymlinks})
	if err != nil {
		logger.Error("plugin discovery failed", "plugin_roots", pluginRoots, "error", err)
		return nil, err
	}
	aliases, err := plugin.ApplyAliases(registry, cfg.Plugins)
	if err != nil {
		logger.Error("plugin aliasing failed", "error", err)
		return nil, err
	}
	for _, alias := range aliases {
		logger.Info("plugin alias registered", "plugin", alias.Name, "uses", alias.Uses)
	}

	// Preflight: report which config files were loaded
	{
		logger.Info("config loaded", "path", configPath, "source", configSource)

		configDir := resolveConfigDir(configPath)

		var sourceFiles []string
		for f := range cfg.SourceFiles {
			sourceFiles = append(sourceFiles, f)
		}
		sort.Strings(sourceFiles)
		for _, f := range sourceFiles {
			rel, err := filepath.Rel(configDir, f)
			if err != nil || strings.HasPrefix(rel, "..") {
				rel = f
			}
			logger.Info("config file", "file", rel)
		}

		pluginsConfigured := len(cfg.Plugins)
		pluginsEnabled := 0
		for _, p := range cfg.Plugins {
			if p.Enabled {
				pluginsEnabled++
			}
		}
		logger.Info("config summary",
			"plugins_discovered", len(registry.All()),
			"plugins_configured", pluginsConfigured,
			"plugins_enabled", pluginsEnabled,
			"api_listen", cfg.API.Listen,
		)
	}

	// Strict mode enforcement
	if cfg.Service.StrictMode {
		logger.Info("strict mode enabled, performing pre-flight checks")

		configDir := resolveConfigDir(configPath)
		files, err := config.DiscoverConfigFiles(configDir)
		if err == nil {
			result, err := config.VerifyIntegrity(configDir, files)
			if err != nil || !result.Passed {
				logger.Error("integrity check failed (strict mode)", "errors", result.Errors)
				return nil, fmt.Errorf("integrity check failed")
			}
		}

		if err := verifyPluginFingerprintsForConfig(configPath); err != nil {
			logger.Error("plugin fingerprint check failed (strict mode)", "error", err)
			return nil, fmt.Errorf("plugin fingerprint check failed: %w", err)
		}

		doc := doctor.New(cfg, registry)
		report := doc.Validate()
		if !report.Valid {
			logger.Error("configuration validation failed (strict mode)")
			for _, e := range report.Errors {
				logger.Error("config error", "detail", e)
			}
			return nil, fmt.Errorf("configuration validation failed")
		}

		if cfg.API.Enabled && len(cfg.API.Auth.Tokens) == 0 {
			logger.Error("no API tokens configured (strict mode requires at least one token when API is enabled)")
			return nil, fmt.Errorf("no API tokens configured")
		}
	}

	logger.Info("ductile starting", "version", version, "config", configPath)

	ctx := context.Background()
	db, err := storage.OpenSQLite(ctx, cfg.State.Path)
	if err != nil {
		logger.Error("failed to open database", "path", cfg.State.Path, "error", err)
		return nil, err
	}
	logger.Info("database opened", "path", cfg.State.Path)

	logger.Info("plugin discovery complete", "count", len(registry.All()))
	if err := validateScheduledCommands(cfg, registry); err != nil {
		logger.Error("invalid scheduled command configuration", "error", err)
		return nil, err
	}

	configDir := configPath
	if stat, err := os.Stat(configDir); err != nil || !stat.IsDir() {
		configDir = filepath.Dir(configPath)
	}

	wsBaseDir := filepath.Join(filepath.Dir(cfg.State.Path), "workspaces")
	wsManager, err := workspace.NewFSManager(wsBaseDir)
	if err != nil {
		logger.Error("failed to initialize workspace manager", "base_dir", wsBaseDir, "error", err)
		return nil, err
	}

	pipelineFiles := make([]string, 0, len(cfg.SourceFiles))
	for f := range cfg.SourceFiles {
		pipelineFiles = append(pipelineFiles, f)
	}
	sort.Strings(pipelineFiles)

	routerEngine, err := router.LoadFromConfigFiles(pipelineFiles, registry, logger)
	if err != nil {
		logger.Error("failed to load router pipelines", "config_dir", configDir, "error", err)
		return nil, err
	}
	if r, ok := routerEngine.(*router.Router); ok {
		pipelines := r.PipelineSummary()
		logger.Info("pipeline discovery complete", "config_dir", configDir, "pipelines_loaded", len(pipelines))
		for _, p := range pipelines {
			logger.Info("pipeline registered", "name", p.Name, "trigger", p.Trigger)
		}
	}

	activeSnapshotID := strings.TrimSpace(opts.existingSnapshotID)
	if activeSnapshotID == "" {
		reason := opts.snapshotReason
		if reason == "" {
			reason = configsnapshot.ReasonStartup
		}
		pluginFingerprints := loadLockedPluginFingerprints(configPath)
		snapshot, err := configsnapshot.Build(configsnapshot.BuildInput{
			Config:             cfg,
			ConfigPath:         configPath,
			ConfigSource:       configSource,
			Reason:             reason,
			DuctileVersion:     snapshotVersion(),
			BinaryPath:         binaryPath(),
			PluginFingerprints: pluginFingerprints,
		})
		if err != nil {
			logger.Error("failed to build config snapshot", "error", err)
			return nil, err
		}
		if err := configsnapshot.Insert(ctx, db, snapshot); err != nil {
			logger.Error("failed to store config snapshot", "error", err)
			return nil, err
		}
		activeSnapshotID = snapshot.ID
		logger.Info("config snapshot recorded", "snapshot_id", activeSnapshotID, "reason", reason, "config_hash", snapshot.ConfigHash)
	}

	q := queue.New(
		db,
		queue.WithLogger(logger),
		queue.WithDedupeTTL(cfg.Service.DedupeTTL),
		queue.WithConfigSnapshotIDProvider(func() string {
			return activeSnapshotID
		}),
	)
	st := state.NewStore(db)
	contextStore := state.NewContextStore(db)
	hub := events.NewHub(256)

	rt := &runtimeState{
		cfg:                    cfg,
		configPath:             configPath,
		logger:                 logger,
		registry:               registry,
		router:                 routerEngine,
		stopDone:               make(chan struct{}),
		errCh:                  errCh,
		db:                     db,
		configSource:           configSource,
		activeConfigSnapshotID: activeSnapshotID,
	}

	rt.ctx, rt.cancel = newRuntimeContext()

	sched := scheduler.New(cfg, q, hub, logger,
		scheduler.WithCommandSupportChecker(func(pluginName, commandName string) bool {
			plug, ok := registry.Get(pluginName)
			if !ok {
				return false
			}
			return plug.SupportsCommand(commandName)
		}),
		scheduler.WithWorkspaceJanitor(wsManager, cfg.Workspace.TTL, cfg.Workspace.JanitorInterval),
	)
	rt.scheduler = sched
	disp := dispatch.New(q, st, contextStore, wsManager, routerEngine, registry, hub, cfg)
	rt.dispatcher = disp

	// Wire recovery hooks: when the scheduler marks a dead orphan during crash
	// recovery, delegate to the dispatcher's hook-firing machinery so on-hook
	// pipelines (e.g. job-failure-notify → discord_notify) are triggered.
	sched.SetRecoveryHook(disp.FireRecoveryHook)

	if err := sched.Start(rt.ctx); err != nil && err != context.Canceled {
		return nil, fmt.Errorf("scheduler: %w", err)
	}

	rt.wg.Add(1)
	go func() {
		defer rt.wg.Done()
		if err := disp.Start(rt.ctx); err != nil && err != context.Canceled {
			rt.errCh <- fmt.Errorf("dispatcher: %w", err)
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
		binaryPath := ""
		if execPath, err := os.Executable(); err == nil {
			binaryPath = execPath
		}

		apiConfig := api.Config{
			Listen:            cfg.API.Listen,
			Tokens:            tokens,
			MaxConcurrentSync: cfg.API.MaxConcurrentSync,
			MaxSyncTimeout:    cfg.API.MaxSyncTimeout,
			ConfigPath:        configPath,
			BinaryPath:        binaryPath,
			Version:           version,
			RuntimeConfig:     cfg,
			ReloadFunc:        reloadFunc,
		}
		apiServer := api.New(apiConfig, q, registry, routerEngine, disp, contextStore, hub, log.WithComponent("api"))
		rt.apiServer = apiServer
		rt.wg.Add(1)
		go func() {
			defer rt.wg.Done()
			if err := apiServer.Start(rt.ctx); err != nil && err != context.Canceled {
				rt.errCh <- fmt.Errorf("api: %w", err)
			}
		}()
		logger.Info("API server enabled", "listen", cfg.API.Listen)
	}

	if cfg.Webhooks != nil && len(cfg.Webhooks.Endpoints) > 0 {
		tokensMap := make(map[string]string, len(cfg.Tokens))
		for _, t := range cfg.Tokens {
			tokensMap[t.Name] = t.Key
		}
		webhookConfig, err := webhook.FromGlobalConfig(cfg.Webhooks, tokensMap)
		if err != nil {
			logger.Error("failed to configure webhooks", "error", err)
			return nil, err
		}

		webhookServer := webhook.New(webhookConfig, q, log.WithComponent("webhook"))
		rt.webhook = webhookServer
		rt.wg.Add(1)
		go func() {
			defer rt.wg.Done()
			if err := webhookServer.Start(rt.ctx); err != nil && err != context.Canceled {
				rt.errCh <- fmt.Errorf("webhook: %w", err)
			}
		}()
		logger.Info("webhook server enabled", "listen", webhookConfig.Listen, "endpoints", len(webhookConfig.Endpoints))
	}

	return rt, nil
}

func runStart(args []string) int {
	fs := flag.NewFlagSet("start", flag.ExitOnError)
	configPath := fs.String("config", "", "Path to configuration file or directory")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to parse flags: %v\n", err)
		return 1
	}

	configSource := "explicit"
	if *configPath == "" {
		if os.Getenv("DUCTILE_CONFIG_DIR") != "" {
			configSource = "env:DUCTILE_CONFIG_DIR"
		} else {
			configSource = "auto-discovered"
		}
		discovered, err := config.DiscoverConfigDir()
		if err != nil {
			fmt.Fprintf(os.Stderr, "no config found: %v\nHint: create ~/.config/ductile/config.yaml or use --config\n", err)
			return 1
		}
		*configPath = discovered
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		return 1
	}

	pidLockPath := getPIDLockPath(cfg)
	pidLock, err := lock.AcquirePIDLock(pidLockPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to acquire PID lock (another instance may be running): %v\n", err)
		return 1
	}
	defer func() { _ = pidLock.Release() }()

	manager := &reloadManager{
		configPath:   *configPath,
		configSource: configSource,
		errCh:        make(chan error, 4),
	}
	manager.reloadFunc = manager.Reload

	runtime, err := buildRuntime(cfg, *configPath, configSource, manager.reloadFunc, manager.errCh, runtimeBuildOptions{
		snapshotReason: configsnapshot.ReasonStartup,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start runtime: %v\n", err)
		return 1
	}
	manager.runtime = runtime

	logger := runtime.logger
	logger.Info("acquired PID lock", "path", pidLockPath)
	logger.Info("ductile running (press Ctrl+C to stop)")

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	for {
		select {
		case sig := <-sigCh:
			if sig == syscall.SIGHUP {
				if _, err := manager.Reload(context.Background()); err != nil {
					logger.Error("config reload failed", "error", err)
				} else {
					logger.Info("config reloaded successfully")
				}
				continue
			}
			logger.Info("received shutdown signal", "signal", sig)
			manager.Stop()
			logger.Info("ductile stopped")
			return 0
		case err := <-manager.errCh:
			logger.Error("component failed", "error", err)
			manager.Stop()
			logger.Info("ductile stopped")
			return 1
		}
	}
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

func runInspect(args []string) int {
	// Custom flag parsing because we want to support flags intermixed with the job ID
	// like 'ductile job inspect <id> --json' or 'ductile job inspect --json <id>'
	var configPath string
	var jsonOut bool

	fs := flag.NewFlagSet("inspect", flag.ContinueOnError)
	fs.StringVar(&configPath, "config", "", "Path to configuration")
	fs.BoolVar(&jsonOut, "json", false, "Output report in JSON")

	var jobID string
	remainingArgs := args
	for len(remainingArgs) > 0 {
		if err := fs.Parse(remainingArgs); err != nil {
			fmt.Fprintf(os.Stderr, "Flag error: %v\n", err)
			return 1
		}
		if fs.NArg() > 0 {
			if jobID == "" {
				jobID = fs.Arg(0)
			}
			remainingArgs = fs.Args()[1:]
		} else {
			remainingArgs = nil
		}
	}

	if jobID == "" {
		fmt.Fprintf(os.Stderr, "Usage: ductile job inspect <job_id> [--config PATH] [--json]\n")
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
	defer func() { _ = db.Close() }()

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

func runJobLogs(args []string) int {
	fs := flag.NewFlagSet("job logs", flag.ContinueOnError)
	configPath := fs.String("config", "", "Path to configuration")
	jsonOut := fs.Bool("json", false, "Output JSON")
	plugin := fs.String("plugin", "", "Filter by plugin")
	command := fs.String("command", "", "Filter by command")
	statusRaw := fs.String("status", "", "Filter by status")
	submittedBy := fs.String("submitted-by", "", "Filter by submitted_by")
	fromRaw := fs.String("from", "", "Filter by completed_at >= from (RFC3339)")
	toRaw := fs.String("to", "", "Filter by completed_at <= to (RFC3339)")
	query := fs.String("query", "", "Search last_error/stderr/result")
	limit := fs.Int("limit", 50, "Max rows (<=200)")
	includeResult := fs.Bool("include-result", false, "Include full result payloads")

	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "Flag error: %v\n", err)
		return 1
	}
	if fs.NArg() > 0 {
		printJobLogsHelp()
		return 1
	}
	if *limit <= 0 || *limit > 200 {
		fmt.Fprintln(os.Stderr, "limit must be between 1 and 200")
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

	db, err := storage.OpenSQLite(context.Background(), cfg.State.Path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open database: %v\n", err)
		return 1
	}
	defer func() { _ = db.Close() }()

	filter := queue.JobLogFilter{
		Plugin:        strings.TrimSpace(*plugin),
		Command:       strings.TrimSpace(*command),
		SubmittedBy:   strings.TrimSpace(*submittedBy),
		Query:         strings.TrimSpace(*query),
		Limit:         *limit,
		IncludeResult: *includeResult,
	}

	if strings.TrimSpace(*statusRaw) != "" {
		status, ok := parseJobStatusFlag(*statusRaw)
		if !ok {
			fmt.Fprintln(os.Stderr, "invalid status filter")
			return 1
		}
		filter.Status = &status
	}

	if strings.TrimSpace(*fromRaw) != "" {
		parsed, err := parseTimeFlag(*fromRaw)
		if err != nil {
			fmt.Fprintln(os.Stderr, "invalid from timestamp")
			return 1
		}
		filter.Since = &parsed
	}

	if strings.TrimSpace(*toRaw) != "" {
		parsed, err := parseTimeFlag(*toRaw)
		if err != nil {
			fmt.Fprintln(os.Stderr, "invalid to timestamp")
			return 1
		}
		filter.Until = &parsed
	}

	q := queue.New(db)
	logs, total, err := q.ListJobLogs(context.Background(), filter)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to query job logs: %v\n", err)
		return 1
	}

	if *jsonOut {
		out := struct {
			Total int                  `json:"total"`
			Logs  []*queue.JobLogEntry `json:"logs"`
		}{
			Total: total,
			Logs:  logs,
		}
		payload, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to marshal JSON: %v\n", err)
			return 1
		}
		fmt.Println(string(payload))
		return 0
	}

	fmt.Printf("Job Logs (total=%d)\n", total)
	for _, entry := range logs {
		fmt.Printf("- %s %s %s:%s job=%s attempt=%d submitted_by=%s\n", entry.CompletedAt.Format(time.RFC3339), entry.Status, entry.Plugin, entry.Command, entry.JobID, entry.Attempt, entry.SubmittedBy)
		if entry.LastError != nil {
			fmt.Printf("  last_error: %s\n", *entry.LastError)
		}
		if entry.Stderr != nil {
			fmt.Printf("  stderr: %s\n", *entry.Stderr)
		}
		if *includeResult && len(entry.Result) > 0 {
			fmt.Printf("  result: %s\n", string(entry.Result))
		}
	}

	return 0
}

func parseJobStatusFlag(raw string) (queue.Status, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "pending":
		return queue.StatusQueued, true
	case "ok":
		return queue.StatusSucceeded, true
	case "error":
		return queue.StatusFailed, true
	case string(queue.StatusQueued), string(queue.StatusRunning), string(queue.StatusSucceeded), string(queue.StatusFailed), string(queue.StatusTimedOut), string(queue.StatusDead):
		return queue.Status(strings.ToLower(strings.TrimSpace(raw))), true
	default:
		return "", false
	}
}

func parseTimeFlag(raw string) (time.Time, error) {
	layouts := []string{time.RFC3339Nano, time.RFC3339}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, raw); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid time")
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
