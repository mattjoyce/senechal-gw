package dispatch

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mattjoyce/senechal-gw/internal/config"
	"github.com/mattjoyce/senechal-gw/internal/log"
	"github.com/mattjoyce/senechal-gw/internal/plugin"
	"github.com/mattjoyce/senechal-gw/internal/protocol"
	"github.com/mattjoyce/senechal-gw/internal/queue"
	"github.com/mattjoyce/senechal-gw/internal/state"
	"github.com/mattjoyce/senechal-gw/internal/storage"
)

func TestMain(m *testing.M) {
	log.Setup("ERROR") // Suppress logs in tests
	os.Exit(m.Run())
}

func setupTestDispatcher(t *testing.T) (*Dispatcher, *sql.DB, string, func()) {
	t.Helper()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	pluginsDir := filepath.Join(tmpDir, "plugins")

	db, err := storage.OpenSQLite(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}

	q := queue.New(db)
	st := state.NewStore(db)

	// Create test plugin directory
	if err := os.MkdirAll(pluginsDir, 0755); err != nil {
		t.Fatalf("failed to create plugins dir: %v", err)
	}

	registry := plugin.NewRegistry()

	cfg := config.Defaults()
	cfg.PluginsDir = pluginsDir

	disp := New(q, st, registry, cfg)

	cleanup := func() {
		db.Close()
		os.RemoveAll(tmpDir)
	}

	return disp, db, pluginsDir, cleanup
}

func createTestPlugin(t *testing.T, pluginsDir, name, script string) *plugin.Plugin {
	t.Helper()

	pluginDir := filepath.Join(pluginsDir, name)
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		t.Fatalf("failed to create plugin dir: %v", err)
	}

	// Write manifest
	manifest := fmt.Sprintf(`name: %s
version: 1.0.0
protocol: 1
entrypoint: run.sh
commands: [poll, handle, health]
`, name)

	manifestPath := filepath.Join(pluginDir, "manifest.yaml")
	if err := os.WriteFile(manifestPath, []byte(manifest), 0644); err != nil {
		t.Fatalf("failed to write manifest: %v", err)
	}

	// Write executable script
	scriptPath := filepath.Join(pluginDir, "run.sh")
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		t.Fatalf("failed to write script: %v", err)
	}

	// Load and validate plugin
	reg, err := plugin.Discover(pluginsDir, func(level, msg string, args ...interface{}) {})
	if err != nil {
		t.Fatalf("failed to discover plugins: %v", err)
	}

	plug, ok := reg.Get(name)
	if !ok {
		t.Fatalf("plugin %q not found after discovery", name)
	}

	return plug
}

func TestDispatcher_ExecuteJob_Success(t *testing.T) {
	disp, db, pluginsDir, cleanup := setupTestDispatcher(t)
	defer cleanup()

	// Create a simple echo plugin that returns success
	script := `#!/bin/bash
read input
echo '{"status": "ok", "state_updates": {"last_run": "2024-01-01T00:00:00Z"}}'
`
	plug := createTestPlugin(t, pluginsDir, "echo", script)
	disp.registry.Add(plug)

	// Configure plugin
	disp.cfg.Plugins["echo"] = config.PluginConf{
		Enabled: true,
		Config:  map[string]interface{}{"test": "value"},
		Timeouts: &config.TimeoutsConfig{
			Poll: 5 * time.Second,
		},
	}

	ctx := context.Background()

	// Enqueue a job
	jobID, err := disp.queue.Enqueue(ctx, queue.EnqueueRequest{
		Plugin:      "echo",
		Command:     "poll",
		SubmittedBy: "test",
	})
	if err != nil {
		t.Fatalf("failed to enqueue job: %v", err)
	}

	// Dequeue and execute
	job, err := disp.queue.Dequeue(ctx)
	if err != nil {
		t.Fatalf("failed to dequeue job: %v", err)
	}
	if job == nil {
		t.Fatal("expected job, got nil")
	}

	disp.executeJob(ctx, job)

	// Verify job completed
	var status string
	err = db.QueryRow("SELECT status FROM job_queue WHERE id = ?", jobID).Scan(&status)
	if err != nil {
		t.Fatalf("failed to query job status: %v", err)
	}

	if status != "succeeded" {
		t.Errorf("expected status=succeeded, got %s", status)
	}

	// Verify state was updated
	pluginState, err := disp.state.Get(ctx, "echo")
	if err != nil {
		t.Fatalf("failed to get plugin state: %v", err)
	}

	var stateMap map[string]interface{}
	if err := json.Unmarshal(pluginState, &stateMap); err != nil {
		t.Fatalf("failed to unmarshal state: %v", err)
	}

	if lastRun, ok := stateMap["last_run"]; !ok || lastRun != "2024-01-01T00:00:00Z" {
		t.Errorf("expected last_run in state, got %v", stateMap)
	}
}

