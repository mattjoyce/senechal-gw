package config

import (
	"fmt"
	"path/filepath"
	"sort"
	"time"
)

// ResolvedPlugin describes a configured plugin after the caller has resolved it
// through the live plugin registry. The config package cannot import the plugin
// package (plugin → config would be circular), so the caller — the ductile CLI
// or boot path — builds []ResolvedPlugin from the registry and passes it in.
//
// ManifestPath and EntrypointPath MUST be absolute and symlink-resolved; this
// matches the plugin loader's trust policy and ensures lock-time and verify-time
// paths compare cleanly.
type ResolvedPlugin struct {
	Name           string
	Enabled        bool
	Uses           string // empty for non-aliases; alias base name otherwise
	ManifestPath   string
	EntrypointPath string
}

// ComputePluginFingerprint reads the resolved plugin's manifest and entrypoint
// files, hashes each with BLAKE3, and returns a PluginFingerprint. Missing or
// unreadable files produce a hard error — the operator cannot lock a plugin
// whose bytes cannot be read.
func ComputePluginFingerprint(rp ResolvedPlugin) (PluginFingerprint, error) {
	if rp.Name == "" {
		return PluginFingerprint{}, fmt.Errorf("plugin name is required")
	}
	if !filepath.IsAbs(rp.ManifestPath) {
		return PluginFingerprint{}, fmt.Errorf("plugin %q: manifest path must be absolute, got %q", rp.Name, rp.ManifestPath)
	}
	if !filepath.IsAbs(rp.EntrypointPath) {
		return PluginFingerprint{}, fmt.Errorf("plugin %q: entrypoint path must be absolute, got %q", rp.Name, rp.EntrypointPath)
	}

	manifestHash, err := ComputeBlake3Hash(rp.ManifestPath)
	if err != nil {
		return PluginFingerprint{}, fmt.Errorf("plugin %q: hash manifest %s: %w", rp.Name, rp.ManifestPath, err)
	}
	entrypointHash, err := ComputeBlake3Hash(rp.EntrypointPath)
	if err != nil {
		return PluginFingerprint{}, fmt.Errorf("plugin %q: hash entrypoint %s: %w", rp.Name, rp.EntrypointPath, err)
	}

	return PluginFingerprint{
		Name:           rp.Name,
		Enabled:        rp.Enabled,
		Uses:           rp.Uses,
		ManifestPath:   rp.ManifestPath,
		ManifestHash:   manifestHash,
		EntrypointPath: rp.EntrypointPath,
		EntrypointHash: entrypointHash,
	}, nil
}

// GenerateChecksumsWithPlugins computes BLAKE3 hashes for every discovered
// config file AND every resolved plugin, then writes a v2 manifest embedding
// plugin_fingerprints sorted by Name. The existing GenerateChecksumsFromDiscovery
// function is intentionally left untouched — callers that do not want plugin
// fingerprinting keep their existing behavior.
//
// When dryRun is true, no file is written but all hashing still runs so the
// caller can surface hash-time errors before commit.
func GenerateChecksumsWithPlugins(files *ConfigFiles, plugins []ResolvedPlugin, dryRun bool) error {
	manifest := ChecksumManifest{
		Version:     2,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Hashes:      make(map[string]string),
	}

	for _, path := range files.AllFiles() {
		hash, err := ComputeBlake3Hash(path)
		if err != nil {
			return fmt.Errorf("failed to hash %s: %w", path, err)
		}
		manifest.Hashes[path] = hash
	}

	fingerprints := make([]PluginFingerprint, 0, len(plugins))
	for _, rp := range plugins {
		fp, err := ComputePluginFingerprint(rp)
		if err != nil {
			return err
		}
		fingerprints = append(fingerprints, fp)
	}
	sort.Slice(fingerprints, func(i, j int) bool {
		return fingerprints[i].Name < fingerprints[j].Name
	})
	if len(fingerprints) > 0 {
		manifest.PluginFingerprints = fingerprints
	}

	if dryRun {
		return nil
	}

	checksumPath := filepath.Join(files.Root, ".checksums")
	return writeChecksumsAtomic(checksumPath, manifest)
}

