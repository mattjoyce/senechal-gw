package plugin

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscover(t *testing.T) {
	tests := []struct {
		name      string
		setupFn   func(t *testing.T) string // Returns plugins directory
		wantCount int
		wantErr   bool
		checkFn   func(t *testing.T, reg *Registry)
	}{
		{
			name: "valid plugin discovered",
			setupFn: func(t *testing.T) string {
				dir := t.TempDir()
				pluginDir := filepath.Join(dir, "test-plugin")
				os.Mkdir(pluginDir, 0755)

				manifest := `name: test-plugin
version: 1.0.0
protocol: 2
entrypoint: run.sh
commands: [poll, health]
`
				os.WriteFile(filepath.Join(pluginDir, "manifest.yaml"), []byte(manifest), 0644)

				// Create executable entrypoint
				entrypoint := filepath.Join(pluginDir, "run.sh")
				os.WriteFile(entrypoint, []byte("#!/bin/sh\necho ok"), 0755)

				return dir
			},
			wantCount: 1,
			wantErr:   false,
			checkFn: func(t *testing.T, reg *Registry) {
				plugin, ok := reg.Get("test-plugin")
				if !ok {
					t.Fatal("test-plugin not found")
				}
				if plugin.Protocol != 2 {
					t.Error("protocol version mismatch")
				}
				if !plugin.SupportsCommand("poll") {
					t.Error("should support poll command")
				}
			},
		},
		{
			name: "multiple valid plugins",
			setupFn: func(t *testing.T) string {
				dir := t.TempDir()

				for _, name := range []string{"plugin1", "plugin2"} {
					pluginDir := filepath.Join(dir, name)
					os.Mkdir(pluginDir, 0755)

					manifest := `name: ` + name + `
version: 1.0.0
protocol: 2
entrypoint: run.sh
commands: [poll]
`
					os.WriteFile(filepath.Join(pluginDir, "manifest.yaml"), []byte(manifest), 0644)
					entrypoint := filepath.Join(pluginDir, "run.sh")
					os.WriteFile(entrypoint, []byte("#!/bin/sh\n"), 0755)
				}

				return dir
			},
			wantCount: 2,
			wantErr:   false,
		},
		{
			name: "directory without manifest skipped",
			setupFn: func(t *testing.T) string {
				dir := t.TempDir()
				os.Mkdir(filepath.Join(dir, "no-manifest"), 0755)
				return dir
			},
			wantCount: 0,
			wantErr:   false,
		},
		{
			name: "unsupported protocol skipped",
			setupFn: func(t *testing.T) string {
				dir := t.TempDir()
				pluginDir := filepath.Join(dir, "bad-protocol")
				os.Mkdir(pluginDir, 0755)

				manifest := `name: bad-protocol
version: 1.0.0
protocol: 99
entrypoint: run.sh
commands: [poll]
`
				os.WriteFile(filepath.Join(pluginDir, "manifest.yaml"), []byte(manifest), 0644)
				return dir
			},
			wantCount: 0,
			wantErr:   false,
		},
		{
			name: "non-executable entrypoint skipped",
			setupFn: func(t *testing.T) string {
				dir := t.TempDir()
				pluginDir := filepath.Join(dir, "non-exec")
				os.Mkdir(pluginDir, 0755)

				manifest := `name: non-exec
version: 1.0.0
protocol: 2
entrypoint: run.sh
commands: [poll]
`
				os.WriteFile(filepath.Join(pluginDir, "manifest.yaml"), []byte(manifest), 0644)
				// Create non-executable file
				os.WriteFile(filepath.Join(pluginDir, "run.sh"), []byte("#!/bin/sh\n"), 0644)
				return dir
			},
			wantCount: 0,
			wantErr:   false,
		},
		{
			name: "nonexistent directory",
			setupFn: func(t *testing.T) string {
				return "/nonexistent/path"
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pluginsDir := tt.setupFn(t)

			logger := func(level, msg string, args ...any) {
				// Silent logger for tests
			}

			reg, err := Discover(pluginsDir, logger)

			if (err != nil) != tt.wantErr {
				t.Errorf("Discover() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if len(reg.All()) != tt.wantCount {
					t.Errorf("Discover() found %d plugins, want %d", len(reg.All()), tt.wantCount)
				}

				if tt.checkFn != nil {
					tt.checkFn(t, reg)
				}
			}
		})
	}
}