func TestDispatcher_ExecuteJob_PluginError(t *testing.T) {
	disp, db, pluginsDir, cleanup := setupTestDispatcher(t)
	defer cleanup()

	// Plugin that returns error status
	script := `#!/bin/bash
read input
echo '{"status": "error", "error": "something went wrong"}'
`
	plug := createTestPlugin(t, pluginsDir, "failing", script)
	disp.registry.Add(plug)

	disp.cfg.Plugins["failing"] = config.PluginConf{
		Enabled: true,
		Timeouts: &config.TimeoutsConfig{
			Poll: 5 * time.Second,
		},
	}

	ctx := context.Background()

	jobID, err := disp.queue.Enqueue(ctx, queue.EnqueueRequest{
		Plugin:      "failing",
		Command:     "poll",
		SubmittedBy: "test",
	})
	if err != nil {
		t.Fatalf("failed to enqueue job: %v", err)
	}

	job, err := disp.queue.Dequeue(ctx)
	if err != nil {
		t.Fatalf("failed to dequeue job: %v", err)
	}

	disp.executeJob(ctx, job)

	// Verify job failed
	var status, lastError string
	err = db.QueryRow("SELECT status, last_error FROM job_queue WHERE id = ?", jobID).Scan(&status, &lastError)
	if err != nil {
		t.Fatalf("failed to query job: %v", err)
	}

	if status != "failed" {
		t.Errorf("expected status=failed, got %s", status)
	}

	if lastError != "something went wrong" {
		t.Errorf("expected last_error='something went wrong', got %s", lastError)
	}
}

func TestDispatcher_ExecuteJob_Timeout(t *testing.T) {
	disp, db, pluginsDir, cleanup := setupTestDispatcher(t)
	defer cleanup()

	// Plugin that sleeps longer than timeout
	// Use exec to replace bash with sleep so SIGTERM goes directly to sleep
	script := `#!/bin/bash
read input
exec sleep 10
`
	plug := createTestPlugin(t, pluginsDir, "sleeper", script)
	disp.registry.Add(plug)

	disp.cfg.Plugins["sleeper"] = config.PluginConf{
		Enabled: true,
		Timeouts: &config.TimeoutsConfig{
			Poll: 1 * time.Second, // Very short timeout
		},
	}

	ctx := context.Background()

	jobID, err := disp.queue.Enqueue(ctx, queue.EnqueueRequest{
		Plugin:      "sleeper",
		Command:     "poll",
		SubmittedBy: "test",
	})
	if err != nil {
		t.Fatalf("failed to enqueue job: %v", err)
	}

	job, err := disp.queue.Dequeue(ctx)
	if err != nil {
		t.Fatalf("failed to dequeue job: %v", err)
	}

	start := time.Now()
	disp.executeJob(ctx, job)
	elapsed := time.Since(start)

	// Should timeout within reasonable time (1s timeout + 5s grace + some margin)
	if elapsed > 8*time.Second {
		t.Errorf("timeout took too long: %v", elapsed)
	}

	// Verify job timed out
	var status string
	err = db.QueryRow("SELECT status FROM job_queue WHERE id = ?", jobID).Scan(&status)
	if err != nil {
		t.Fatalf("failed to query job: %v", err)
	}

	if status != "timed_out" {
		t.Errorf("expected status=timed_out, got %s", status)
	}
}

