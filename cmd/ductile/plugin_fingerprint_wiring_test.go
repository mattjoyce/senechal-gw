package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mattjoyce/ductile/internal/config"
)

// buildFingerprintFixture writes a minimal config directory with service.allow_symlinks=true
// (so macOS /var/folders/ → /private/var/folders/ does not trip the symlink refusal)
// and one configured plugin named "gmail" with its manifest + entrypoint.
// Returns the absolute configDir path.
func buildFingerprintFixture(t *testing.T, pluginEnabled bool) string {
	t.Helper()
	tmp := t.TempDir()

	configYAML := `
plugin_roots:
  - plugins
service:
  allow_symlinks: true
plugins:
  gmail:
    enabled: ` + boolStr(pluginEnabled) + `
`
	if err := os.WriteFile(filepath.Join(tmp, "config.yaml"), []byte(configYAML), 0644); err != nil {
		t.Fatalf("write config.yaml: %v", err)
	}

	pluginDir := filepath.Join(tmp, "plugins", "gmail")
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		t.Fatalf("mkdir plugin: %v", err)
	}
	manifest := `manifest_spec: ductile.plugin
manifest_version: 1
name: gmail
version: 0.1.0
protocol: 2
entrypoint: gmail
commands:
  - name: poll
    type: write
`
	if err := os.WriteFile(filepath.Join(pluginDir, "manifest.yaml"), []byte(manifest), 0644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "gmail"), []byte("#!/bin/sh\necho gmail\n"), 0755); err != nil {
		t.Fatalf("write entrypoint: %v", err)
	}
	return tmp
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func TestResolveConfiguredPluginFingerprintsHappyPath(t *testing.T) {
	tmp := buildFingerprintFixture(t, true)
	configPath := filepath.Join(tmp, "config.yaml")

	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}

	resolved, err := resolveConfiguredPluginFingerprints(cfg, configPath)
	if err != nil {
		t.Fatalf("resolveConfiguredPluginFingerprints: %v", err)
	}
	if len(resolved) != 1 {
		t.Fatalf("want 1 resolved plugin, got %d", len(resolved))
	}
	if resolved[0].Name != "gmail" || !resolved[0].Enabled {
		t.Fatalf("wrong resolved entry: %+v", resolved[0])
	}
	if !filepath.IsAbs(resolved[0].ManifestPath) || !filepath.IsAbs(resolved[0].EntrypointPath) {
		t.Fatalf("resolved paths must be absolute: %+v", resolved[0])
	}
}

func TestResolveConfiguredPluginFingerprintsMissingPluginErrors(t *testing.T) {
	tmp := buildFingerprintFixture(t, true)
	configPath := filepath.Join(tmp, "config.yaml")

	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	// Configure a plugin that doesn't exist on disk.
	cfg.Plugins["ghost"] = config.PluginConf{Enabled: true}

	_, err = resolveConfiguredPluginFingerprints(cfg, configPath)
	if err == nil {
		t.Fatal("expected error for configured-but-missing plugin")
	}
	if !strings.Contains(err.Error(), "ghost") {
		t.Fatalf("error should name the missing plugin: %v", err)
	}
}

func TestResolveConfiguredPluginFingerprintsDisabledStillIncluded(t *testing.T) {
	tmp := buildFingerprintFixture(t, false)
	configPath := filepath.Join(tmp, "config.yaml")

	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	resolved, err := resolveConfiguredPluginFingerprints(cfg, configPath)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if len(resolved) != 1 || resolved[0].Enabled {
		t.Fatalf("disabled plugin should still appear with Enabled=false: %+v", resolved)
	}
}

func TestRunConfigHashUpdatePluginsFlagWritesFingerprints(t *testing.T) {
	tmp := buildFingerprintFixture(t, true)

	code, stdout, stderr := captureRunConfigHashUpdate(t, []string{"--config-dir", tmp, "--plugins", "-v"})
	if code != 0 {
		t.Fatalf("runConfigHashUpdate code=%d stderr=%s", code, stderr)
	}
	if !strings.Contains(stdout, "DISCOVER [plugin] gmail") {
		t.Fatalf("verbose output missing plugin discovery line: %s", stdout)
	}

	m, err := config.LoadChecksums(tmp)
	if err != nil {
		t.Fatalf("LoadChecksums: %v", err)
	}
	if len(m.PluginFingerprints) != 1 {
		t.Fatalf("want 1 fingerprint in .checksums, got %d", len(m.PluginFingerprints))
	}
	if m.PluginFingerprints[0].Name != "gmail" {
		t.Fatalf("wrong fingerprint: %+v", m.PluginFingerprints[0])
	}
}

