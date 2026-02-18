package plugin

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	supportedProtocol = 2
	manifestFilename  = "manifest.yaml"
)

// Registry holds discovered plugins indexed by name.
type Registry struct {
	plugins map[string]*Plugin
}

// NewRegistry creates an empty plugin registry.
func NewRegistry() *Registry {
	return &Registry{
		plugins: make(map[string]*Plugin),
	}
}

// Get retrieves a plugin by name.
func (r *Registry) Get(name string) (*Plugin, bool) {
	p, ok := r.plugins[name]
	return p, ok
}

// All returns all registered plugins.
func (r *Registry) All() map[string]*Plugin {
	return r.plugins
}

// Add registers a plugin in the registry.
func (r *Registry) Add(plugin *Plugin) error {
	if _, exists := r.plugins[plugin.Name]; exists {
		return fmt.Errorf("plugin %q already registered", plugin.Name)
	}
	r.plugins[plugin.Name] = plugin
	return nil
}

// Discover scans pluginsDir for plugins with manifest.yaml and validates them.
// Returns a registry of valid plugins. Invalid plugins are logged but not fatal.
func Discover(pluginsDir string, logger func(level, msg string, args ...any)) (*Registry, error) {
	if logger == nil {
		logger = func(level, msg string, args ...any) {}
	}
	absPluginsDir, err := filepath.Abs(pluginsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve plugins directory: %w", err)
	}

	// Check plugins directory exists
	if _, err := os.Stat(absPluginsDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("plugins directory does not exist: %s", absPluginsDir)
	}

	registry := NewRegistry()

	entries, err := os.ReadDir(absPluginsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read plugins directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue // Skip files, only process directories
		}

		pluginName := entry.Name()
		pluginPath := filepath.Join(absPluginsDir, pluginName)

		// Check for manifest.yaml
		manifestPath := filepath.Join(pluginPath, manifestFilename)
		if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
			continue // No manifest, skip this directory
		}

		// Load and validate plugin
		plugin, err := loadPlugin(pluginName, pluginPath, absPluginsDir)
		if err != nil {
			logger("warn", "failed to load plugin", "plugin", pluginName, "error", err.Error())
			continue
		}

		if err := registry.Add(plugin); err != nil {
			logger("warn", "duplicate plugin", "plugin", pluginName, "error", err.Error())
			continue
		}

		logger("info", "loaded plugin", "plugin", pluginName, "version", plugin.Version, "protocol", plugin.Protocol)
	}

	return registry, nil
}

// loadPlugin reads and validates a single plugin.
func loadPlugin(name, pluginPath, pluginsDir string) (*Plugin, error) {
	manifestPath := filepath.Join(pluginPath, manifestFilename)

	// Read manifest file
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest: %w", err)
	}

	// Parse manifest
	var manifest Manifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("failed to parse manifest YAML: %w", err)
	}

	// Validate manifest fields
	if err := validateManifest(&manifest); err != nil {
		return nil, fmt.Errorf("invalid manifest: %w", err)
	}

	// Check protocol version
	if manifest.Protocol != supportedProtocol {
		return nil, fmt.Errorf("unsupported protocol version %d (supported: %d)", manifest.Protocol, supportedProtocol)
	}

	// Construct entrypoint path
	entrypointPath := filepath.Join(pluginPath, manifest.Entrypoint)

	// Trust checks (SPEC §5.5)
	if err := validateTrust(entrypointPath, pluginPath, pluginsDir); err != nil {
		return nil, fmt.Errorf("trust validation failed: %w", err)
	}

	return &Plugin{
		Name:        manifest.Name,
		Path:        pluginPath,
		Entrypoint:  entrypointPath,
		Protocol:    manifest.Protocol,
		Version:     manifest.Version,
		Description: manifest.Description,
		Commands:    manifest.Commands,
		ConfigKeys:  manifest.ConfigKeys,
	}, nil
}

// validateManifest checks required manifest fields.
func validateManifest(m *Manifest) error {
	if m.Name == "" {
		return fmt.Errorf("name is required")
	}

	if m.Protocol == 0 {
		return fmt.Errorf("protocol version is required")
	}

	if m.Entrypoint == "" {
		return fmt.Errorf("entrypoint is required")
	}

	// Check for path traversal in entrypoint
	if strings.Contains(m.Entrypoint, "..") {
		return fmt.Errorf("entrypoint contains path traversal: %s", m.Entrypoint)
	}

	if len(m.Commands) == 0 {
		return fmt.Errorf("at least one command must be declared")
	}

	// Validate command names
	validCommands := map[string]bool{"poll": true, "handle": true, "health": true, "init": true}
	for _, cmd := range m.Commands {
		if cmd.Name == "" {
			return fmt.Errorf("command name is required")
		}
		if !validCommands[cmd.Name] {
			return fmt.Errorf("invalid command %q (valid: poll, handle, health, init)", cmd.Name)
		}
		if !cmd.Type.valid() {
			return fmt.Errorf("invalid command type %q for %q (valid: read, write)", cmd.Type, cmd.Name)
		}
	}

	return nil
}

// validateTrust enforces security constraints (SPEC §5.5).
func validateTrust(entrypointPath, pluginPath, pluginsDir string) error {
	// Resolve symlinks
	resolvedEntrypoint, err := filepath.EvalSymlinks(entrypointPath)
	if err != nil {
		return fmt.Errorf("failed to resolve entrypoint symlink: %w", err)
	}

	resolvedPluginPath, err := filepath.EvalSymlinks(pluginPath)
	if err != nil {
		return fmt.Errorf("failed to resolve plugin path symlink: %w", err)
	}

	resolvedPluginsDir, err := filepath.EvalSymlinks(pluginsDir)
	if err != nil {
		return fmt.Errorf("failed to resolve plugins directory symlink: %w", err)
	}

	// Check entrypoint is under plugins_dir
	if !strings.HasPrefix(resolvedEntrypoint, resolvedPluginsDir+string(os.PathSeparator)) {
		return fmt.Errorf("entrypoint %s is not under plugins directory %s", resolvedEntrypoint, resolvedPluginsDir)
	}

	// Check entrypoint is under plugin directory
	if !strings.HasPrefix(resolvedEntrypoint, resolvedPluginPath+string(os.PathSeparator)) {
		return fmt.Errorf("entrypoint %s is not under plugin directory %s", resolvedEntrypoint, resolvedPluginPath)
	}

	// Check entrypoint is executable
	info, err := os.Stat(resolvedEntrypoint)
	if err != nil {
		return fmt.Errorf("entrypoint not found: %w", err)
	}

	mode := info.Mode()
	if mode&0111 == 0 {
		return fmt.Errorf("entrypoint is not executable: %s", resolvedEntrypoint)
	}

	// Check plugin directory is not world-writable
	pluginInfo, err := os.Stat(resolvedPluginPath)
	if err != nil {
		return fmt.Errorf("plugin directory not found: %w", err)
	}

	if pluginInfo.Mode().Perm()&0002 != 0 {
		return fmt.Errorf("plugin directory is world-writable: %s", resolvedPluginPath)
	}

	return nil
}