func TestDispatcher_ExecuteJob_ProtocolError(t *testing.T) {
	disp, db, pluginsDir, cleanup := setupTestDispatcher(t)
	defer cleanup()

	// Plugin that outputs invalid JSON
	script := `#!/bin/bash
read input
echo 'not valid json'
`
	plug := createTestPlugin(t, pluginsDir, "broken", script)
	disp.registry.Add(plug)

	disp.cfg.Plugins["broken"] = config.PluginConf{
		Enabled: true,
		Timeouts: &config.TimeoutsConfig{
			Poll: 5 * time.Second,
		},
	}

	ctx := context.Background()

	jobID, err := disp.queue.Enqueue(ctx, queue.EnqueueRequest{
		Plugin:      "broken",
		Command:     "poll",
		SubmittedBy: "test",
	})
	if err != nil {
		t.Fatalf("failed to enqueue job: %v", err)
	}

	job, err := disp.queue.Dequeue(ctx)
	if err != nil {
		t.Fatalf("failed to dequeue job: %v", err)
	}

	disp.executeJob(ctx, job)

	// Verify job failed due to protocol error
	var status string
	err = db.QueryRow("SELECT status FROM job_queue WHERE id = ?", jobID).Scan(&status)
	if err != nil {
		t.Fatalf("failed to query job: %v", err)
	}

	if status != "failed" {
		t.Errorf("expected status=failed, got %s", status)
	}
}

func TestDispatcher_ExecuteJob_HandleCommand(t *testing.T) {
	disp, db, pluginsDir, cleanup := setupTestDispatcher(t)
	defer cleanup()

	// Plugin that echoes the event type it received
	script := `#!/bin/bash
read input
# Extract event type from JSON (basic bash parsing)
echo '{"status": "ok", "logs": [{"level": "info", "message": "handled event"}]}'
`
	plug := createTestPlugin(t, pluginsDir, "handler", script)
	disp.registry.Add(plug)

	disp.cfg.Plugins["handler"] = config.PluginConf{
		Enabled: true,
		Timeouts: &config.TimeoutsConfig{
			Handle: 5 * time.Second,
		},
	}

	ctx := context.Background()

	// Create an event payload
	event := protocol.Event{
		Type: "test.event",
		Payload: map[string]any{
			"key": "value",
		},
	}
	eventJSON, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("failed to marshal event: %v", err)
	}

	jobID, err := disp.queue.Enqueue(ctx, queue.EnqueueRequest{
		Plugin:      "handler",
		Command:     "handle",
		Payload:     eventJSON,
		SubmittedBy: "test",
	})
	if err != nil {
		t.Fatalf("failed to enqueue job: %v", err)
	}

	job, err := disp.queue.Dequeue(ctx)
	if err != nil {
		t.Fatalf("failed to dequeue job: %v", err)
	}

	disp.executeJob(ctx, job)

	// Verify job succeeded
	var status string
	err = db.QueryRow("SELECT status FROM job_queue WHERE id = ?", jobID).Scan(&status)
	if err != nil {
		t.Fatalf("failed to query job: %v", err)
	}

	if status != "succeeded" {
		t.Errorf("expected status=succeeded, got %s", status)
	}
}

func TestDispatcher_GetTimeout(t *testing.T) {
	disp, _, _, cleanup := setupTestDispatcher(t)
	defer cleanup()

	tests := []struct {
		name     string
		timeouts *config.TimeoutsConfig
		command  string
		want     time.Duration
	}{
		{
			name:     "poll with custom timeout",
			timeouts: &config.TimeoutsConfig{Poll: 90 * time.Second},
			command:  "poll",
			want:     90 * time.Second,
		},
		{
			name:     "poll with default",
			timeouts: nil,
			command:  "poll",
			want:     60 * time.Second,
		},
		{
			name:     "handle with custom timeout",
			timeouts: &config.TimeoutsConfig{Handle: 180 * time.Second},
			command:  "handle",
			want:     180 * time.Second,
		},
		{
			name:     "health with default",
			timeouts: nil,
			command:  "health",
			want:     10 * time.Second,
		},
		{
			name:     "unknown command defaults to 60s",
			timeouts: nil,
			command:  "unknown",
			want:     60 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := disp.getTimeout(tt.timeouts, tt.command)
			if got != tt.want {
				t.Errorf("getTimeout() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTruncateStderr(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int
	}{
		{
			name:  "short string unchanged",
			input: "short",
			want:  5,
		},
		{
			name:  "exactly at limit unchanged",
			input: string(make([]byte, maxStderrBytes)),
			want:  maxStderrBytes,
		},
		{
			name:  "over limit truncated",
			input: string(make([]byte, maxStderrBytes+1000)),
			want:  maxStderrBytes,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateStderr(tt.input)
			if len(got) != tt.want {
				t.Errorf("truncateStderr() length = %d, want %d", len(got), tt.want)
			}
		})
	}
}
