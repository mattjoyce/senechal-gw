package main

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mattjoyce/ductile/internal/config"
	"github.com/mattjoyce/ductile/internal/queue"
	"github.com/mattjoyce/ductile/internal/storage"
)

func captureOutputWithExitCode(t *testing.T, run func() int) (int, string, string) {
	t.Helper()

	oldStdout := os.Stdout
	oldStderr := os.Stderr

	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe stdout failed: %v", err)
	}
	stderrR, stderrW, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe stderr failed: %v", err)
	}

	os.Stdout = stdoutW
	os.Stderr = stderrW

	code := run()

	_ = stdoutW.Close()
	_ = stderrW.Close()
	os.Stdout = oldStdout
	os.Stderr = oldStderr

	stdoutBytes, _ := io.ReadAll(stdoutR)
	stderrBytes, _ := io.ReadAll(stderrR)

	_ = stdoutR.Close()
	_ = stderrR.Close()

	return code, string(stdoutBytes), string(stderrBytes)
}

func captureRunConfigHashUpdate(t *testing.T, args []string) (int, string, string) {
	t.Helper()
	return captureOutputWithExitCode(t, func() int {
		return runConfigHashUpdate(args)
	})
}

func setVersionMetadataForTest(t *testing.T, v, commit, built string) {
	t.Helper()

	origVersion := version
	origCommit := gitCommit
	origBuildDate := buildDate

	version = v
	gitCommit = commit
	buildDate = built

	t.Cleanup(func() {
		version = origVersion
		gitCommit = origCommit
		buildDate = origBuildDate
	})
}

func TestRunConfigHashUpdateVerboseDryRunShortFlag(t *testing.T) {
	tmpDir := t.TempDir()

	configPath := filepath.Join(tmpDir, "config.yaml")
	configYAML := `
include:
  - tokens.yaml
`
	if err := os.WriteFile(configPath, []byte(configYAML), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "tokens.yaml"), []byte("api_key: test\n"), 0600); err != nil {
		t.Fatal(err)
	}

	code, stdout, stderr := captureRunConfigHashUpdate(t, []string{"--config", configPath, "-v", "--dry-run"})
	if code != 0 {
		t.Fatalf("runConfigHashUpdate() code = %d, stderr: %s", code, stderr)
	}

	if !strings.Contains(stdout, "Processing directory") {
		t.Fatalf("stdout missing verbose directory progress: %s", stdout)
	}
	if !strings.Contains(stdout, "HASH tokens.yaml:") {
		t.Fatalf("stdout missing tokens hash line: %s", stdout)
	}
	if !strings.Contains(stdout, "SKIP webhooks.yaml: not found (optional)") {
		t.Fatalf("stdout missing optional skip line: %s", stdout)
	}
	if !strings.Contains(stdout, "DRY-RUN .checksums:") {
		t.Fatalf("stdout missing dry-run line: %s", stdout)
	}
	if !strings.Contains(stdout, "Dry run completed") {
		t.Fatalf("stdout missing dry-run summary: %s", stdout)
	}

	hashPattern := regexp.MustCompile(`HASH tokens\.yaml: [a-f0-9]{64}`)
	if !hashPattern.MatchString(stdout) {
		t.Fatalf("stdout missing valid hash output: %s", stdout)
	}

	if _, err := os.Stat(filepath.Join(tmpDir, ".checksums")); !os.IsNotExist(err) {
		t.Fatal(".checksums should not be written in dry-run mode")
	}
}

func TestRunConfigHashUpdateVerboseLongFlagWritesChecksums(t *testing.T) {
	tmpDir := t.TempDir()

	configPath := filepath.Join(tmpDir, "config.yaml")
	configYAML := `
include:
  - tokens.yaml
`
	if err := os.WriteFile(configPath, []byte(configYAML), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "tokens.yaml"), []byte("api_key: test\n"), 0600); err != nil {
		t.Fatal(err)
	}

	code, stdout, stderr := captureRunConfigHashUpdate(t, []string{"--config", configPath, "--verbose"})
	if code != 0 {
		t.Fatalf("runConfigHashUpdate() code = %d, stderr: %s", code, stderr)
	}

	if !strings.Contains(stdout, "WROTE .checksums:") {
		t.Fatalf("stdout missing wrote checksums line: %s", stdout)
	}
	if !strings.Contains(stdout, "Successfully locked configuration") {
		t.Fatalf("stdout missing success summary: %s", stdout)
	}

	if _, err := os.Stat(filepath.Join(tmpDir, ".checksums")); err != nil {
		t.Fatalf("expected .checksums to be written: %v", err)
	}
}

