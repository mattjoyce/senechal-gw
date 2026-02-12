package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"

	"gopkg.in/yaml.v3"
)

var envVarPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// Load reads and parses configuration from a file.
// Supports both single-file mode (all config in one file) and multi-file mode (via include array).
func Load(configPath string) (*Config, error) {
	// Resolve to absolute path for consistent relative path resolution
	absPath, err := filepath.Abs(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve config path %q: %w", configPath, err)
	}

	// Check if path is directory (legacy multi-file discovery) or file
	info, err := os.Stat(absPath)
	if err != nil {
		return nil, fmt.Errorf("config file not found: %s\n"+
			"Hint: Check the path or run with --config flag", absPath)
	}

	if info.IsDir() {
		// Directory provided - look for config.yaml inside
		absPath = filepath.Join(absPath, "config.yaml")
		if _, err := os.Stat(absPath); err != nil {
			return nil, fmt.Errorf("directory provided but config.yaml not found: %s", absPath)
		}
	}

	// Load main config file
	cfg, err := loadConfigFile(absPath, make(map[string]bool))
	if err != nil {
		return nil, err
	}
	cfg.SourceFiles = make(map[string]*yaml.Node)

	// Add root node to SourceFiles (manually since loadConfigFile returns a partial Config)
	rootData, _ := os.ReadFile(absPath)
	var rootNode yaml.Node
	if err := yaml.Unmarshal(rootData, &rootNode); err == nil {
		cfg.SourceFiles[absPath] = &rootNode
	}

	// If include array exists, load and merge included files
	var includedPaths []string
	if len(cfg.Include) > 0 {
		configDir := filepath.Dir(absPath)
		visited := make(map[string]bool)
		if err := loadIncludes(cfg, cfg.Include, configDir, visited); err != nil {
			return nil, err
		}
		for path := range visited {
			includedPaths = append(includedPaths, path)
		}
	}

	// Apply config defaults before validation
	cfg = applyConfigDefaults(cfg)

	// Hash-verify all configuration files (root config + all includes)
	allPaths := append([]string{absPath}, includedPaths...)
	if err := verifyAllConfigHashes(allPaths); err != nil {
		return nil, err
	}

	// Validate configuration (including cross-file references if multi-file mode)
	if len(cfg.Include) > 0 {
		// Multi-file mode: extract tokens for cross-validation
		tokens := extractTokensFromConfig(cfg)
		validator := &ConfigValidator{
			config: cfg,
			tokens: tokens,
		}
		if err := validator.ValidateCrossReferences(); err != nil {
			return nil, fmt.Errorf("configuration validation failed: %w", err)
		}
	}

	// Standard validation
	if err := validate(cfg); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	// Apply plugin defaults
	for name, pluginConf := range cfg.Plugins {
		merged := mergePluginDefaults(pluginConf)
		cfg.Plugins[name] = merged
	}

	return cfg, nil
}

// DiscoverConfigDir finds the config directory by checking standard locations.
// Priority order: --config-dir flag, $SENECHAL_CONFIG_DIR, ~/.config/senechal-gw, /etc/senechal-gw, ./config.yaml (legacy)
func DiscoverConfigDir() (string, error) {
	// 1. Check environment variable
	if dir := os.Getenv("SENECHAL_CONFIG_DIR"); dir != "" {
		if _, err := os.Stat(dir); err == nil {
			return dir, nil
		}
	}

	// 2. Check user config directory
	if homeDir, err := os.UserHomeDir(); err == nil {
		userConfigDir := filepath.Join(homeDir, ".config", "senechal-gw")
		if _, err := os.Stat(userConfigDir); err == nil {
			return userConfigDir, nil
		}
	}

	// 3. Check system config directory
	systemConfigDir := "/etc/senechal-gw"
	if _, err := os.Stat(systemConfigDir); err == nil {
		return systemConfigDir, nil
	}

	// 4. Fallback to legacy single-file config in current directory
	legacyConfigPath := "./config.yaml"
	if _, err := os.Stat(legacyConfigPath); err == nil {
		return legacyConfigPath, nil
	}

	return "", fmt.Errorf("no config found (checked: $SENECHAL_CONFIG_DIR, ~/.config/senechal-gw, /etc/senechal-gw, ./config.yaml)")
}

