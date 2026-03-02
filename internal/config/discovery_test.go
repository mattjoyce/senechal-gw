package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverConfigFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Create mandatory config.yaml
	writeTestFile(t, filepath.Join(tmpDir, "config.yaml"), "service:\n  name: test\n")

	// Create optional named files
	writeTestFile(t, filepath.Join(tmpDir, "tokens.yaml"), "tokens: []\n")
	writeTestFile(t, filepath.Join(tmpDir, "webhooks.yaml"), "webhooks: []\n")
	writeTestFile(t, filepath.Join(tmpDir, "routes.yaml"), "routes: []\n")

	// Create scopes directory
	os.MkdirAll(filepath.Join(tmpDir, "scopes"), 0755)
	writeTestFile(t, filepath.Join(tmpDir, "scopes", "admin.json"), `{"scopes":["*"]}`)

	cf, err := DiscoverConfigFiles(tmpDir)
	if err != nil {
		t.Fatalf("DiscoverConfigFiles() failed: %v", err)
	}

	if cf.Config != filepath.Join(tmpDir, "config.yaml") {
		t.Errorf("Config = %q", cf.Config)
	}
	if cf.Tokens != filepath.Join(tmpDir, "tokens.yaml") {
		t.Errorf("Tokens = %q", cf.Tokens)
	}
	if cf.Webhooks != filepath.Join(tmpDir, "webhooks.yaml") {
		t.Errorf("Webhooks = %q", cf.Webhooks)
	}
	if cf.Routes != filepath.Join(tmpDir, "routes.yaml") {
		t.Errorf("Routes = %q", cf.Routes)
	}
	if len(cf.Plugins) != 0 {
		t.Fatalf("len(Plugins) = %d, want 0", len(cf.Plugins))
	}
	if len(cf.Pipelines) != 0 {
		t.Fatalf("len(Pipelines) = %d, want 0", len(cf.Pipelines))
	}
	if len(cf.Scopes) != 1 {
		t.Fatalf("len(Scopes) = %d, want 1", len(cf.Scopes))
	}
}

func TestDiscoverConfigFilesMissingConfigYAML(t *testing.T) {
	tmpDir := t.TempDir()

	_, err := DiscoverConfigFiles(tmpDir)
	if err == nil {
		t.Fatal("DiscoverConfigFiles() should fail when config.yaml is missing")
	}
}

func TestDiscoverConfigFilesMinimal(t *testing.T) {
	tmpDir := t.TempDir()
	writeTestFile(t, filepath.Join(tmpDir, "config.yaml"), "service:\n  name: test\n")

	cf, err := DiscoverConfigFiles(tmpDir)
	if err != nil {
		t.Fatalf("DiscoverConfigFiles() failed: %v", err)
	}

	if cf.Tokens != "" {
		t.Errorf("Tokens should be empty, got %q", cf.Tokens)
	}
	if len(cf.Plugins) != 0 {
		t.Errorf("Plugins should be empty, got %d", len(cf.Plugins))
	}
}

func TestConfigFilesFileTier(t *testing.T) {
	cf := &ConfigFiles{
		Config:   "/etc/ductile/config.yaml",
		Tokens:   "/etc/ductile/tokens.yaml",
		Webhooks: "/etc/ductile/webhooks.yaml",
		Scopes:   []string{"/etc/ductile/scopes/admin.json"},
	}

	if cf.FileTier(cf.Tokens) != TierHighSecurity {
		t.Error("tokens.yaml should be high security")
	}
	if cf.FileTier(cf.Webhooks) != TierHighSecurity {
		t.Error("webhooks.yaml should be high security")
	}
	if cf.FileTier(cf.Scopes[0]) != TierHighSecurity {
		t.Error("scopes/*.json should be high security")
	}
	if cf.FileTier(cf.Config) != TierOperational {
		t.Error("config.yaml should be operational")
	}
}

func TestConfigFilesAllFiles(t *testing.T) {
	cf := &ConfigFiles{
		Config:   "/etc/ductile/config.yaml",
		Tokens:   "/etc/ductile/tokens.yaml",
		Webhooks: "/etc/ductile/webhooks.yaml",
		Routes:   "/etc/ductile/routes.yaml",
		Scopes:   []string{"/etc/ductile/scopes/admin.json"},
	}

	all := cf.AllFiles()
	if len(all) != 5 {
		t.Errorf("AllFiles() returned %d files, want 5", len(all))
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write %s: %v", path, err)
	}
}