func TestDiscoverMany_MultipleRootsAndDuplicatePrecedence(t *testing.T) {
	rootA := t.TempDir()
	rootB := t.TempDir()

	makePlugin := func(root, dirName, manifestName string) {
		pDir := filepath.Join(root, dirName)
		if err := os.MkdirAll(pDir, 0o755); err != nil {
			t.Fatalf("mkdir plugin dir: %v", err)
		}
		manifest := `name: ` + manifestName + `
version: 1.0.0
protocol: 2
entrypoint: run.sh
commands: [poll]
`
		if err := os.WriteFile(filepath.Join(pDir, "manifest.yaml"), []byte(manifest), 0o644); err != nil {
			t.Fatalf("write manifest: %v", err)
		}
		if err := os.WriteFile(filepath.Join(pDir, "run.sh"), []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatalf("write entrypoint: %v", err)
		}
	}

	makePlugin(rootA, "echo", "echo")
	makePlugin(rootA, "from-a", "from-a")
	makePlugin(rootB, "echo-alt", "echo") // duplicate manifest name; should be ignored
	makePlugin(rootB, "from-b", "from-b")

	reg, err := DiscoverMany([]string{rootA, rootB}, nil)
	if err != nil {
		t.Fatalf("DiscoverMany() error = %v", err)
	}
	if len(reg.All()) != 3 {
		t.Fatalf("DiscoverMany() found %d plugins, want 3", len(reg.All()))
	}

	echo, ok := reg.Get("echo")
	if !ok {
		t.Fatalf("expected echo to be discovered")
	}
	wantEchoPath := filepath.Join(rootA, "echo")
	if echo.Path != wantEchoPath {
		t.Fatalf("expected first-root echo path %q, got %q", wantEchoPath, echo.Path)
	}
}

func TestDiscoverMany_RecursiveManifestScan(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "team", "plugin-x")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("mkdir nested plugin dir: %v", err)
	}
	manifest := `name: plugin-x
version: 1.0.0
protocol: 2
entrypoint: run.sh
commands: [poll]
`
	if err := os.WriteFile(filepath.Join(nested, "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(nested, "run.sh"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write entrypoint: %v", err)
	}

	reg, err := DiscoverMany([]string{root}, nil)
	if err != nil {
		t.Fatalf("DiscoverMany() error = %v", err)
	}
	if _, ok := reg.Get("plugin-x"); !ok {
		t.Fatalf("expected nested plugin to be discovered")
	}
}

