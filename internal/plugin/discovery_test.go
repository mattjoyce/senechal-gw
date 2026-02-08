package plugin

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscover(t *testing.T) {
	tests := []struct {
		name       string
		setupFn    func(t *testing.T) string // Returns plugins directory
		wantCount  int
		wantErr    bool
		checkFn    func(t *testing.T, reg *Registry)
	}{
		{
			name: "valid plugin discovered",
			setupFn: func(t *testing.T) string {
				dir := t.TempDir()
				pluginDir := filepath.Join(dir, "test-plugin")
				os.Mkdir(pluginDir, 0755)

				manifest := `name: test-plugin
version: 1.0.0
protocol: 1
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
				if plugin.Protocol != 1 {
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
protocol: 1
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
protocol: 1
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

			logger := func(level, msg string, args ...interface{}) {
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
				Protocol:   1,
				Entrypoint: "run.sh",
				Commands:   []string{"poll"},
			},
			wantErr: false,
		},
		{
			name: "missing name",
			manifest: &Manifest{
				Protocol:   1,
				Entrypoint: "run.sh",
				Commands:   []string{"poll"},
			},
			wantErr: true,
		},
		{
			name: "missing protocol",
			manifest: &Manifest{
				Name:       "test",
				Entrypoint: "run.sh",
				Commands:   []string{"poll"},
			},
			wantErr: true,
		},
		{
			name: "missing entrypoint",
			manifest: &Manifest{
				Name:     "test",
				Protocol: 1,
				Commands: []string{"poll"},
			},
			wantErr: true,
		},
		{
			name: "missing commands",
			manifest: &Manifest{
				Name:       "test",
				Protocol:   1,
				Entrypoint: "run.sh",
			},
			wantErr: true,
		},
		{
			name: "path traversal in entrypoint",
			manifest: &Manifest{
				Name:       "test",
				Protocol:   1,
				Entrypoint: "../evil/run.sh",
				Commands:   []string{"poll"},
			},
			wantErr: true,
		},
		{
			name: "invalid command",
			manifest: &Manifest{
				Name:       "test",
				Protocol:   1,
				Entrypoint: "run.sh",
				Commands:   []string{"invalid_command"},
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

func TestPluginSupportsCommand(t *testing.T) {
	plugin := &Plugin{
		Commands: []string{"poll", "health"},
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
