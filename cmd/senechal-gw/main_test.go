package main

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/mattjoyce/senechal-gw/internal/config"
	"github.com/mattjoyce/senechal-gw/internal/storage"
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
	if !strings.Contains(stdout, "Usage: senechal-gw config check") {
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
	if !strings.Contains(stdout, "Usage: senechal-gw config <action>") {
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
	if !strings.Contains(stdout, "Usage: senechal-gw job inspect") {
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
	if !strings.Contains(stdout, "Usage: senechal-gw system start") {
		t.Fatalf("stdout missing start action help usage: %s", stdout)
	}
}

func TestPrintUsageUsesActionTerminology(t *testing.T) {
	_, stdout, _ := captureOutputWithExitCode(t, func() int {
		printUsage()
		return 0
	})
	if !strings.Contains(stdout, "senechal-gw <noun> <action> [flags]") {
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
	if !strings.Contains(stdout, "senechal-gw 1.2.3") {
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
    enabled: false
`
	if err := os.WriteFile(configPath, []byte(configYAML), 0644); err != nil {
		t.Fatal(err)
	}

	code, _, stderr := captureOutputWithExitCode(t, func() int {
		return runConfigSet([]string{"--config", configPath, "--apply", "plugin:echo.enabled=true"})
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
	if reloaded.Plugins["echo"].Enabled {
		t.Fatal("plugin:echo.enabled should remain false after failed apply")
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
