package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDirMinimal(t *testing.T) {
	tmpDir := t.TempDir()

	// Minimal directory-mode config
	writeTestFile(t, filepath.Join(tmpDir, "config.yaml"), `
service:
  tick_interval: 60s
state:
  path: ./test.db
plugins_dir: ./plugins
`)
	os.MkdirAll(filepath.Join(tmpDir, "plugins"), 0755)
	writeTestFile(t, filepath.Join(tmpDir, "plugins", "echo.yaml"), `
plugins:
  echo:
    enabled: true
    schedule:
      every: 5m
`)

	// Generate checksums (no high-security files, so no manifest needed;
	// but let's generate one for completeness)
	files, err := DiscoverConfigFiles(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := GenerateChecksumsFromDiscovery(files, false); err != nil {
		t.Fatal(err)
	}

	cfg, warnings, err := LoadDir(tmpDir)
	if err != nil {
		t.Fatalf("LoadDir() failed: %v", err)
	}

	if len(warnings) > 0 {
		t.Logf("warnings: %v", warnings)
	}

	if _, ok := cfg.Plugins["echo"]; !ok {
		t.Error("echo plugin not loaded from plugins/")
	}
	if cfg.ConfigDir != tmpDir {
		t.Errorf("ConfigDir = %q, want %q", cfg.ConfigDir, tmpDir)
	}
}

func TestLoadDirWithAllFiles(t *testing.T) {
	tmpDir := t.TempDir()

	writeTestFile(t, filepath.Join(tmpDir, "config.yaml"), `
service:
  tick_interval: 60s
  name: test-gw
state:
  path: ./test.db
plugins_dir: ./plugins
`)

	os.MkdirAll(filepath.Join(tmpDir, "plugins"), 0755)
	writeTestFile(t, filepath.Join(tmpDir, "plugins", "echo.yaml"), `
plugins:
  echo:
    enabled: true
    schedule:
      every: 5m
`)

	os.MkdirAll(filepath.Join(tmpDir, "pipelines"), 0755)
	writeTestFile(t, filepath.Join(tmpDir, "pipelines", "wisdom.yaml"), `
pipelines:
  - name: video-wisdom
    on: discord.link
`)

	writeTestFile(t, filepath.Join(tmpDir, "routes.yaml"), `
routes:
  - from: echo
    event_type: echo.output
    to: echo
`)

	writeTestFile(t, filepath.Join(tmpDir, "tokens.yaml"), `
tokens:
  - name: admin
    key: secret123
`)

	writeTestFile(t, filepath.Join(tmpDir, "webhooks.yaml"), `
webhooks:
  - name: github
    path: /webhook/github
    plugin: echo
    secret: gh-secret
    signature_header: X-Hub-Signature-256
`)

	// Generate checksums
	files, err := DiscoverConfigFiles(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := GenerateChecksumsFromDiscovery(files, false); err != nil {
		t.Fatal(err)
	}

	cfg, _, err := LoadDir(tmpDir)
	if err != nil {
		t.Fatalf("LoadDir() failed: %v", err)
	}

	// Verify plugins grafted
	if _, ok := cfg.Plugins["echo"]; !ok {
		t.Error("echo plugin not loaded")
	}

	// Verify pipelines grafted
	if len(cfg.Pipelines) != 1 {
		t.Fatalf("len(Pipelines) = %d, want 1", len(cfg.Pipelines))
	}
	if cfg.Pipelines[0].Name != "video-wisdom" {
		t.Errorf("Pipeline name = %q, want %q", cfg.Pipelines[0].Name, "video-wisdom")
	}

	// Verify routes grafted
	if len(cfg.Routes) != 1 {
		t.Fatalf("len(Routes) = %d, want 1", len(cfg.Routes))
	}

	// Verify tokens loaded
	if len(cfg.Tokens) != 1 {
		t.Fatalf("len(Tokens) = %d, want 1", len(cfg.Tokens))
	}
	if cfg.Tokens[0].Name != "admin" {
		t.Errorf("Token name = %q, want %q", cfg.Tokens[0].Name, "admin")
	}

	// Verify webhooks loaded
	if cfg.Webhooks == nil || len(cfg.Webhooks.Endpoints) != 1 {
		t.Fatal("webhooks not loaded")
	}
	if cfg.Webhooks.Endpoints[0].Name != "github" {
		t.Errorf("Webhook name = %q, want %q", cfg.Webhooks.Endpoints[0].Name, "github")
	}
}

func TestLoadDirMultiplePlugins(t *testing.T) {
	tmpDir := t.TempDir()

	writeTestFile(t, filepath.Join(tmpDir, "config.yaml"), `
service:
  tick_interval: 60s
state:
  path: ./test.db
plugins_dir: ./plugins
`)

	os.MkdirAll(filepath.Join(tmpDir, "plugins"), 0755)
	writeTestFile(t, filepath.Join(tmpDir, "plugins", "aaa.yaml"), `
plugins:
  aaa:
    enabled: true
    schedule:
      every: 5m
    config:
      key: from-aaa
`)
	writeTestFile(t, filepath.Join(tmpDir, "plugins", "bbb.yaml"), `
plugins:
  bbb:
    enabled: true
    schedule:
      every: hourly
`)
	// zzz.yaml overrides aaa's config key (later alphabetically wins)
	writeTestFile(t, filepath.Join(tmpDir, "plugins", "zzz.yaml"), `
plugins:
  aaa:
    enabled: true
    schedule:
      every: daily
    config:
      key: from-zzz
`)

	files, err := DiscoverConfigFiles(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := GenerateChecksumsFromDiscovery(files, false); err != nil {
		t.Fatal(err)
	}

	cfg, _, err := LoadDir(tmpDir)
	if err != nil {
		t.Fatalf("LoadDir() failed: %v", err)
	}

	// aaa should be overridden by zzz.yaml
	aaa := cfg.Plugins["aaa"]
	if aaa.Schedule.Every != "daily" {
		t.Errorf("aaa schedule = %q, want %q (zzz.yaml should override)", aaa.Schedule.Every, "daily")
	}
	if aaa.Config["key"] != "from-zzz" {
		t.Errorf("aaa config.key = %q, want %q", aaa.Config["key"], "from-zzz")
	}

	// bbb should exist
	if _, ok := cfg.Plugins["bbb"]; !ok {
		t.Error("bbb plugin not loaded")
	}
}

func TestLoadDirIntegrityFailure(t *testing.T) {
	tmpDir := t.TempDir()

	writeTestFile(t, filepath.Join(tmpDir, "config.yaml"), `
service:
  tick_interval: 60s
state:
  path: ./test.db
plugins_dir: ./plugins
`)
	writeTestFile(t, filepath.Join(tmpDir, "tokens.yaml"), `
tokens:
  - name: admin
    key: original
`)

	// Generate checksums
	files, err := DiscoverConfigFiles(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := GenerateChecksumsFromDiscovery(files, false); err != nil {
		t.Fatal(err)
	}

	// Tamper with tokens.yaml
	writeTestFile(t, filepath.Join(tmpDir, "tokens.yaml"), `
tokens:
  - name: hacked
    key: evil
`)

	_, _, err = LoadDir(tmpDir)
	if err == nil {
		t.Fatal("LoadDir() should fail when high-security file is tampered")
	}
}

func TestLoadDirNoChecksumNoHighSecurity(t *testing.T) {
	tmpDir := t.TempDir()

	writeTestFile(t, filepath.Join(tmpDir, "config.yaml"), `
service:
  tick_interval: 60s
state:
  path: ./test.db
plugins_dir: ./plugins
`)
	os.MkdirAll(filepath.Join(tmpDir, "plugins"), 0755)
	writeTestFile(t, filepath.Join(tmpDir, "plugins", "echo.yaml"), `
plugins:
  echo:
    enabled: true
    schedule:
      every: 5m
`)

	// No .checksums file, no high-security files — should work with warnings
	cfg, warnings, err := LoadDir(tmpDir)
	if err != nil {
		t.Fatalf("LoadDir() should succeed without checksums when no high-security files: %v", err)
	}
	if len(warnings) == 0 {
		t.Error("expected warning about missing manifest")
	}
	if _, ok := cfg.Plugins["echo"]; !ok {
		t.Error("echo plugin should still be loaded")
	}
}