// DiscoverScopeDirs is preserved for backward compatibility but delegates to DiscoverAllConfigFiles.
func DiscoverScopeDirs(configPath string) ([]string, error) {
	return DiscoverAllConfigFiles(configPath)
}

// DiscoverAllConfigFiles returns absolute paths to all configuration files in the include tree.
func DiscoverAllConfigFiles(configPath string) ([]string, error) {
	absPath, err := filepath.Abs(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve config path %q: %w", configPath, err)
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return nil, fmt.Errorf("config file not found: %s\nHint: Check the path or run with --config flag", absPath)
	}

	if info.IsDir() {
		absPath = filepath.Join(absPath, "config.yaml")
		if _, err := os.Stat(absPath); err != nil {
			return nil, fmt.Errorf("directory provided but config.yaml not found: %s", absPath)
		}
	}

	cfg, err := loadConfigFile(absPath, make(map[string]bool))
	if err != nil {
		return nil, err
	}

	visited := map[string]bool{absPath: true}
	if len(cfg.Include) > 0 {
		if err := collectIncludes(cfg.Include, filepath.Dir(absPath), visited); err != nil {
			return nil, err
		}
	}

	files := make([]string, 0, len(visited))
	for f := range visited {
		files = append(files, f)
	}
	sort.Strings(files)
	return files, nil
}

func collectIncludes(includes []string, baseDir string, visited map[string]bool) error {
	for i, includePath := range includes {
		includePath = interpolateEnv(includePath)
		var resolvedPath string
		if filepath.IsAbs(includePath) {
			resolvedPath = includePath
		} else {
			resolvedPath = filepath.Join(baseDir, includePath)
		}

		absPath, err := filepath.Abs(resolvedPath)
		if err != nil {
			return fmt.Errorf("include[%d]: failed to resolve path %q: %w", i, includePath, err)
		}

		if visited[absPath] {
			continue
		}

		if _, err := os.Stat(absPath); err != nil {
			return fmt.Errorf("include[%d]: file not found: %s\n"+
				"Referenced from: %s\n"+
				"Hint: Check the path is correct and the file exists", i, absPath, baseDir)
		}

		visited[absPath] = true

		data, err := os.ReadFile(absPath)
		if err != nil {
			return err
		}
		interpolated := interpolateEnv(string(data))
		var partial struct {
			Include []string `yaml:"include"`
		}
		if err := yaml.Unmarshal([]byte(interpolated), &partial); err != nil {
			return fmt.Errorf("failed to parse YAML for includes in %s: %w", absPath, err)
		}

		if len(partial.Include) > 0 {
			if err := collectIncludes(partial.Include, filepath.Dir(absPath), visited); err != nil {
				return err
			}
		}
	}
	return nil
}

// loadIncludes recursively loads and merges files from the include array.
// visited tracks loaded files to prevent cycles.
func loadIncludes(cfg *Config, includes []string, baseDir string, visited map[string]bool) error {
	for i, includePath := range includes {
		// Apply env var interpolation to path
		includePath = interpolateEnv(includePath)

		// Resolve relative paths relative to baseDir
		var resolvedPath string
		if filepath.IsAbs(includePath) {
			resolvedPath = includePath
		} else {
			resolvedPath = filepath.Join(baseDir, includePath)
		}

		// Convert to absolute path for cycle detection
		absPath, err := filepath.Abs(resolvedPath)
		if err != nil {
			return fmt.Errorf("include[%d]: failed to resolve path %q: %w", i, includePath, err)
		}

		// Check for cycles
		if visited[absPath] {
			return fmt.Errorf("include[%d]: circular dependency detected: %s", i, absPath)
		}

		// Check if file exists - HARD FAIL with good UX
		if _, err := os.Stat(absPath); err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("include[%d]: file not found: %s\n"+
					"Referenced from: %s\n"+
					"Hint: Check the path is correct and the file exists", i, absPath, baseDir)
			}
			return fmt.Errorf("include[%d]: failed to access file %s: %w", i, absPath, err)
		}

		visited[absPath] = true

		// Load included file
		includedData, _ := os.ReadFile(absPath)
		var includedNode yaml.Node
		if err := yaml.Unmarshal(includedData, &includedNode); err == nil {
			cfg.SourceFiles[absPath] = &includedNode
		}

		includedCfg, err := loadConfigFile(absPath, visited)
		if err != nil {
			return fmt.Errorf("include[%d] (%s): %w", i, includePath, err)
		}

		// Deep merge included config into main config
		if err := deepMergeConfig(cfg, includedCfg); err != nil {
			return fmt.Errorf("include[%d] (%s): merge failed: %w", i, includePath, err)
		}

		// If included file has its own includes, recursively load them
		if len(includedCfg.Include) > 0 {
			includedBaseDir := filepath.Dir(absPath)
			if err := loadIncludes(cfg, includedCfg.Include, includedBaseDir, visited); err != nil {
				return err
			}
		}
	}

	return nil
}

