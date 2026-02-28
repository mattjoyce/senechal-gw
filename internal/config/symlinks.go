package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// SymlinkWarning captures a detected symlink in a config-related path.
type SymlinkWarning struct {
	Path     string
	Resolved string
}

// CollectConfigPaths gathers config-related paths for symlink checks.
func CollectConfigPaths(configPath string, cfg *Config) ([]string, error) {
	absPath, err := filepath.Abs(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve config path %q: %w", configPath, err)
	}
	info, err := os.Stat(absPath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat config path %q: %w", absPath, err)
	}

	paths := []string{absPath}

	if info.IsDir() {
		files, err := DiscoverConfigFiles(absPath)
		if err != nil {
			return nil, err
		}
		paths = append(paths, files.AllFiles()...)
	} else {
		if cfg != nil {
			for f := range cfg.SourceFiles {
				paths = append(paths, f)
			}
		}
		paths = append(paths, filepath.Dir(absPath))
	}

	seen := make(map[string]struct{}, len(paths))
	unique := make([]string, 0, len(paths))
	for _, path := range paths {
		if path == "" {
			continue
		}
		abs, err := filepath.Abs(path)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve path %q: %w", path, err)
		}
		if _, ok := seen[abs]; ok {
			continue
		}
		seen[abs] = struct{}{}
		unique = append(unique, abs)
	}
	return unique, nil
}

// DetectSymlinks returns warnings for any paths that resolve through a symlink.
func DetectSymlinks(paths []string) ([]SymlinkWarning, error) {
	warnings := make([]SymlinkWarning, 0)
	for _, path := range paths {
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve symlinks for %q: %w", path, err)
		}
		if filepath.Clean(resolved) != filepath.Clean(path) {
			warnings = append(warnings, SymlinkWarning{Path: path, Resolved: resolved})
		}
	}
	return warnings, nil
}
