package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGenerateChecksumsWithReportDryRun(t *testing.T) {
	tmpDir := t.TempDir()

	if err := os.WriteFile(filepath.Join(tmpDir, "tokens.yaml"), []byte("api_key: test\n"), 0600); err != nil {
		t.Fatal(err)
	}

	report, err := GenerateChecksumsWithReport(tmpDir, []string{"tokens.yaml", "webhooks.yaml"}, true)
	if err != nil {
		t.Fatalf("GenerateChecksumsWithReport() failed: %v", err)
	}

	if report.Written {
		t.Fatal("report.Written = true, want false in dry-run")
	}

	if len(report.Files) != 2 {
		t.Fatalf("len(report.Files) = %d, want 2", len(report.Files))
	}

	if !report.Files[0].Exists || report.Files[0].Hash == "" {
		t.Fatal("tokens.yaml should exist with computed hash")
	}
	if report.Files[1].Exists || report.Files[1].Hash != "" {
		t.Fatal("webhooks.yaml should be reported as missing without hash")
	}

	if _, err := os.Stat(filepath.Join(tmpDir, ".checksums")); !os.IsNotExist(err) {
		t.Fatal(".checksums should not be written in dry-run mode")
	}
}

func TestGenerateChecksumsWithReportWritesChecksums(t *testing.T) {
	tmpDir := t.TempDir()

	if err := os.WriteFile(filepath.Join(tmpDir, "tokens.yaml"), []byte("api_key: test\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "webhooks.yaml"), []byte("listen: :8080\n"), 0600); err != nil {
		t.Fatal(err)
	}

	report, err := GenerateChecksumsWithReport(tmpDir, []string{"tokens.yaml", "webhooks.yaml"}, false)
	if err != nil {
		t.Fatalf("GenerateChecksumsWithReport() failed: %v", err)
	}

	if !report.Written {
		t.Fatal("report.Written = false, want true")
	}

	if _, err := os.Stat(filepath.Join(tmpDir, ".checksums")); err != nil {
		t.Fatalf("expected .checksums to be written: %v", err)
	}

	manifest, err := LoadChecksums(tmpDir)
	if err != nil {
		t.Fatalf("LoadChecksums() failed: %v", err)
	}
	if len(manifest.Hashes) != 2 {
		t.Fatalf("len(manifest.Hashes) = %d, want 2", len(manifest.Hashes))
	}
}