// loadConfigFile loads and parses a single config file.
// visited is used for cycle detection when loading includes (nil for top-level).
func loadConfigFile(path string, visited map[string]bool) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	// Apply environment variable interpolation
	interpolated := interpolateEnv(string(data))

	// Parse YAML into partial config (don't apply defaults yet)
	var cfg Config
	if err := yaml.Unmarshal([]byte(interpolated), &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	return &cfg, nil
}

// deepMergeConfig merges src into dst, with src taking precedence for non-zero values.
func deepMergeConfig(dst, src *Config) error {
	// Merge service config (non-zero values from src override dst)
	if src.Service.Name != "" {
		dst.Service.Name = src.Service.Name
	}
	if src.Service.TickInterval != 0 {
		dst.Service.TickInterval = src.Service.TickInterval
	}
	if src.Service.LogLevel != "" {
		dst.Service.LogLevel = src.Service.LogLevel
	}
	if src.Service.LogFormat != "" {
		dst.Service.LogFormat = src.Service.LogFormat
	}
	if src.Service.DedupeTTL != 0 {
		dst.Service.DedupeTTL = src.Service.DedupeTTL
	}
	if src.Service.JobLogRetention != 0 {
		dst.Service.JobLogRetention = src.Service.JobLogRetention
	}

	// Merge state config
	if src.State.Path != "" {
		dst.State.Path = src.State.Path
	}

	// Merge API config
	if src.API.Enabled {
		dst.API.Enabled = src.API.Enabled
	}
	if src.API.Listen != "" {
		dst.API.Listen = src.API.Listen
	}
	if src.API.Auth.APIKey != "" {
		dst.API.Auth.APIKey = src.API.Auth.APIKey
	}

	// Merge plugins_dir
	if src.PluginsDir != "" {
		dst.PluginsDir = src.PluginsDir
	}

	// Merge plugins (additive - src plugins added/override dst plugins)
	if src.Plugins != nil {
		if dst.Plugins == nil {
			dst.Plugins = make(map[string]PluginConf)
		}
		for name, plugin := range src.Plugins {
			dst.Plugins[name] = plugin
		}
	}

	// Merge routes (append)
	if len(src.Routes) > 0 {
		dst.Routes = append(dst.Routes, src.Routes...)
	}

	// Merge webhooks
	if src.Webhooks != nil {
		if dst.Webhooks == nil {
			dst.Webhooks = &WebhooksConfig{}
		}
		if src.Webhooks.Listen != "" {
			dst.Webhooks.Listen = src.Webhooks.Listen
		}
		if len(src.Webhooks.Endpoints) > 0 {
			dst.Webhooks.Endpoints = append(dst.Webhooks.Endpoints, src.Webhooks.Endpoints...)
		}
	}

	return nil
}

