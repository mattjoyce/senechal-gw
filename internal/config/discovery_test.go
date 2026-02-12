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

	// Create plugins directory with YAML files
	os.MkdirAll(filepath.Join(tmpDir, "plugins"), 0755)
	writeTestFile(t, filepath.Join(tmpDir, "plugins", "echo.yaml"), "plugins:\n  echo:\n    enabled: true\n")
	writeTestFile(t, filepath.Join(tmpDir, "plugins", "withings.yaml"), "plugins:\n  withings:\n    enabled: true\n")

	// Create pipelines directory
	os.MkdirAll(filepath.Join(tmpDir, "pipelines"), 0755)
	writeTestFile(t, filepath.Join(tmpDir, "pipelines", "wisdom.yaml"), "pipelines:\n  - name: wisdom\n")

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
	if len(cf.Plugins) != 2 {
		t.Fatalf("len(Plugins) = %d, want 2", len(cf.Plugins))
	}
	// Verify alphabetical order
	if filepath.Base(cf.Plugins[0]) != "echo.yaml" {
		t.Errorf("Plugins[0] = %q, want echo.yaml", filepath.Base(cf.Plugins[0]))
	}
	if filepath.Base(cf.Plugins[1]) != "withings.yaml" {
		t.Errorf("Plugins[1] = %q, want withings.yaml", filepath.Base(cf.Plugins[1]))
	}
	if len(cf.Pipelines) != 1 {
		t.Fatalf("len(Pipelines) = %d, want 1", len(cf.Pipelines))
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
		Config:   "/etc/senechal/config.yaml",
		Tokens:   "/etc/senechal/tokens.yaml",
		Webhooks: "/etc/senechal/webhooks.yaml",
		Plugins:  []string{"/etc/senechal/plugins/echo.yaml"},
		Scopes:   []string{"/etc/senechal/scopes/admin.json"},
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
	if cf.FileTier(cf.Plugins[0]) != TierOperational {
		t.Error("plugins/*.yaml should be operational")
	}
}

func TestConfigFilesAllFiles(t *testing.T) {
	cf := &ConfigFiles{
		Config:    "/etc/senechal/config.yaml",
		Tokens:    "/etc/senechal/tokens.yaml",
		Webhooks:  "/etc/senechal/webhooks.yaml",
		Routes:    "/etc/senechal/routes.yaml",
		Plugins:   []string{"/etc/senechal/plugins/echo.yaml"},
		Pipelines: []string{"/etc/senechal/pipelines/test.yaml"},
		Scopes:    []string{"/etc/senechal/scopes/admin.json"},
	}

	all := cf.AllFiles()
	if len(all) != 7 {
		t.Errorf("AllFiles() returned %d files, want 7", len(all))
	}
}

func TestIsConfigSpecDir(t *testing.T) {
	// Directory with config.yaml + plugins/ → true
	tmpDir := t.TempDir()
	writeTestFile(t, filepath.Join(tmpDir, "config.yaml"), "service:\n  name: test\n")
	os.MkdirAll(filepath.Join(tmpDir, "plugins"), 0755)

	if !IsConfigSpecDir(tmpDir) {
		t.Error("should detect directory with config.yaml + plugins/ as CONFIG_SPEC dir")
	}

	// Directory with config.yaml + pipelines/ → true
	tmpDir1b := t.TempDir()
	writeTestFile(t, filepath.Join(tmpDir1b, "config.yaml"), "service:\n  name: test\n")
	os.MkdirAll(filepath.Join(tmpDir1b, "pipelines"), 0755)

	if !IsConfigSpecDir(tmpDir1b) {
		t.Error("should detect directory with config.yaml + pipelines/ as CONFIG_SPEC dir")
	}

	// Directory with only config.yaml → false (could be include-mode)
	tmpDir2 := t.TempDir()
	writeTestFile(t, filepath.Join(tmpDir2, "config.yaml"), "service:\n  name: test\n")

	if IsConfigSpecDir(tmpDir2) {
		t.Error("bare config.yaml without subdirectory indicators should not be CONFIG_SPEC dir")
	}

	// Directory with config.yaml + tokens.yaml but no subdir → false (include-mode)
	tmpDir3 := t.TempDir()
	writeTestFile(t, filepath.Join(tmpDir3, "config.yaml"), "include:\n  - tokens.yaml\n")
	writeTestFile(t, filepath.Join(tmpDir3, "tokens.yaml"), "api_key: test\n")

	if IsConfigSpecDir(tmpDir3) {
		t.Error("config.yaml + tokens.yaml without subdirectory should not be CONFIG_SPEC dir")
	}

	// Empty directory → false
	tmpDir4 := t.TempDir()
	if IsConfigSpecDir(tmpDir4) {
		t.Error("empty directory should not be CONFIG_SPEC dir")
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write %s: %v", path, err)
	}
}
