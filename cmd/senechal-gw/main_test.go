package main

import (
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
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

	if !strings.Contains(stdout, "Processing directory:") {
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