func verifyAllConfigHashes(paths []string) error {
	// Group paths by directory to avoid loading the same checksums file multiple times
	dirToFiles := make(map[string][]string)
	for _, path := range paths {
		dir := filepath.Dir(path)
		dirToFiles[dir] = append(dirToFiles[dir], path)
	}

	for dir, files := range dirToFiles {
		// Load checksums from this directory
		checksums, err := LoadChecksums(dir)
		if err != nil {
			// If .checksums is missing, we skip verification for this directory.
			continue
		}

		// Verify each file in this directory that has a hash in checksums
		for _, path := range files {
			basename := filepath.Base(path)
			expectedHash, ok := checksums.Hashes[basename]
			if !ok {
				return fmt.Errorf("config file %s has no hash in checksums at %s\n"+
					"Run: senechal-gw config lock --config-dir %s", basename, dir, dir)
			}

			if err := VerifyFileHash(path, expectedHash); err != nil {
				return fmt.Errorf("config verification failed for %s: %w\n"+
					"This indicates tampering or unauthorized modification.\n"+
					"If you edited this file intentionally, run: senechal-gw config lock --config-dir %s", path, err, dir)
			}
		}
	}

	return nil
}

// verifyScopeFilesRecursively is preserved for backward compatibility but delegates to verifyAllConfigHashes.
func verifyScopeFilesRecursively(paths []string) error {
	return verifyAllConfigHashes(paths)
}

// extractTokensFromConfig extracts token definitions from config for cross-validation.
// In include-based mode, tokens are defined inline in config (not separate file).
func extractTokensFromConfig(cfg *Config) map[string]string {
	// In the include approach, tokens would be merged into the config
	// For now, return empty map - tokens validation will be updated separately
	// when we determine how tokens are structured in this approach
	return make(map[string]string)
}

// applyConfigDefaults merges default values into config where not explicitly set.
func applyConfigDefaults(cfg *Config) *Config {
	defaults := Defaults()

	// Apply service defaults if not set
	if cfg.Service.Name == "" {
		cfg.Service.Name = defaults.Service.Name
	}
	if cfg.Service.TickInterval == 0 {
		cfg.Service.TickInterval = defaults.Service.TickInterval
	}
	if cfg.Service.LogLevel == "" {
		cfg.Service.LogLevel = defaults.Service.LogLevel
	}
	if cfg.Service.LogFormat == "" {
		cfg.Service.LogFormat = defaults.Service.LogFormat
	}
	if cfg.Service.DedupeTTL == 0 {
		cfg.Service.DedupeTTL = defaults.Service.DedupeTTL
	}
	if cfg.Service.JobLogRetention == 0 {
		cfg.Service.JobLogRetention = defaults.Service.JobLogRetention
	}

	// Apply state defaults if not set
	if cfg.State.Path == "" {
		cfg.State.Path = defaults.State.Path
	}

	// Apply API defaults if not set
	if !cfg.API.Enabled && cfg.API.Listen == "" {
		cfg.API = defaults.API
	}

	// Apply plugins_dir default if not set
	if cfg.PluginsDir == "" {
		cfg.PluginsDir = defaults.PluginsDir
	}

	return cfg
}

// interpolateEnv replaces ${VAR} with environment variable values.
// Undefined variables are left as-is (not expanded).
func interpolateEnv(input string) string {
	return envVarPattern.ReplaceAllStringFunc(input, func(match string) string {
		// Extract variable name from ${VAR}
		varName := envVarPattern.FindStringSubmatch(match)[1]

		// Look up environment variable
		if value, exists := os.LookupEnv(varName); exists {
			return value
		}

		// If not found, leave the placeholder (will fail validation if required)
		return match
	})
}