func TestRunConfigHashUpdateNoPluginsFlagOmitsFingerprints(t *testing.T) {
	tmp := buildFingerprintFixture(t, true)

	code, _, stderr := captureRunConfigHashUpdate(t, []string{"--config-dir", tmp})
	if code != 0 {
		t.Fatalf("runConfigHashUpdate code=%d stderr=%s", code, stderr)
	}

	m, err := config.LoadChecksums(tmp)
	if err != nil {
		t.Fatalf("LoadChecksums: %v", err)
	}
	if len(m.PluginFingerprints) != 0 {
		t.Fatalf("default behavior should NOT emit fingerprints, got %+v", m.PluginFingerprints)
	}
}

func TestVerifyPluginFingerprintsForConfigHappyPath(t *testing.T) {
	tmp := buildFingerprintFixture(t, true)
	// Lock first, including plugins.
	if code, _, stderr := captureRunConfigHashUpdate(t, []string{"--config-dir", tmp, "--plugins"}); code != 0 {
		t.Fatalf("lock failed: %s", stderr)
	}

	if err := verifyPluginFingerprintsForConfig(filepath.Join(tmp, "config.yaml")); err != nil {
		t.Fatalf("verify should pass on unchanged bytes: %v", err)
	}
}

func TestVerifyPluginFingerprintsForConfigEntrypointTamperFails(t *testing.T) {
	tmp := buildFingerprintFixture(t, true)
	if code, _, stderr := captureRunConfigHashUpdate(t, []string{"--config-dir", tmp, "--plugins"}); code != 0 {
		t.Fatalf("lock failed: %s", stderr)
	}
	// Tamper with entrypoint.
	entryPath := filepath.Join(tmp, "plugins", "gmail", "gmail")
	if err := os.WriteFile(entryPath, []byte("#!/bin/sh\necho tampered\n"), 0755); err != nil {
		t.Fatalf("tamper: %v", err)
	}

	err := verifyPluginFingerprintsForConfig(filepath.Join(tmp, "config.yaml"))
	if err == nil {
		t.Fatal("expected error after entrypoint tampered")
	}
	if !strings.Contains(err.Error(), "gmail") || !strings.Contains(err.Error(), "entrypoint") {
		t.Fatalf("error should name plugin and entrypoint kind: %v", err)
	}
	if !strings.Contains(err.Error(), "ductile config lock --plugins") {
		t.Fatalf("error should include recovery command: %v", err)
	}
}

func TestVerifyPluginFingerprintsForConfigNoChecksumsIsNoOp(t *testing.T) {
	tmp := buildFingerprintFixture(t, true)
	// No lock at all.
	if err := verifyPluginFingerprintsForConfig(filepath.Join(tmp, "config.yaml")); err != nil {
		t.Fatalf("verify should no-op when .checksums absent: %v", err)
	}
}

func TestVerifyPluginFingerprintsForConfigNoPluginSectionIsNoOp(t *testing.T) {
	tmp := buildFingerprintFixture(t, true)
	// Lock WITHOUT --plugins so plugin_fingerprints is omitted.
	if code, _, stderr := captureRunConfigHashUpdate(t, []string{"--config-dir", tmp}); code != 0 {
		t.Fatalf("lock failed: %s", stderr)
	}
	if err := verifyPluginFingerprintsForConfig(filepath.Join(tmp, "config.yaml")); err != nil {
		t.Fatalf("verify should no-op when plugin_fingerprints section absent: %v", err)
	}
}