// VerifyPluginFingerprints compares recorded PluginFingerprint entries against
// a live registry snapshot keyed by plugin Name. Returns an *IntegrityResult
// the caller merges with VerifyIntegrity's result.
//
// Classification (matches the red-teamed invariant — identity is bytes, not path):
//
//   - Enabled plugin, manifest or entrypoint hash mismatch → hard error.
//   - Disabled plugin, any mismatch → warning (operator may still be rebuilding unrelated bytes).
//   - Recorded plugin no longer present in currentPlugins → warning (stale, run lock).
//   - Bytes match but path changed → informational warning (not a security event).
//   - File read error: error if enabled, warning if disabled.
//
// Empty fingerprints slice returns a passing result with no findings so that
// deployments that have not opted into plugin fingerprinting see unchanged
// behavior.
//
// Every diagnostic message names the plugin, the path, short hash prefixes,
// and the literal recovery command so operators can always recover.
func VerifyPluginFingerprints(fingerprints []PluginFingerprint, currentPlugins map[string]ResolvedPlugin) *IntegrityResult {
	result := &IntegrityResult{Passed: true}
	if len(fingerprints) == 0 {
		return result
	}

	addFinding := func(msg string, enabled bool) {
		if enabled {
			result.Passed = false
			result.Errors = append(result.Errors, msg)
		} else {
			result.Warnings = append(result.Warnings, msg)
		}
	}

	shortHash := func(h string) string {
		if len(h) < 12 {
			return h
		}
		return h[:12]
	}

	for _, fp := range fingerprints {
		current, stillConfigured := currentPlugins[fp.Name]
		if !stillConfigured {
			result.Warnings = append(result.Warnings, fmt.Sprintf(
				"plugin %q recorded in .checksums but no longer configured; run 'ductile config lock --plugins' to clear stale entry",
				fp.Name))
			continue
		}

		currentManifestHash, err := ComputeBlake3Hash(current.ManifestPath)
		if err != nil {
			addFinding(fmt.Sprintf(
				"plugin %q: failed to read manifest at %s: %v; run 'ductile config lock --plugins' after investigating",
				fp.Name, current.ManifestPath, err), fp.Enabled)
			continue
		}
		currentEntryHash, err := ComputeBlake3Hash(current.EntrypointPath)
		if err != nil {
			addFinding(fmt.Sprintf(
				"plugin %q: failed to read entrypoint at %s: %v; run 'ductile config lock --plugins' after investigating",
				fp.Name, current.EntrypointPath, err), fp.Enabled)
			continue
		}

		if currentManifestHash != fp.ManifestHash {
			addFinding(fmt.Sprintf(
				"plugin %q: manifest hash mismatch at %s (expected %s, got %s); run 'ductile config lock --plugins' after reviewing the change",
				fp.Name, current.ManifestPath, shortHash(fp.ManifestHash), shortHash(currentManifestHash)),
				fp.Enabled)
		} else if current.ManifestPath != fp.ManifestPath {
			result.Warnings = append(result.Warnings, fmt.Sprintf(
				"plugin %q: manifest path changed but bytes match (was %s, now %s); run 'ductile config lock --plugins' to refresh the record",
				fp.Name, fp.ManifestPath, current.ManifestPath))
		}

		if currentEntryHash != fp.EntrypointHash {
			addFinding(fmt.Sprintf(
				"plugin %q: entrypoint hash mismatch at %s (expected %s, got %s); run 'ductile config lock --plugins' after reviewing the change",
				fp.Name, current.EntrypointPath, shortHash(fp.EntrypointHash), shortHash(currentEntryHash)),
				fp.Enabled)
		} else if current.EntrypointPath != fp.EntrypointPath {
			result.Warnings = append(result.Warnings, fmt.Sprintf(
				"plugin %q: entrypoint path changed but bytes match (was %s, now %s); run 'ductile config lock --plugins' to refresh the record",
				fp.Name, fp.EntrypointPath, current.EntrypointPath))
		}
	}

	return result
}