func TestValidateManifest(t *testing.T) {
	tests := []struct {
		name     string
		manifest *Manifest
		wantErr  bool
	}{
		{
			name: "valid manifest",
			manifest: &Manifest{
				Name:       "test",
				Protocol:   2,
				Entrypoint: "run.sh",
				Commands:   Commands{{Name: "poll", Type: CommandTypeWrite}},
			},
			wantErr: false,
		},
		{
			name: "missing name",
			manifest: &Manifest{
				Protocol:   2,
				Entrypoint: "run.sh",
				Commands:   Commands{{Name: "poll", Type: CommandTypeWrite}},
			},
			wantErr: true,
		},
		{
			name: "missing protocol",
			manifest: &Manifest{
				Name:       "test",
				Entrypoint: "run.sh",
				Commands:   Commands{{Name: "poll", Type: CommandTypeWrite}},
			},
			wantErr: true,
		},
		{
			name: "missing entrypoint",
			manifest: &Manifest{
				Name:     "test",
				Protocol: 1,
				Commands: Commands{{Name: "poll", Type: CommandTypeWrite}},
			},
			wantErr: true,
		},
		{
			name: "missing commands",
			manifest: &Manifest{
				Name:       "test",
				Protocol:   2,
				Entrypoint: "run.sh",
			},
			wantErr: true,
		},
		{
			name: "path traversal in entrypoint",
			manifest: &Manifest{
				Name:       "test",
				Protocol:   2,
				Entrypoint: "../evil/run.sh",
				Commands:   Commands{{Name: "poll", Type: CommandTypeWrite}},
			},
			wantErr: true,
		},
		{
			name: "invalid command",
			manifest: &Manifest{
				Name:       "test",
				Protocol:   2,
				Entrypoint: "run.sh",
				Commands:   Commands{{Name: "invalid_command", Type: CommandTypeWrite}},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateManifest(tt.manifest)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateManifest() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateTrust(t *testing.T) {
	tests := []struct {
		name    string
		setupFn func(t *testing.T) (entrypoint, pluginPath, pluginsDir string)
		wantErr bool
	}{
		{
			name: "valid executable",
			setupFn: func(t *testing.T) (string, string, string) {
				dir := t.TempDir()
				pluginDir := filepath.Join(dir, "test")
				os.Mkdir(pluginDir, 0755)

				entrypoint := filepath.Join(pluginDir, "run.sh")
				os.WriteFile(entrypoint, []byte("#!/bin/sh\n"), 0755)

				return entrypoint, pluginDir, dir
			},
			wantErr: false,
		},
		{
			name: "non-executable",
			setupFn: func(t *testing.T) (string, string, string) {
				dir := t.TempDir()
				pluginDir := filepath.Join(dir, "test")
				os.Mkdir(pluginDir, 0755)

				entrypoint := filepath.Join(pluginDir, "run.sh")
				os.WriteFile(entrypoint, []byte("#!/bin/sh\n"), 0644) // Not executable

				return entrypoint, pluginDir, dir
			},
			wantErr: true,
		},
		{
			name: "world-writable plugin directory",
			setupFn: func(t *testing.T) (string, string, string) {
				// Note: This test may not work on all filesystems (e.g., macOS APFS temp dirs)
				// where permissions are restricted. Skip if chmod fails to set world-writable.
				dir := t.TempDir()
				pluginDir := filepath.Join(dir, "test")
				os.Mkdir(pluginDir, 0755)

				// Explicitly chmod to world-writable
				if err := os.Chmod(pluginDir, 0777); err != nil {
					t.Skip("cannot set world-writable on this filesystem")
				}

				// Verify it actually became world-writable
				info, _ := os.Stat(pluginDir)
				if info.Mode().Perm()&0002 == 0 {
					t.Skip("filesystem does not support world-writable directories")
				}

				entrypoint := filepath.Join(pluginDir, "run.sh")
				os.WriteFile(entrypoint, []byte("#!/bin/sh\n"), 0755)

				return entrypoint, pluginDir, dir
			},
			wantErr: true,
		},
		{
			name: "nonexistent entrypoint",
			setupFn: func(t *testing.T) (string, string, string) {
				dir := t.TempDir()
				pluginDir := filepath.Join(dir, "test")
				os.Mkdir(pluginDir, 0755)

				entrypoint := filepath.Join(pluginDir, "nonexistent.sh")

				return entrypoint, pluginDir, dir
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entrypoint, pluginPath, pluginsDir := tt.setupFn(t)

			err := validateTrust(entrypoint, pluginPath, pluginsDir)

			if (err != nil) != tt.wantErr {
				t.Errorf("validateTrust() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateTrustInRoots(t *testing.T) {
	rootA := t.TempDir()
	rootB := t.TempDir()
	pluginDir := filepath.Join(rootB, "test")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatalf("mkdir plugin dir: %v", err)
	}
	entrypoint := filepath.Join(pluginDir, "run.sh")
	if err := os.WriteFile(entrypoint, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write entrypoint: %v", err)
	}

	if err := validateTrustInRoots(entrypoint, pluginDir, []string{rootA, rootB}); err != nil {
		t.Fatalf("validateTrustInRoots() error = %v, want nil", err)
	}

	if err := validateTrustInRoots(entrypoint, pluginDir, []string{rootA}); err == nil {
		t.Fatal("validateTrustInRoots() expected error when rootB is not configured")
	}
}

func TestPluginSupportsCommand(t *testing.T) {
	plugin := &Plugin{
		Commands: Commands{
			{Name: "poll", Type: CommandTypeWrite},
			{Name: "health", Type: CommandTypeRead},
		},
	}

	if !plugin.SupportsCommand("poll") {
		t.Error("should support poll")
	}

	if !plugin.SupportsCommand("health") {
		t.Error("should support health")
	}

	if plugin.SupportsCommand("handle") {
		t.Error("should not support handle")
	}
}

func TestDiscover_TypedCommandMetadata(t *testing.T) {
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "test-plugin")
	os.Mkdir(pluginDir, 0755)

	manifest := `name: test-plugin
version: 1.0.0
protocol: 2
entrypoint: run.sh
commands:
  - name: poll
    type: write
  - name: health
    type: read
`
	os.WriteFile(filepath.Join(pluginDir, "manifest.yaml"), []byte(manifest), 0644)
	os.WriteFile(filepath.Join(pluginDir, "run.sh"), []byte("#!/bin/sh\necho ok"), 0755)

	logger := func(level, msg string, args ...any) {}
	reg, err := Discover(dir, logger)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	p, ok := reg.Get("test-plugin")
	if !ok {
		t.Fatalf("test-plugin not discovered")
	}
	if got := p.GetReadCommands(); len(got) != 1 || got[0] != "health" {
		t.Fatalf("GetReadCommands() = %v, want [health]", got)
	}
	if got := p.GetWriteCommands(); len(got) != 1 || got[0] != "poll" {
		t.Fatalf("GetWriteCommands() = %v, want [poll]", got)
	}
}