// TestVerifyPluginFingerprintsForConfigManifestTamperFails parallels the
// entrypoint-tamper case but flips the manifest bytes, exercising the
// ManifestHash-mismatch branch of VerifyPluginFingerprints end-to-end.
func TestVerifyPluginFingerprintsForConfigManifestTamperFails(t *testing.T) {
	tmp := buildFingerprintFixture(t, true)
	if code, _, stderr := captureRunConfigHashUpdate(t, []string{"--config-dir", tmp, "--plugins"}); code != 0 {
		t.Fatalf("lock failed: %s", stderr)
	}
	manPath := filepath.Join(tmp, "plugins", "gmail", "manifest.yaml")
	tampered := `manifest_spec: ductile.plugin
manifest_version: 1
name: gmail
version: 9.9.9
protocol: 2
entrypoint: gmail
commands:
  - name: poll
    type: write
`
	if err := os.WriteFile(manPath, []byte(tampered), 0644); err != nil {
		t.Fatalf("tamper manifest: %v", err)
	}

	err := verifyPluginFingerprintsForConfig(filepath.Join(tmp, "config.yaml"))
	if err == nil {
		t.Fatal("expected error after manifest tamper")
	}
	if !strings.Contains(err.Error(), "gmail") || !strings.Contains(err.Error(), "manifest") {
		t.Fatalf("error should name plugin and manifest kind: %v", err)
	}
	if !strings.Contains(err.Error(), "ductile config lock --plugins") {
		t.Fatalf("error should include recovery command: %v", err)
	}
}

// TestRunConfigHashUpdatePluginsFlagEmbedsAlias exercises the alias path end
// to end: a second plugin entry with `uses: gmail` must share the base's
// paths and hashes, and carry Uses in the manifest.
func TestRunConfigHashUpdatePluginsFlagEmbedsAlias(t *testing.T) {
	tmp := buildFingerprintFixture(t, true)
	// Rewrite config.yaml to add gmail-work: uses: gmail alongside gmail.
	aliasConfig := `
plugin_roots:
  - plugins
service:
  allow_symlinks: true
plugins:
  gmail:
    enabled: true
  gmail-work:
    enabled: true
    uses: gmail
`
	if err := os.WriteFile(filepath.Join(tmp, "config.yaml"), []byte(aliasConfig), 0644); err != nil {
		t.Fatalf("rewrite config.yaml: %v", err)
	}

	if code, _, stderr := captureRunConfigHashUpdate(t, []string{"--config-dir", tmp, "--plugins"}); code != 0 {
		t.Fatalf("lock failed: %s", stderr)
	}
	m, err := config.LoadChecksums(tmp)
	if err != nil {
		t.Fatalf("LoadChecksums: %v", err)
	}
	if len(m.PluginFingerprints) != 2 {
		t.Fatalf("want 2 fingerprints (base + alias), got %d", len(m.PluginFingerprints))
	}
	// Sorted by name: gmail, gmail-work
	base := m.PluginFingerprints[0]
	alias := m.PluginFingerprints[1]
	if alias.Name != "gmail-work" || alias.Uses != "gmail" {
		t.Fatalf("alias not recorded with Uses: %+v", alias)
	}
	if alias.ManifestHash != base.ManifestHash || alias.EntrypointHash != base.EntrypointHash {
		t.Fatalf("alias should share base hashes: base=%+v alias=%+v", base, alias)
	}

	// Now verify: no tampering, should pass cleanly.
	if err := verifyPluginFingerprintsForConfig(filepath.Join(tmp, "config.yaml")); err != nil {
		t.Fatalf("verify should pass for locked alias pair: %v", err)
	}
}

// TestVerifyPluginFingerprintsForConfigDisabledTamperIsNotFatal confirms
// that tampering a disabled plugin yields warnings-only (non-fatal) so
// reload does NOT reject the config.
func TestVerifyPluginFingerprintsForConfigDisabledTamperIsNotFatal(t *testing.T) {
	tmp := buildFingerprintFixture(t, false) // disabled
	if code, _, stderr := captureRunConfigHashUpdate(t, []string{"--config-dir", tmp, "--plugins"}); code != 0 {
		t.Fatalf("lock failed: %s", stderr)
	}
	// Tamper disabled plugin's entrypoint.
	if err := os.WriteFile(filepath.Join(tmp, "plugins", "gmail", "gmail"), []byte("rebuilt\n"), 0755); err != nil {
		t.Fatalf("tamper: %v", err)
	}

	if err := verifyPluginFingerprintsForConfig(filepath.Join(tmp, "config.yaml")); err != nil {
		t.Fatalf("disabled plugin tamper must not fail verify (warn-only): %v", err)
	}
}

