package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyIntegrityAllValid(t *testing.T) {
	tmpDir := t.TempDir()
	setupIntegrityDir(t, tmpDir)

	files, err := DiscoverConfigFiles(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	// Generate checksums for all discovered files
	if err := GenerateChecksumsFromDiscovery(files, false); err != nil {
		t.Fatal(err)
	}

	result, err := VerifyIntegrity(tmpDir, files)
	if err != nil {
		t.Fatal(err)
	}

	if !result.Passed {
		t.Errorf("expected Passed=true, got errors: %v", result.Errors)
	}
	if len(result.Warnings) > 0 {
		t.Errorf("unexpected warnings: %v", result.Warnings)
	}
}

func TestVerifyIntegrityHighSecurityMismatch(t *testing.T) {
	tmpDir := t.TempDir()
	setupIntegrityDir(t, tmpDir)

	files, err := DiscoverConfigFiles(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	// Generate checksums
	if err := GenerateChecksumsFromDiscovery(files, false); err != nil {
		t.Fatal(err)
	}

	// Tamper with tokens.yaml (high security)
	writeTestFile(t, filepath.Join(tmpDir, "tokens.yaml"), "tokens:\n  - name: tampered\n    key: hacked\n")

	result, err := VerifyIntegrity(tmpDir, files)
	if err != nil {
		t.Fatal(err)
	}

	if result.Passed {
		t.Fatal("expected Passed=false for high-security file mismatch")
	}
	if len(result.Errors) == 0 {
		t.Fatal("expected errors for high-security file mismatch")
	}
	if !strings.Contains(result.Errors[0], "hash mismatch") {
		t.Errorf("error should mention hash mismatch, got: %s", result.Errors[0])
	}
}

func TestVerifyIntegrityOperationalMismatchWarns(t *testing.T) {
	tmpDir := t.TempDir()
	setupIntegrityDir(t, tmpDir)

	files, err := DiscoverConfigFiles(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	// Generate checksums
	if err := GenerateChecksumsFromDiscovery(files, false); err != nil {
		t.Fatal(err)
	}

	// Tamper with a plugin file (operational)
	writeTestFile(t, filepath.Join(tmpDir, "plugins", "echo.yaml"), "plugins:\n  echo:\n    enabled: false\n")

	result, err := VerifyIntegrity(tmpDir, files)
	if err != nil {
		t.Fatal(err)
	}

	if !result.Passed {
		t.Fatal("operational mismatch should not cause hard fail")
	}
	if len(result.Warnings) == 0 {
		t.Fatal("expected warnings for operational file mismatch")
	}
}

func TestVerifyIntegrityNoManifestWithHighSecurity(t *testing.T) {
	tmpDir := t.TempDir()
	setupIntegrityDir(t, tmpDir)

	files, err := DiscoverConfigFiles(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	// Don't generate checksums — should hard fail because tokens.yaml exists
	result, err := VerifyIntegrity(tmpDir, files)
	if err != nil {
		t.Fatal(err)
	}

	if result.Passed {
		t.Fatal("expected Passed=false when no manifest + high-security files")
	}
}

func TestVerifyIntegrityNoManifestNoHighSecurity(t *testing.T) {
	tmpDir := t.TempDir()
	writeTestFile(t, filepath.Join(tmpDir, "config.yaml"), "service:\n  name: test\n")
	os.MkdirAll(filepath.Join(tmpDir, "plugins"), 0755)
	writeTestFile(t, filepath.Join(tmpDir, "plugins", "echo.yaml"), "plugins:\n  echo:\n    enabled: true\n")

	files, err := DiscoverConfigFiles(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	result, err := VerifyIntegrity(tmpDir, files)
	if err != nil {
		t.Fatal(err)
	}

	if !result.Passed {
		t.Fatal("no manifest + no high-security files should pass with warning")
	}
	if len(result.Warnings) == 0 {
		t.Fatal("expected warning about missing manifest")
	}
}

func setupIntegrityDir(t *testing.T, dir string) {
	t.Helper()
	writeTestFile(t, filepath.Join(dir, "config.yaml"), "service:\n  name: test\n  tick_interval: 60s\nstate:\n  path: ./test.db\nplugins_dir: ./plugins\n")
	writeTestFile(t, filepath.Join(dir, "tokens.yaml"), "tokens:\n  - name: admin\n    key: secret123\n")
	os.MkdirAll(filepath.Join(dir, "plugins"), 0755)
	writeTestFile(t, filepath.Join(dir, "plugins", "echo.yaml"), "plugins:\n  echo:\n    enabled: true\n    schedule:\n      every: 5m\n")
}