func TestRunConfigNounActionHelp(t *testing.T) {
	code, stdout, stderr := captureOutputWithExitCode(t, func() int {
		return runConfigNoun([]string{"check", "--help"})
	})
	if code != 0 {
		t.Fatalf("runConfigNoun() code = %d, stderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, "Usage: ductile config check") {
		t.Fatalf("stdout missing action help usage: %s", stdout)
	}
}

func TestRunConfigNounHelpTerminology(t *testing.T) {
	code, stdout, stderr := captureOutputWithExitCode(t, func() int {
		return runConfigNoun([]string{"--help"})
	})
	if code != 0 {
		t.Fatalf("runConfigNoun() code = %d, stderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, "Usage: ductile config <action>") {
		t.Fatalf("stdout missing action terminology: %s", stdout)
	}
	if strings.Contains(stdout, "<verb>") {
		t.Fatalf("stdout should not reference <verb>: %s", stdout)
	}
}

func TestRunJobNounActionHelp(t *testing.T) {
	code, stdout, stderr := captureOutputWithExitCode(t, func() int {
		return runJobNoun([]string{"inspect", "--help"})
	})
	if code != 0 {
		t.Fatalf("runJobNoun() code = %d, stderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, "Usage: ductile job inspect") {
		t.Fatalf("stdout missing inspect action help usage: %s", stdout)
	}
}

func TestRunSystemNounActionHelp(t *testing.T) {
	code, stdout, stderr := captureOutputWithExitCode(t, func() int {
		return runSystemNoun([]string{"start", "--help"})
	})
	if code != 0 {
		t.Fatalf("runSystemNoun() code = %d, stderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, "Usage: ductile system start") {
		t.Fatalf("stdout missing start action help usage: %s", stdout)
	}
}

func TestRunSystemNounResetActionHelp(t *testing.T) {
	code, stdout, stderr := captureOutputWithExitCode(t, func() int {
		return runSystemNoun([]string{"reset", "--help"})
	})
	if code != 0 {
		t.Fatalf("runSystemNoun() code = %d, stderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, "Usage: ductile system reset") {
		t.Fatalf("stdout missing reset action help usage: %s", stdout)
	}
}

func TestPrintUsageUsesActionTerminology(t *testing.T) {
	_, stdout, _ := captureOutputWithExitCode(t, func() int {
		printUsage()
		return 0
	})
	if !strings.Contains(stdout, "ductile <noun> <action> [flags]") {
		t.Fatalf("usage missing action terminology: %s", stdout)
	}
	if strings.Contains(stdout, "<noun> <verb>") {
		t.Fatalf("usage should not reference verb terminology: %s", stdout)
	}
}

func TestRunCLIRootVersionFlag(t *testing.T) {
	setVersionMetadataForTest(t, "1.2.3", "abc1234567890", "2026-02-12T11:30:00Z")

	code, stdout, stderr := captureOutputWithExitCode(t, func() int {
		return runCLI([]string{"--version"})
	})
	if code != 0 {
		t.Fatalf("runCLI() code = %d, stderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, "ductile 1.2.3") {
		t.Fatalf("stdout missing semantic version: %s", stdout)
	}
	if !strings.Contains(stdout, "commit: abc123456789") {
		t.Fatalf("stdout missing short commit: %s", stdout)
	}
	if !strings.Contains(stdout, "built_at: 2026-02-12T11:30:00Z") {
		t.Fatalf("stdout missing build time: %s", stdout)
	}
}

func TestRunVersionJSONOutputIncludesMetadata(t *testing.T) {
	setVersionMetadataForTest(t, "2.0.0-rc.1", "aabbccddeeff001122334455", "2026-02-12T11:30:00-05:00")

	code, stdout, stderr := captureOutputWithExitCode(t, func() int {
		return runVersion([]string{"--json"})
	})
	if code != 0 {
		t.Fatalf("runVersion() code = %d, stderr: %s", code, stderr)
	}

	var out struct {
		Version   string `json:"version"`
		Commit    string `json:"commit"`
		BuildTime string `json:"build_time"`
	}
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("failed to parse version JSON: %v\noutput=%s", err, stdout)
	}

	if out.Version != "2.0.0-rc.1" {
		t.Fatalf("version = %q, want %q", out.Version, "2.0.0-rc.1")
	}
	if out.Commit != "aabbccddeeff" {
		t.Fatalf("commit = %q, want %q", out.Commit, "aabbccddeeff")
	}
	if out.BuildTime != "2026-02-12T16:30:00Z" {
		t.Fatalf("build_time = %q, want %q", out.BuildTime, "2026-02-12T16:30:00Z")
	}
}

func TestRunConfigSetApplyRejectsInvalidConfigAndRollsBack(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	configYAML := `
service:
  name: test-gw
plugins:
  echo:
    enabled: true
    schedule:
      every: 5m
`
	if err := os.WriteFile(configPath, []byte(configYAML), 0644); err != nil {
		t.Fatal(err)
	}

	code, _, stderr := captureOutputWithExitCode(t, func() int {
		return runConfigSet([]string{"--config", configPath, "--apply", "plugins.echo.schedule.every="})
	})
	if code == 0 {
		t.Fatalf("runConfigSet() should fail for invalid apply, stderr: %s", stderr)
	}
	if !strings.Contains(stderr, "Apply failed: validation failed:") {
		t.Fatalf("stderr missing validation failure details: %s", stderr)
	}

	reloaded, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("config should still be valid after failed apply: %v", err)
	}
	if got := reloaded.Plugins["echo"].Schedule.Every; got != "5m" {
		t.Fatalf("plugins.echo.schedule.every should remain %q after failed apply, got %q", "5m", got)
	}
}

func TestRunSystemResetResetsCircuitBreaker(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "state.db")
	configPath := filepath.Join(tmpDir, "config.yaml")
	configYAML := `
state:
  path: ` + dbPath + `
plugins:
  echo:
    enabled: false
`
	if err := os.WriteFile(configPath, []byte(configYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	db, err := storage.OpenSQLite(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	q := queue.New(db)
	openedAt := time.Now().UTC().Add(-5 * time.Minute)
	if err := q.UpsertCircuitBreaker(context.Background(), queue.CircuitBreaker{
		Plugin:       "echo",
		Command:      "poll",
		State:        queue.CircuitOpen,
		FailureCount: 3,
		OpenedAt:     &openedAt,
	}); err != nil {
		t.Fatalf("UpsertCircuitBreaker: %v", err)
	}

	code, stdout, stderr := captureOutputWithExitCode(t, func() int {
		return runSystemReset([]string{"--config", configPath, "echo"})
	})
	if code != 0 {
		t.Fatalf("runSystemReset() code = %d, stderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, "Reset circuit breaker for echo (poll)") {
		t.Fatalf("stdout missing reset confirmation: %s", stdout)
	}

	got, err := q.GetCircuitBreaker(context.Background(), "echo", "poll")
	if err != nil {
		t.Fatalf("GetCircuitBreaker: %v", err)
	}
	if got == nil {
		t.Fatal("expected circuit breaker row after reset")
	}
	if got.State != queue.CircuitClosed {
		t.Fatalf("state=%q want %q", got.State, queue.CircuitClosed)
	}
	if got.FailureCount != 0 {
		t.Fatalf("failure_count=%d want 0", got.FailureCount)
	}
}

func writeConfigDirFixture(t *testing.T, dir string) {
	t.Helper()

	pluginsDir := filepath.Join(dir, "plugins")
	if err := os.MkdirAll(pluginsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	configYAML := `
service:
  tick_interval: 60s
  log_level: info
state:
  path: ` + filepath.Join(dir, "state.db") + `
plugins_dir: ` + pluginsDir + `
plugins: {}
`
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(configYAML), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRunConfigTokenCreateAndInspectJSON(t *testing.T) {
	tmpDir := t.TempDir()
	writeConfigDirFixture(t, tmpDir)

	createCode, createStdout, createStderr := captureOutputWithExitCode(t, func() int {
		return runConfigTokenCreate([]string{
			"--config-dir", tmpDir,
			"--name", "github-integration",
			"--scopes", "read:jobs,read:events",
			"--format", "json",
		})
	})
	if createCode != 0 {
		t.Fatalf("runConfigTokenCreate() code = %d, stderr: %s", createCode, createStderr)
	}

	var createOut struct {
		Status string `json:"status"`
		Token  struct {
			Name       string `json:"name"`
			ScopesFile string `json:"scopes_file"`
			ScopesHash string `json:"scopes_hash"`
		} `json:"token"`
		EnvVar string `json:"env_var"`
	}
	if err := json.Unmarshal([]byte(createStdout), &createOut); err != nil {
		t.Fatalf("failed to parse create json: %v\noutput=%s", err, createStdout)
	}
	if createOut.Status != "success" {
		t.Fatalf("status = %q, want success", createOut.Status)
	}
	if createOut.Token.Name != "github-integration" {
		t.Fatalf("token.name = %q", createOut.Token.Name)
	}
	if !strings.HasPrefix(createOut.Token.ScopesHash, "blake3:") {
		t.Fatalf("token.scopes_hash missing blake3 prefix: %q", createOut.Token.ScopesHash)
	}
	if createOut.EnvVar != "GITHUB_INTEGRATION_TOKEN" {
		t.Fatalf("env_var = %q, want %q", createOut.EnvVar, "GITHUB_INTEGRATION_TOKEN")
	}

	if _, err := os.Stat(filepath.Join(tmpDir, "tokens.yaml")); err != nil {
		t.Fatalf("tokens.yaml not written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "scopes", "github-integration.json")); err != nil {
		t.Fatalf("scope file not written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmpDir, ".checksums")); err != nil {
		t.Fatalf(".checksums not written: %v", err)
	}

	inspectCode, inspectStdout, inspectStderr := captureOutputWithExitCode(t, func() int {
		return runConfigTokenInspect([]string{
			"github-integration",
			"--config-dir", tmpDir,
			"--format", "json",
		})
	})
	if inspectCode != 0 {
		t.Fatalf("runConfigTokenInspect() code = %d, stderr: %s", inspectCode, inspectStderr)
	}

	var inspectOut struct {
		Name        string `json:"name"`
		HashMatches bool   `json:"hash_matches"`
	}
	if err := json.Unmarshal([]byte(inspectStdout), &inspectOut); err != nil {
		t.Fatalf("failed to parse inspect json: %v\noutput=%s", err, inspectStdout)
	}
	if inspectOut.Name != "github-integration" {
		t.Fatalf("inspect name = %q", inspectOut.Name)
	}
	if !inspectOut.HashMatches {
		t.Fatalf("inspect hash_matches = false, output=%s", inspectStdout)
	}
}

func TestRunConfigScopeAddWithPositionalBeforeFlags(t *testing.T) {
	tmpDir := t.TempDir()
	writeConfigDirFixture(t, tmpDir)

	createCode, _, createStderr := captureOutputWithExitCode(t, func() int {
		return runConfigTokenCreate([]string{
			"--config-dir", tmpDir,
			"--name", "github-integration",
			"--scopes", "read:jobs",
		})
	})
	if createCode != 0 {
		t.Fatalf("runConfigTokenCreate() code = %d, stderr: %s", createCode, createStderr)
	}

	scopeCode, _, scopeStderr := captureOutputWithExitCode(t, func() int {
		return runConfigScopeAdd([]string{
			"github-integration",
			"echo:ro",
			"--config-dir", tmpDir,
		})
	})
	if scopeCode != 0 {
		t.Fatalf("runConfigScopeAdd() code = %d, stderr: %s", scopeCode, scopeStderr)
	}

	raw, err := os.ReadFile(filepath.Join(tmpDir, "scopes", "github-integration.json"))
	if err != nil {
		t.Fatalf("read scope file: %v", err)
	}
	var doc struct {
		Scopes []string `json:"scopes"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse scope file: %v", err)
	}

	if !containsString(doc.Scopes, "read:jobs") || !containsString(doc.Scopes, "echo:ro") {
		t.Fatalf("scope file missing expected scopes: %v", doc.Scopes)
	}
}

func containsString(list []string, value string) bool {
	return slices.Contains(list, value)
}

func writeRouteFixture(t *testing.T, dir string) {
	t.Helper()

	pluginsDir := filepath.Join(dir, "plugins")
	if err := os.MkdirAll(pluginsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	configYAML := `
service:
  tick_interval: 60s
  log_level: info
state:
  path: ` + filepath.Join(dir, "state.db") + `
plugins_dir: ` + pluginsDir + `
plugins:
  withings:
    enabled: false
    schedule:
      every: daily
  slack:
    enabled: false
    schedule:
      every: daily
`
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(configYAML), 0o644); err != nil {
		t.Fatal(err)
	}
}

func createTestPlugin(t *testing.T, pluginsDir, name string) {
	t.Helper()

	pluginDir := filepath.Join(pluginsDir, name)
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `
name: ` + name + `
version: "1.0.0"
protocol: 2
entrypoint: run.sh
commands:
  - name: handle
    type: write
`
	if err := os.WriteFile(filepath.Join(pluginDir, "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\nexit 0\n"
	if err := os.WriteFile(filepath.Join(pluginDir, "run.sh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestRunConfigPluginSetCreatesPluginFile(t *testing.T) {
	tmpDir := t.TempDir()
	writeConfigDirFixture(t, tmpDir)

	code, _, stderr := captureOutputWithExitCode(t, func() int {
		return runConfigPluginSet([]string{
			"echo",
			"schedule.every",
			"2h",
			"--config-dir", tmpDir,
		})
	})
	if code != 0 {
		t.Fatalf("runConfigPluginSet() code = %d, stderr: %s", code, stderr)
	}

	pluginFile := filepath.Join(tmpDir, "plugins", "echo.yaml")
	raw, err := os.ReadFile(pluginFile)
	if err != nil {
		t.Fatalf("read plugin file: %v", err)
	}
	if !strings.Contains(string(raw), "every: 2h") {
		t.Fatalf("plugin file missing updated schedule: %s", string(raw))
	}
}

func TestRunConfigRouteAddAndRemove(t *testing.T) {
	tmpDir := t.TempDir()
	writeRouteFixture(t, tmpDir)

	addCode, _, addStderr := captureOutputWithExitCode(t, func() int {
		return runConfigRouteAdd([]string{
			"--config-dir", tmpDir,
			"--from", "withings",
			"--event", "weight_updated",
			"--to", "slack",
		})
	})
	if addCode != 0 {
		t.Fatalf("runConfigRouteAdd() code = %d, stderr: %s", addCode, addStderr)
	}

	raw, err := os.ReadFile(filepath.Join(tmpDir, "routes.yaml"))
	if err != nil {
		t.Fatalf("read routes file: %v", err)
	}
	if !strings.Contains(string(raw), "withings") || !strings.Contains(string(raw), "weight_updated") {
		t.Fatalf("route not written: %s", string(raw))
	}

	removeCode, _, removeStderr := captureOutputWithExitCode(t, func() int {
		return runConfigRouteRemove([]string{
			"--config-dir", tmpDir,
			"--from", "withings",
			"--event", "weight_updated",
			"--to", "slack",
		})
	})
	if removeCode != 0 {
		t.Fatalf("runConfigRouteRemove() code = %d, stderr: %s", removeCode, removeStderr)
	}
}

func TestRunConfigWebhookAddAndList(t *testing.T) {
	tmpDir := t.TempDir()
	writeConfigDirFixture(t, tmpDir)
	pluginsDir := filepath.Join(tmpDir, "plugins")
	createTestPlugin(t, pluginsDir, "github-handler")

	addCode, _, addStderr := captureOutputWithExitCode(t, func() int {
		return runConfigWebhookAdd([]string{
			"--config-dir", tmpDir,
			"--name", "github",
			"--path", "/webhook/github",
			"--plugin", "github-handler",
			"--secret", "abc123",
		})
	})
	if addCode != 0 && addCode != 2 {
		t.Fatalf("runConfigWebhookAdd() code = %d, stderr: %s", addCode, addStderr)
	}

	listCode, stdout, listStderr := captureOutputWithExitCode(t, func() int {
		return runConfigWebhookList([]string{"--config-dir", tmpDir, "--format", "json"})
	})
	if listCode != 0 {
		t.Fatalf("runConfigWebhookList() code = %d, stderr: %s", listCode, listStderr)
	}

	var hooks []struct {
		Name   string `json:"name"`
		Path   string `json:"path"`
		Plugin string `json:"plugin"`
	}
	if err := json.Unmarshal([]byte(stdout), &hooks); err != nil {
		t.Fatalf("failed to parse webhook list json: %v\noutput=%s", err, stdout)
	}
	if len(hooks) != 1 || hooks[0].Path != "/webhook/github" || hooks[0].Plugin != "github-handler" {
		t.Fatalf("unexpected webhook list: %+v", hooks)
	}
}

func TestRunConfigInitBackupRestore(t *testing.T) {
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, "cfg")

	initCode, _, initStderr := captureOutputWithExitCode(t, func() int {
		return runConfigInit([]string{"--config-dir", configDir})
	})
	if initCode != 0 {
		t.Fatalf("runConfigInit() code = %d, stderr: %s", initCode, initStderr)
	}
	if _, err := os.Stat(filepath.Join(configDir, "config.yaml")); err != nil {
		t.Fatalf("config.yaml missing after init: %v", err)
	}

	backupPath := filepath.Join(tmpDir, "backup.tar.gz")
	backupCode, _, backupStderr := captureOutputWithExitCode(t, func() int {
		return runConfigBackup([]string{"--config-dir", configDir, "--output", backupPath})
	})
	if backupCode != 0 {
		t.Fatalf("runConfigBackup() code = %d, stderr: %s", backupCode, backupStderr)
	}
	if _, err := os.Stat(backupPath); err != nil {
		t.Fatalf("backup archive missing: %v", err)
	}

	if err := os.WriteFile(filepath.Join(configDir, "tokens.yaml"), []byte("tokens:\n  - name: changed\n    key: x\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	restoreCode, _, restoreStderr := captureOutputWithExitCode(t, func() int {
		return runConfigRestore([]string{backupPath, "--config-dir", configDir})
	})
	if restoreCode != 0 {
		t.Fatalf("runConfigRestore() code = %d, stderr: %s", restoreCode, restoreStderr)
	}

	raw, err := os.ReadFile(filepath.Join(configDir, "tokens.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "changed") {
		t.Fatalf("restore did not replace modified tokens.yaml: %s", string(raw))
	}
}

func TestRunConfigCheckSupportsConfigDirFlag(t *testing.T) {
	tmpDir := t.TempDir()
	writeConfigDirFixture(t, tmpDir)

	code, _, stderr := captureOutputWithExitCode(t, func() int {
		return runConfigCheck([]string{"--config-dir", tmpDir})
	})
	if code != 0 {
		t.Fatalf("runConfigCheck() code = %d, stderr: %s", code, stderr)
	}
}

func TestRunConfigGetAndSetSupportConfigDirFlag(t *testing.T) {
	tmpDir := t.TempDir()
	writeConfigDirFixture(t, tmpDir)

	setCode, _, setStderr := captureOutputWithExitCode(t, func() int {
		return runConfigSet([]string{"--config-dir", tmpDir, "--apply", "service.log_level=debug"})
	})
	if setCode != 0 {
		t.Fatalf("runConfigSet() code = %d, stderr: %s", setCode, setStderr)
	}

	getCode, stdout, getStderr := captureOutputWithExitCode(t, func() int {
		return runConfigGet([]string{"--config-dir", tmpDir, "service.log_level"})
	})
	if getCode != 0 {
		t.Fatalf("runConfigGet() code = %d, stderr: %s", getCode, getStderr)
	}
	if !strings.Contains(stdout, "debug") {
		t.Fatalf("runConfigGet() output missing updated value: %s", stdout)
	}
}

func TestRunSystemStatusJSONHealthy(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "state.db")
	configPath := filepath.Join(tmpDir, "config.yaml")

	if err := os.MkdirAll(filepath.Join(tmpDir, "plugins"), 0o755); err != nil {
		t.Fatal(err)
	}
	db, err := storage.OpenSQLite(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	_ = db.Close()

	configYAML := `
service:
  tick_interval: 30s
  log_level: info
state:
  path: ` + dbPath + `
plugins_dir: ` + filepath.Join(tmpDir, "plugins") + `
plugins: {}
`
	if err := os.WriteFile(configPath, []byte(configYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	code, stdout, stderr := captureOutputWithExitCode(t, func() int {
		return runSystemStatus([]string{"--config", configPath, "--json"})
	})
	if code != 0 {
		t.Fatalf("runSystemStatus() code = %d, stderr: %s", code, stderr)
	}

	var report struct {
		Healthy bool `json:"healthy"`
		Checks  []struct {
			Name string `json:"name"`
			OK   bool   `json:"ok"`
		} `json:"checks"`
	}
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatalf("failed to parse JSON status output: %v\noutput=%s", err, stdout)
	}
	if !report.Healthy {
		t.Fatalf("expected healthy=true, got false; output=%s", stdout)
	}
	if len(report.Checks) < 4 {
		t.Fatalf("expected at least 4 checks, got %d", len(report.Checks))
	}
}

func TestRunSystemStatusConfigLoadFailure(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte("invalid: [yaml"), 0o644); err != nil {
		t.Fatal(err)
	}

	code, stdout, _ := captureOutputWithExitCode(t, func() int {
		return runSystemStatus([]string{"--config", configPath})
	})
	if code == 0 {
		t.Fatalf("runSystemStatus() should fail for invalid config; stdout=%s", stdout)
	}
	if !strings.Contains(stdout, "config_load: FAIL") {
		t.Fatalf("expected config_load failure in output; stdout=%s", stdout)
	}
	if !strings.Contains(stdout, "state_db: FAIL") || !strings.Contains(stdout, "pid_lock: FAIL") {
		t.Fatalf("expected dependent checks to fail when config load fails; stdout=%s", stdout)
	}
}

func TestRunSystemStatusDetectsActivePIDLock(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "state.db")
	configPath := filepath.Join(tmpDir, "config.yaml")

	if err := os.MkdirAll(filepath.Join(tmpDir, "plugins"), 0o755); err != nil {
		t.Fatal(err)
	}
	db, err := storage.OpenSQLite(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	_ = db.Close()

	configYAML := `
service:
  tick_interval: 30s
  log_level: info
state:
  path: ` + dbPath + `
plugins_dir: ` + filepath.Join(tmpDir, "plugins") + `
plugins: {}
`
	if err := os.WriteFile(configPath, []byte(configYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := loadConfigForTool(configPath)
	if err != nil {
		t.Fatalf("loadConfigForTool: %v", err)
	}
	lockPath := getPIDLockPath(cfg)
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lockPath, []byte(strconv.Itoa(os.Getpid())+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	code, stdout, stderr := captureOutputWithExitCode(t, func() int {
		return runSystemStatus([]string{"--config", configPath, "--json"})
	})
	if code == 0 {
		t.Fatalf("runSystemStatus() should fail when active pid lock exists; stderr=%s stdout=%s", stderr, stdout)
	}

	var report struct {
		Healthy bool `json:"healthy"`
		Checks  []struct {
			Name      string `json:"name"`
			OK        bool   `json:"ok"`
			ActivePID int    `json:"active_pid"`
		} `json:"checks"`
	}
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatalf("failed to parse JSON status output: %v\noutput=%s", err, stdout)
	}
	if report.Healthy {
		t.Fatalf("expected healthy=false when active lock exists; output=%s", stdout)
	}

	foundPIDCheck := false
	for _, c := range report.Checks {
		if c.Name == "pid_lock" {
			foundPIDCheck = true
			if c.OK {
				t.Fatalf("expected pid_lock check to fail when active pid exists; output=%s", stdout)
			}
			if c.ActivePID != os.Getpid() {
				t.Fatalf("expected active_pid=%d, got %d", os.Getpid(), c.ActivePID)
			}
		}
	}
	if !foundPIDCheck {
		t.Fatalf("expected pid_lock check in output; output=%s", stdout)
	}
}