// TestRunConfigHashUpdatePluginsDryRunLeavesChecksumsUntouched verifies
// --plugins --dry-run hashes everything (still errors on missing plugin
// etc.) but never writes .checksums, so operators can sanity-check before
// committing.
func TestRunConfigHashUpdatePluginsDryRunLeavesChecksumsUntouched(t *testing.T) {
	tmp := buildFingerprintFixture(t, true)
	// Seed an existing .checksums (without plugin section) so we can confirm
	// dry-run does NOT overwrite it.
	if code, _, stderr := captureRunConfigHashUpdate(t, []string{"--config-dir", tmp}); code != 0 {
		t.Fatalf("seed lock failed: %s", stderr)
	}
	before, err := os.ReadFile(filepath.Join(tmp, ".checksums"))
	if err != nil {
		t.Fatalf("read seed checksums: %v", err)
	}

	if code, _, stderr := captureRunConfigHashUpdate(t, []string{"--config-dir", tmp, "--plugins", "--dry-run"}); code != 0 {
		t.Fatalf("dry-run lock failed: %s", stderr)
	}

	after, err := os.ReadFile(filepath.Join(tmp, ".checksums"))
	if err != nil {
		t.Fatalf("read checksums after dry-run: %v", err)
	}
	if string(before) != string(after) {
		t.Fatalf("--plugins --dry-run must not modify .checksums\nbefore=%s\nafter=%s", before, after)
	}
}

// TestVerifyPluginFingerprintsForConfigStaleRecordNotFatal captures the
// case where the operator locked a plugin, then removed it from config.yaml
// without re-locking. Classification is warning-only, so verify must not
// reject the reload.
func TestVerifyPluginFingerprintsForConfigStaleRecordNotFatal(t *testing.T) {
	tmp := buildFingerprintFixture(t, true)
	if code, _, stderr := captureRunConfigHashUpdate(t, []string{"--config-dir", tmp, "--plugins"}); code != 0 {
		t.Fatalf("lock failed: %s", stderr)
	}

	// Rewrite config.yaml removing gmail entirely (but keeping plugin files).
	cleaned := `
plugin_roots:
  - plugins
service:
  allow_symlinks: true
plugins: {}
`
	if err := os.WriteFile(filepath.Join(tmp, "config.yaml"), []byte(cleaned), 0644); err != nil {
		t.Fatalf("rewrite config.yaml: %v", err)
	}
	if err := verifyPluginFingerprintsForConfig(filepath.Join(tmp, "config.yaml")); err != nil {
		t.Fatalf("stale fingerprint record must be warn-only, got error: %v", err)
	}
}

// TestRunConfigHashUpdatePluginsRelockOverwritesCleanly verifies that
// running the lock twice produces a valid manifest matching the second
// run (idempotent overwrite via writeChecksumsAtomic), with no stale
// artifacts left behind.
func TestRunConfigHashUpdatePluginsRelockOverwritesCleanly(t *testing.T) {
	tmp := buildFingerprintFixture(t, true)
	if code, _, stderr := captureRunConfigHashUpdate(t, []string{"--config-dir", tmp, "--plugins"}); code != 0 {
		t.Fatalf("first lock failed: %s", stderr)
	}
	first, _ := config.LoadChecksums(tmp)

	// Modify the plugin entrypoint between locks.
	if err := os.WriteFile(filepath.Join(tmp, "plugins", "gmail", "gmail"), []byte("v2\n"), 0755); err != nil {
		t.Fatalf("modify entrypoint: %v", err)
	}

	if code, _, stderr := captureRunConfigHashUpdate(t, []string{"--config-dir", tmp, "--plugins"}); code != 0 {
		t.Fatalf("relock failed: %s", stderr)
	}
	second, _ := config.LoadChecksums(tmp)

	if first.PluginFingerprints[0].EntrypointHash == second.PluginFingerprints[0].EntrypointHash {
		t.Fatal("relock should produce a new entrypoint hash for modified bytes")
	}

	// No .checksums.tmp-* artifacts should remain after the two locks.
	entries, err := os.ReadDir(tmp)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".checksums.tmp-") {
			t.Fatalf("stray atomic-write temp file after relock: %s", e.Name())
		}
	}
}
