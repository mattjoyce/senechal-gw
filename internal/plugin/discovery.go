package plugin

import (
	"fmt"
	"io/fs"
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

// Discover scans a single pluginsDir for plugins with manifest.yaml and validates them.
// Returns a registry of valid plugins. Invalid plugins are logged but not fatal.
func Discover(pluginsDir string, logger func(level, msg string, args ...any)) (*Registry, error) {
	return DiscoverMany([]string{pluginsDir}, logger)
}

// DiscoverMany scans multiple plugin roots for manifest.yaml files and validates plugins.
// Roots are processed in input order; duplicate plugin names keep the first discovered plugin.
func DiscoverMany(pluginRoots []string, logger func(level, msg string, args ...any)) (*Registry, error) {
	if logger == nil {
		logger = func(level, msg string, args ...any) {}
	}
	if len(pluginRoots) == 0 {
		return nil, fmt.Errorf("at least one plugin root is required")
	}

	absRoots := make([]string, 0, len(pluginRoots))
	seenRoots := make(map[string]struct{}, len(pluginRoots))
	for _, root := range pluginRoots {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		absRoot, err := filepath.Abs(root)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve plugin root %q: %w", root, err)
		}
		info, err := os.Stat(absRoot)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, fmt.Errorf("plugin root does not exist: %s", absRoot)
			}
			return nil, fmt.Errorf("failed to stat plugin root %s: %w", absRoot, err)
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("plugin root is not a directory: %s", absRoot)
		}
		if _, ok := seenRoots[absRoot]; ok {
			continue
		}
		seenRoots[absRoot] = struct{}{}
		absRoots = append(absRoots, absRoot)
	}
	if len(absRoots) == 0 {
		return nil, fmt.Errorf("at least one plugin root is required")
	}

	registry := NewRegistry()
	for _, root := range absRoots {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.IsDir() || d.Name() != manifestFilename {
				return nil
			}

			pluginPath := filepath.Dir(path)
			pluginDirName := filepath.Base(pluginPath)

			plugin, err := loadPlugin(pluginDirName, pluginPath, root)
			if err != nil {
				logger("warn", "failed to load plugin", "root", root, "path", pluginPath, "error", err.Error())
				return nil
			}

			if err := registry.Add(plugin); err != nil {
				if existing, ok := registry.Get(plugin.Name); ok {
					logger(
						"warn",
						"duplicate plugin ignored (keeping first discovered)",
						"plugin", plugin.Name,
						"ignored_path", plugin.Path,
						"kept_path", existing.Path,
					)
				} else {
					logger("warn", "duplicate plugin", "plugin", plugin.Name, "error", err.Error())
				}
				return nil
			}

			logger("info", "loaded plugin", "plugin", plugin.Name, "path", plugin.Path, "version", plugin.Version, "protocol", plugin.Protocol)
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("failed to scan plugin root %s: %w", root, err)
		}
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
		ManifestSpec:    manifest.ManifestSpec,
		ManifestVersion: manifest.ManifestVersion,
		Name:            manifest.Name,
		Path:            pluginPath,
		Entrypoint:      entrypointPath,
		Protocol:        manifest.Protocol,
		Version:         manifest.Version,
		Description:     manifest.Description,
		Commands:        manifest.Commands,
		ConfigKeys:      manifest.ConfigKeys,
	}, nil
}

// validateManifest checks required manifest fields.
func validateManifest(m *Manifest) error {
	if strings.TrimSpace(m.ManifestSpec) == "" {
		return fmt.Errorf("manifest_spec is required")
	}
	if m.ManifestSpec != SupportedManifestSpec {
		return fmt.Errorf("unsupported manifest_spec %q (supported: %q)", m.ManifestSpec, SupportedManifestSpec)
	}
	if m.ManifestVersion == 0 {
		return fmt.Errorf("manifest_version is required")
	}
	if m.ManifestVersion != SupportedManifestVersion {
		return fmt.Errorf("unsupported manifest_version %d (supported: %d)", m.ManifestVersion, SupportedManifestVersion)
	}

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
	return validateTrustInRoots(entrypointPath, pluginPath, []string{pluginsDir})
}

func validateTrustInRoots(entrypointPath, pluginPath string, pluginRoots []string) error {
	if len(pluginRoots) == 0 {
		return fmt.Errorf("no plugin roots configured")
	}

	// Resolve symlinks
	resolvedEntrypoint, err := filepath.EvalSymlinks(entrypointPath)
	if err != nil {
		return fmt.Errorf("failed to resolve entrypoint symlink: %w", err)
	}

	resolvedPluginPath, err := filepath.EvalSymlinks(pluginPath)
	if err != nil {
		return fmt.Errorf("failed to resolve plugin path symlink: %w", err)
	}

	// Check entrypoint is under one of the configured plugin roots
	inApprovedRoot := false
	for _, root := range pluginRoots {
		resolvedRoot, err := filepath.EvalSymlinks(root)
		if err != nil {
			return fmt.Errorf("failed to resolve plugin root symlink %s: %w", root, err)
		}
		if strings.HasPrefix(resolvedEntrypoint, resolvedRoot+string(os.PathSeparator)) {
			inApprovedRoot = true
			break
		}
	}
	if !inApprovedRoot {
		return fmt.Errorf("entrypoint %s is not under any configured plugin root", resolvedEntrypoint)
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
