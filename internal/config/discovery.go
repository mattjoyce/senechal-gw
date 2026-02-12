package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// DiscoverConfigFiles walks a config directory and returns the manifest of discovered files.
// Returns error if config.yaml is missing (hard requirement).
func DiscoverConfigFiles(configDir string) (*ConfigFiles, error) {
	absDir, err := filepath.Abs(configDir)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve config dir %q: %w", configDir, err)
	}

	cf := &ConfigFiles{Root: absDir}

	// config.yaml is mandatory
	configPath := filepath.Join(absDir, "config.yaml")
	if _, err := os.Stat(configPath); err != nil {
		return nil, fmt.Errorf("config.yaml not found in %s: %w", absDir, err)
	}
	cf.Config = configPath

	// Optional named files
	if path := filepath.Join(absDir, "webhooks.yaml"); fileExists(path) {
		cf.Webhooks = path
	}
	if path := filepath.Join(absDir, "tokens.yaml"); fileExists(path) {
		cf.Tokens = path
	}
	if path := filepath.Join(absDir, "routes.yaml"); fileExists(path) {
		cf.Routes = path
	}

	// Walk plugins/*.yaml
	cf.Plugins, err = walkYAMLDir(filepath.Join(absDir, "plugins"))
	if err != nil {
		return nil, fmt.Errorf("failed to walk plugins/: %w", err)
	}

	// Walk pipelines/*.yaml
	cf.Pipelines, err = walkYAMLDir(filepath.Join(absDir, "pipelines"))
	if err != nil {
		return nil, fmt.Errorf("failed to walk pipelines/: %w", err)
	}

	// Walk scopes/*.json
	cf.Scopes, err = walkJSONDir(filepath.Join(absDir, "scopes"))
	if err != nil {
		return nil, fmt.Errorf("failed to walk scopes/: %w", err)
	}

	return cf, nil
}

// IsConfigSpecDir returns true if the directory looks like a CONFIG_SPEC directory.
// A CONFIG_SPEC directory must have config.yaml plus at least one subdirectory indicator
// (plugins/ or pipelines/). Named files alone (tokens.yaml, webhooks.yaml) are not
// sufficient, as they could also appear in include-based configurations.
func IsConfigSpecDir(dir string) bool {
	configPath := filepath.Join(dir, "config.yaml")
	if !fileExists(configPath) {
		return false
	}

	// Only subdirectories are unambiguous indicators of directory mode
	if dirExists(filepath.Join(dir, "plugins")) {
		return true
	}
	if dirExists(filepath.Join(dir, "pipelines")) {
		return true
	}

	return false
}

// walkYAMLDir returns sorted absolute paths of *.yaml files in dir.
// Returns nil (not error) if the directory doesn't exist.
func walkYAMLDir(dir string) ([]string, error) {
	return walkDirWithExt(dir, ".yaml")
}

// walkJSONDir returns sorted absolute paths of *.json files in dir.
// Returns nil (not error) if the directory doesn't exist.
func walkJSONDir(dir string) ([]string, error) {
	return walkDirWithExt(dir, ".json")
}

func walkDirWithExt(dir, ext string) ([]string, error) {
	if !dirExists(dir) {
		return nil, nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var files []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.HasSuffix(entry.Name(), ext) {
			files = append(files, filepath.Join(dir, entry.Name()))
		}
	}
	sort.Strings(files)
	return files, nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