// validate performs basic validation on the configuration.
func validate(cfg *Config) error {
	// Service validation
	if cfg.Service.TickInterval <= 0 {
		return fmt.Errorf("service.tick_interval must be positive")
	}

	validLogLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
	if !validLogLevels[cfg.Service.LogLevel] {
		return fmt.Errorf("service.log_level must be one of: debug, info, warn, error (got %q)", cfg.Service.LogLevel)
	}

	// State validation
	if cfg.State.Path == "" {
		return fmt.Errorf("state.path is required")
	}

	// Plugins dir validation
	if cfg.PluginsDir == "" {
		return fmt.Errorf("plugins_dir is required")
	}

	// API auth validation
	if cfg.API.Enabled {
		// If tokens are configured, validate them. api_key remains supported for back-compat.
		if envVarPattern.MatchString(cfg.API.Auth.APIKey) {
			matches := envVarPattern.FindStringSubmatch(cfg.API.Auth.APIKey)
			if len(matches) > 1 {
				return fmt.Errorf("api.auth.api_key: environment variable ${%s} is not set", matches[1])
			}
			return fmt.Errorf("api.auth.api_key: unresolved environment variable")
		}
		for i, tok := range cfg.API.Auth.Tokens {
			if tok.Token == "" {
				return fmt.Errorf("api.auth.tokens[%d].token is required", i)
			}
			if envVarPattern.MatchString(tok.Token) {
				matches := envVarPattern.FindStringSubmatch(tok.Token)
				if len(matches) > 1 {
					return fmt.Errorf("api.auth.tokens[%d].token: environment variable ${%s} is not set", i, matches[1])
				}
				return fmt.Errorf("api.auth.tokens[%d].token: unresolved environment variable", i)
			}
			if len(tok.Scopes) == 0 {
				return fmt.Errorf("api.auth.tokens[%d].scopes must be non-empty", i)
			}
		}
	}

	// Plugin validation
	for name, plugin := range cfg.Plugins {
		if !plugin.Enabled {
			continue // Skip disabled plugins
		}

		// Schedule required for enabled plugins
		if plugin.Schedule == nil {
			return fmt.Errorf("plugin %q: schedule is required for enabled plugins", name)
		}

		if plugin.Schedule.Every == "" {
			return fmt.Errorf("plugin %q: schedule.every is required", name)
		}

		// Validate schedule.every values (MVP subset)
		validIntervals := []string{"5m", "15m", "30m", "hourly", "2h", "6h", "daily", "weekly", "monthly"}
		valid := false
		for _, interval := range validIntervals {
			if plugin.Schedule.Every == interval {
				valid = true
				break
			}
		}
		if !valid {
			return fmt.Errorf("plugin %q: schedule.every must be one of %v (got %q)",
				name, validIntervals, plugin.Schedule.Every)
		}

		// Check for unresolved env vars in config (security: no secrets leaked in logs)
		if plugin.Config != nil {
			if err := checkUnresolvedEnvVars(plugin.Config, name); err != nil {
				return err
			}
		}
	}

	return nil
}

// checkUnresolvedEnvVars recursively checks for ${VAR} placeholders in config values.
func checkUnresolvedEnvVars(data map[string]interface{}, pluginName string) error {
	for key, value := range data {
		switch v := value.(type) {
		case string:
			if envVarPattern.MatchString(v) {
				// Extract variable name for better error message
				matches := envVarPattern.FindStringSubmatch(v)
				if len(matches) > 1 {
					return fmt.Errorf("plugin %q: environment variable ${%s} is not set", pluginName, matches[1])
				}
				return fmt.Errorf("plugin %q: unresolved environment variable in config.%s", pluginName, key)
			}
		case map[string]interface{}:
			if err := checkUnresolvedEnvVars(v, pluginName); err != nil {
				return err
			}
		}
	}
	return nil
}

// mergePluginDefaults applies default values to plugin config where not specified.
func mergePluginDefaults(plugin PluginConf) PluginConf {
	defaults := DefaultPluginConf()

	if plugin.Retry == nil {
		plugin.Retry = defaults.Retry
	}

	if plugin.Timeouts == nil {
		plugin.Timeouts = defaults.Timeouts
	}

	if plugin.CircuitBreaker == nil {
		plugin.CircuitBreaker = defaults.CircuitBreaker
	}

	if plugin.MaxOutstandingPolls == 0 {
		plugin.MaxOutstandingPolls = defaults.MaxOutstandingPolls
	}

	return plugin
}

// ParseInterval converts schedule interval strings to durations.
// Returns 0 for special cases like "daily", "weekly", "monthly" (handled by scheduler).
func ParseInterval(interval string) (time.Duration, error) {
	// Direct duration strings
	switch interval {
	case "hourly":
		return 1 * time.Hour, nil
	case "daily", "weekly", "monthly":
		return 0, nil // Special handling in scheduler
	}

	// Try parsing as duration (e.g., "5m", "2h")
	d, err := time.ParseDuration(interval)
	if err != nil {
		return 0, fmt.Errorf("invalid schedule interval %q: %w", interval, err)
	}

	if d <= 0 {
		return 0, fmt.Errorf("schedule interval must be positive: %q", interval)
	}

	return d, nil
}
