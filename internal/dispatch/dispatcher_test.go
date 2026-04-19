package dispatch

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mattjoyce/ductile/internal/config"
	"github.com/mattjoyce/ductile/internal/events"
	"github.com/mattjoyce/ductile/internal/log"
	"github.com/mattjoyce/ductile/internal/plugin"
	"github.com/mattjoyce/ductile/internal/protocol"
	"github.com/mattjoyce/ductile/internal/queue"
	"github.com/mattjoyce/ductile/internal/router"
	"github.com/mattjoyce/ductile/internal/state"
	"github.com/mattjoyce/ductile/internal/storage"
	"github.com/mattjoyce/ductile/internal/workspace"
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
	contextStore := state.NewContextStore(db)

	// Create test plugin directory
	if err := os.MkdirAll(pluginsDir, 0755); err != nil {
		t.Fatalf("failed to create plugins dir: %v", err)
	}

	registry := plugin.NewRegistry()
	hub := events.NewHub(128)

	cfg := config.Defaults()
	cfg.PluginRoots = []string{pluginsDir}

	disp := New(q, st, contextStore, nil, nil, registry, hub, cfg)

	cleanup := func() {
		if err := db.Close(); err != nil {
			t.Fatalf("db.Close(): %v", err)
		}
		if err := os.RemoveAll(tmpDir); err != nil {
			t.Fatalf("RemoveAll(%s): %v", tmpDir, err)
		}
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
	manifest := fmt.Sprintf(`manifest_spec: ductile.plugin
manifest_version: 1
name: %s
version: 1.0.0
protocol: 2
entrypoint: run.sh
commands:
  - name: poll
    type: write
  - name: handle
    type: write
  - name: health
    type: read
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
	reg, err := plugin.DiscoverManyWithOptions([]string{pluginsDir}, func(level, msg string, args ...any) {}, plugin.DiscoverOptions{AllowSymlinks: true})
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
echo '{"status": "ok", "result": "ok", "state_updates": {"last_run": "2024-01-01T00:00:00Z"}}'
`
	plug := createTestPlugin(t, pluginsDir, "echo", script)
	if err := disp.registry.Add(plug); err != nil {
		t.Fatalf("registry.Add(echo): %v", err)
	}

	// Configure plugin
	disp.cfg.Plugins["echo"] = config.PluginConf{
		Enabled: true,
		Config:  map[string]any{"test": "value"},
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

	var stateMap map[string]any
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
	if err := disp.registry.Add(plug); err != nil {
		t.Fatalf("registry.Add(failing): %v", err)
	}

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
		MaxAttempts: 1,
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
	if err := disp.registry.Add(plug); err != nil {
		t.Fatalf("registry.Add(sleeper): %v", err)
	}

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
		MaxAttempts: 1,
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
	if err := disp.registry.Add(plug); err != nil {
		t.Fatalf("registry.Add(broken): %v", err)
	}

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
		MaxAttempts: 1,
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

func TestDispatcher_ExecuteJob_WithTemplateErrorFailsJob(t *testing.T) {
	disp, db, pluginsDir, cleanup := setupTestDispatcher(t)
	defer cleanup()

	script := `#!/bin/bash
read input
echo '{"status":"ok","result":"ok"}'
`
	plug := createTestPlugin(t, pluginsDir, "echo", script)
	if err := disp.registry.Add(plug); err != nil {
		t.Fatalf("registry.Add(echo): %v", err)
	}

	set, err := router.LoadFromConfigFiles([]string{writePipelineFile(t, t.TempDir(), `pipelines:
  - name: with-pipeline
    on: event.start
    steps:
      - id: notify
        uses: echo
        with:
          message: "{payload.missing}"
`)}, disp.registry, nil)
	if err != nil {
		t.Fatalf("LoadFromConfigFiles: %v", err)
	}
	disp.router = set

	disp.cfg.Plugins["echo"] = config.PluginConf{
		Enabled:  true,
		Timeouts: &config.TimeoutsConfig{Handle: 5 * time.Second},
	}

	ctx := context.Background()
	eventCtx, err := disp.contexts.Create(ctx, nil, "with-pipeline", "notify", []byte(`{"origin_channel_id":"chan-1"}`))
	if err != nil {
		t.Fatalf("ContextStore.Create(): %v", err)
	}

	payload, err := json.Marshal(protocol.Event{
		Type:    "event.start",
		Payload: map[string]any{"message": "hello"},
	})
	if err != nil {
		t.Fatalf("json.Marshal(event): %v", err)
	}

	jobID, err := disp.queue.Enqueue(ctx, queue.EnqueueRequest{
		Plugin:         "echo",
		Command:        "handle",
		Payload:        payload,
		EventContextID: &eventCtx.ID,
		SubmittedBy:    "test",
	})
	if err != nil {
		t.Fatalf("Enqueue(): %v", err)
	}

	job, err := disp.queue.Dequeue(ctx)
	if err != nil {
		t.Fatalf("Dequeue(): %v", err)
	}
	if job == nil {
		t.Fatal("expected queued job")
	}

	disp.executeJob(ctx, job)

	var status, lastError string
	if err := db.QueryRow("SELECT status, last_error FROM job_queue WHERE id = ?", jobID).Scan(&status, &lastError); err != nil {
		t.Fatalf("query job status: %v", err)
	}
	if status != "failed" {
		t.Fatalf("status = %q, want %q", status, "failed")
	}
	if !strings.Contains(lastError, `resolve "payload.missing": path not found`) {
		t.Fatalf("last_error = %q, want path resolution failure", lastError)
	}
}

func writePipelineFile(t *testing.T, dir, body string) string {
	t.Helper()

	path := filepath.Join(dir, "pipelines.yaml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
	return path
}

func TestCalculateBackoffDelay(t *testing.T) {
	base := 2 * time.Second

	delay1 := calculateBackoffDelay(base, 1, 0)
	if delay1 != 2*time.Second {
		t.Fatalf("attempt 1 delay = %v, want %v", delay1, 2*time.Second)
	}

	delay2 := calculateBackoffDelay(base, 2, 500*time.Millisecond)
	if delay2 != 4500*time.Millisecond {
		t.Fatalf("attempt 2 delay = %v, want %v", delay2, 4500*time.Millisecond)
	}

	delay3 := calculateBackoffDelay(base, 3, 0)
	if delay3 != 8*time.Second {
		t.Fatalf("attempt 3 delay = %v, want %v", delay3, 8*time.Second)
	}
}

func TestDispatcher_ExecuteJob_RetryScheduledOnPluginError(t *testing.T) {
	disp, db, pluginsDir, cleanup := setupTestDispatcher(t)
	defer cleanup()

	script := `#!/bin/bash
read input
echo '{"status":"error","error":"transient failure"}'
`
	plug := createTestPlugin(t, pluginsDir, "retrying", script)
	if err := disp.registry.Add(plug); err != nil {
		t.Fatalf("registry.Add(%s): %v", plug.Name, err)
	}
	disp.cfg.Plugins["retrying"] = config.PluginConf{
		Enabled: true,
		Retry: &config.RetryConfig{
			MaxAttempts: 3,
			BackoffBase: 1 * time.Second,
		},
		Timeouts: &config.TimeoutsConfig{
			Poll: 5 * time.Second,
		},
	}

	ctx := context.Background()
	jobID, err := disp.queue.Enqueue(ctx, queue.EnqueueRequest{
		Plugin:      "retrying",
		Command:     "poll",
		SubmittedBy: "test",
		MaxAttempts: 3,
	})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	job, err := disp.queue.Dequeue(ctx)
	if err != nil {
		t.Fatalf("dequeue: %v", err)
	}
	disp.executeJob(ctx, job)

	var (
		status      string
		attempt     int
		nextRetryAt sql.NullString
	)
	if err := db.QueryRow(`SELECT status, attempt, next_retry_at FROM job_queue WHERE id = ?`, jobID).Scan(&status, &attempt, &nextRetryAt); err != nil {
		t.Fatalf("query retry state: %v", err)
	}
	if status != string(queue.StatusQueued) {
		t.Fatalf("status = %s, want queued", status)
	}
	if attempt != 2 {
		t.Fatalf("attempt = %d, want 2", attempt)
	}
	if !nextRetryAt.Valid || nextRetryAt.String == "" {
		t.Fatalf("next_retry_at should be set")
	}

	retryEvents := filterEventsByType(disp.events.SnapshotSince(0), "job.retry_scheduled")
	if len(retryEvents) == 0 {
		t.Fatalf("expected job.retry_scheduled event")
	}
}

func TestDispatcher_ExecuteJob_NonRetryableResponse(t *testing.T) {
	disp, db, pluginsDir, cleanup := setupTestDispatcher(t)
	defer cleanup()

	script := `#!/bin/bash
read input
echo '{"status":"error","error":"bad request","retry":false}'
`
	plug := createTestPlugin(t, pluginsDir, "noretry", script)
	if err := disp.registry.Add(plug); err != nil {
		t.Fatalf("registry.Add(%s): %v", plug.Name, err)
	}
	disp.cfg.Plugins["noretry"] = config.PluginConf{
		Enabled: true,
		Retry: &config.RetryConfig{
			MaxAttempts: 3,
			BackoffBase: 1 * time.Second,
		},
		Timeouts: &config.TimeoutsConfig{
			Poll: 5 * time.Second,
		},
	}

	ctx := context.Background()
	jobID, err := disp.queue.Enqueue(ctx, queue.EnqueueRequest{
		Plugin:      "noretry",
		Command:     "poll",
		SubmittedBy: "test",
		MaxAttempts: 3,
	})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	job, err := disp.queue.Dequeue(ctx)
	if err != nil {
		t.Fatalf("dequeue: %v", err)
	}
	disp.executeJob(ctx, job)

	var status string
	if err := db.QueryRow(`SELECT status FROM job_queue WHERE id = ?`, jobID).Scan(&status); err != nil {
		t.Fatalf("query status: %v", err)
	}
	if status != string(queue.StatusFailed) {
		t.Fatalf("status = %s, want failed", status)
	}
	retryScheduled := filterEventsByType(disp.events.SnapshotSince(0), "job.retry_scheduled")
	if len(retryScheduled) != 0 {
		t.Fatalf("expected no retry_scheduled events, got %d", len(retryScheduled))
	}
	retryExhausted := filterEventsByType(disp.events.SnapshotSince(0), "job.retry_exhausted")
	if len(retryExhausted) == 0 {
		t.Fatalf("expected retry_exhausted event for non-retryable failure")
	}
}

func TestDispatcher_ExecuteJob_NonRetryableExitCode78(t *testing.T) {
	disp, db, pluginsDir, cleanup := setupTestDispatcher(t)
	defer cleanup()

	script := `#!/bin/bash
read input
echo '{"status":"error","error":"config invalid"}'
exit 78
`
	plug := createTestPlugin(t, pluginsDir, "exit78", script)
	if err := disp.registry.Add(plug); err != nil {
		t.Fatalf("registry.Add(%s): %v", plug.Name, err)
	}
	disp.cfg.Plugins["exit78"] = config.PluginConf{
		Enabled: true,
		Retry: &config.RetryConfig{
			MaxAttempts: 3,
			BackoffBase: 1 * time.Second,
		},
		Timeouts: &config.TimeoutsConfig{
			Poll: 5 * time.Second,
		},
	}

	ctx := context.Background()
	jobID, err := disp.queue.Enqueue(ctx, queue.EnqueueRequest{
		Plugin:      "exit78",
		Command:     "poll",
		SubmittedBy: "test",
		MaxAttempts: 3,
	})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	job, err := disp.queue.Dequeue(ctx)
	if err != nil {
		t.Fatalf("dequeue: %v", err)
	}
	disp.executeJob(ctx, job)

	var status string
	if err := db.QueryRow(`SELECT status FROM job_queue WHERE id = ?`, jobID).Scan(&status); err != nil {
		t.Fatalf("query status: %v", err)
	}
	if status != string(queue.StatusFailed) {
		t.Fatalf("status = %s, want failed", status)
	}
	retryScheduled := filterEventsByType(disp.events.SnapshotSince(0), "job.retry_scheduled")
	if len(retryScheduled) != 0 {
		t.Fatalf("expected no retry_scheduled events, got %d", len(retryScheduled))
	}
}

func TestDispatcher_ExecuteJob_HandleCommand(t *testing.T) {
	disp, db, pluginsDir, cleanup := setupTestDispatcher(t)
	defer cleanup()

	// Plugin that echoes the event type it received
	script := `#!/bin/bash
read input
# Extract event type from JSON (basic bash parsing)
echo '{"status": "ok", "result": "handled event", "logs": [{"level": "info", "message": "handled event"}]}'
`
	plug := createTestPlugin(t, pluginsDir, "handler", script)
	if err := disp.registry.Add(plug); err != nil {
		t.Fatalf("registry.Add(%s): %v", plug.Name, err)
	}

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

func TestDispatcher_RoutesTwoHopChainWithContextAndWorkspace(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "state.db")
	pluginsDir := filepath.Join(tmpDir, "plugins")
	if err := os.MkdirAll(pluginsDir, 0755); err != nil {
		t.Fatalf("MkdirAll(pluginsDir): %v", err)
	}

	db, err := storage.OpenSQLite(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Fatalf("db.Close(): %v", err)
		}
	}()

	q := queue.New(db)
	st := state.NewStore(db)
	contextStore := state.NewContextStore(db)

	scriptA := `#!/bin/bash
payload="$(cat)"
workspace_dir=$(echo "$payload" | sed -n 's/.*"workspace_dir":"\([^"]*\)".*/\1/p')
if [ -n "$workspace_dir" ]; then
  mkdir -p "$workspace_dir"
  echo "artifact-from-a" > "$workspace_dir/artifact.txt"
fi
echo '{"status":"ok","result":"chain start","events":[{"type":"chain.start","dedupe_key":"chain:start:hello","payload":{"origin_channel_id":"chan-1","message":"hello"}}]}'
`
	scriptB := `#!/bin/bash
read input
echo '{"status":"ok","result":"handled by b","logs":[{"level":"info","message":"handled by b"}]}'
`

	registry := plugin.NewRegistry()
	plugA := createTestPlugin(t, pluginsDir, "plugin-a", scriptA)
	plugB := createTestPlugin(t, pluginsDir, "plugin-b", scriptB)
	if err := registry.Add(plugA); err != nil {
		t.Fatalf("registry.Add(plugin-a): %v", err)
	}
	if err := registry.Add(plugB); err != nil {
		t.Fatalf("registry.Add(plugin-b): %v", err)
	}

	pipelineYAML := `pipelines:
  - name: chain
    on: chain.start
    steps:
      - id: step_b
        uses: plugin-b
`
	pipelinePath := filepath.Join(tmpDir, "pipelines.yaml")
	if err := os.WriteFile(pipelinePath, []byte(pipelineYAML), 0644); err != nil {
		t.Fatalf("WriteFile(pipelines.yaml): %v", err)
	}

	routerEngine, err := router.LoadFromConfigFiles([]string{pipelinePath}, registry, nil)
	if err != nil {
		t.Fatalf("LoadFromConfigFiles: %v", err)
	}
	workspaceBaseDir := filepath.Join(tmpDir, "workspaces")
	wsManager, err := workspace.NewFSManager(workspaceBaseDir)
	if err != nil {
		t.Fatalf("NewFSManager: %v", err)
	}

	cfg := config.Defaults()
	cfg.PluginRoots = []string{pluginsDir}
	cfg.Plugins["plugin-a"] = config.PluginConf{
		Enabled: true,
		Timeouts: &config.TimeoutsConfig{
			Poll: 5 * time.Second,
		},
	}
	cfg.Plugins["plugin-b"] = config.PluginConf{
		Enabled: true,
		Timeouts: &config.TimeoutsConfig{
			Handle: 5 * time.Second,
		},
	}

	hub := events.NewHub(128)
	disp := New(q, st, contextStore, wsManager, routerEngine, registry, hub, cfg)
	ctx := context.Background()

	rootJobID, err := q.Enqueue(ctx, queue.EnqueueRequest{
		Plugin:      "plugin-a",
		Command:     "poll",
		SubmittedBy: "test",
	})
	if err != nil {
		t.Fatalf("Enqueue(root): %v", err)
	}

	rootJob, err := q.Dequeue(ctx)
	if err != nil {
		t.Fatalf("Dequeue(root): %v", err)
	}
	if rootJob == nil {
		t.Fatalf("expected root job")
	}
	disp.executeJob(ctx, rootJob)

	var rootStatus string
	if err := db.QueryRow("SELECT status FROM job_queue WHERE id = ?", rootJobID).Scan(&rootStatus); err != nil {
		t.Fatalf("query root status: %v", err)
	}
	if rootStatus != string(queue.StatusSucceeded) {
		t.Fatalf("root status = %s, want %s", rootStatus, queue.StatusSucceeded)
	}

	childJob, err := q.Dequeue(ctx)
	if err != nil {
		t.Fatalf("Dequeue(child): %v", err)
	}
	if childJob == nil {
		t.Fatalf("expected routed child job")
	}
	if childJob.Plugin != "plugin-b" || childJob.Command != "handle" {
		t.Fatalf("unexpected child job: %+v", childJob)
	}
	if childJob.ParentJobID == nil || *childJob.ParentJobID != rootJobID {
		t.Fatalf("child parent_job_id = %v, want %s", childJob.ParentJobID, rootJobID)
	}
	if childJob.EventContextID == nil {
		t.Fatalf("child event_context_id is nil")
	}
	if childJob.DedupeKey == nil || *childJob.DedupeKey != "chain:start:hello" {
		t.Fatalf("child dedupe_key = %v, want %q", childJob.DedupeKey, "chain:start:hello")
	}

	var routedEvent protocol.Event
	if err := json.Unmarshal(childJob.Payload, &routedEvent); err != nil {
		t.Fatalf("unmarshal child payload: %v", err)
	}
	if routedEvent.Type != "chain.start" {
		t.Fatalf("child event type = %q, want %q", routedEvent.Type, "chain.start")
	}
	if routedEvent.Payload["message"] != "hello" {
		t.Fatalf("child event payload missing message: %+v", routedEvent.Payload)
	}

	childContext, err := contextStore.Get(ctx, *childJob.EventContextID)
	if err != nil {
		t.Fatalf("ContextStore.Get(child): %v", err)
	}
	var accumulated map[string]any
	if err := json.Unmarshal(childContext.AccumulatedJSON, &accumulated); err != nil {
		t.Fatalf("unmarshal child context: %v", err)
	}
	if accumulated["origin_channel_id"] != "chan-1" {
		t.Fatalf("origin_channel_id = %#v, want %q", accumulated["origin_channel_id"], "chan-1")
	}

	childArtifact := filepath.Join(workspaceBaseDir, childJob.ID[:2], childJob.ID, "artifact.txt")
	b, err := os.ReadFile(childArtifact)
	if err != nil {
		t.Fatalf("read child artifact: %v", err)
	}
	if string(b) != "artifact-from-a\n" {
		t.Fatalf("child artifact content = %q, want %q", string(b), "artifact-from-a\n")
	}

	disp.executeJob(ctx, childJob)

	var childStatus string
	if err := db.QueryRow("SELECT status FROM job_queue WHERE id = ?", childJob.ID).Scan(&childStatus); err != nil {
		t.Fatalf("query child status: %v", err)
	}
	if childStatus != string(queue.StatusSucceeded) {
		t.Fatalf("child status = %s, want %s", childStatus, queue.StatusSucceeded)
	}
}

func TestDispatcherContextUpdatesForDispatchUsesExplicitBaggage(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "state.db")

	db, err := storage.OpenSQLite(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Fatalf("db.Close(): %v", err)
		}
	}()

	contextStore := state.NewContextStore(db)
	root, err := contextStore.Create(
		context.Background(),
		nil,
		"chain",
		"root",
		json.RawMessage(`{"origin":{"channel":"chan-1"}}`),
	)
	if err != nil {
		t.Fatalf("ContextStore.Create(root): %v", err)
	}

	pipelineYAML := `pipelines:
  - name: chain
    on: chain.start
    steps:
      - id: step_b
        uses: plugin-b
        baggage:
          summary.text: payload.message
          origin.channel: context.origin.channel
          from: payload.metadata
          namespace: whisper.metadata
`
	pipelinePath := filepath.Join(tmpDir, "pipelines.yaml")
	if err := os.WriteFile(pipelinePath, []byte(pipelineYAML), 0o644); err != nil {
		t.Fatalf("WriteFile(pipelines.yaml): %v", err)
	}

	routerEngine, err := router.LoadFromConfigFiles([]string{pipelinePath}, nil, nil)
	if err != nil {
		t.Fatalf("LoadFromConfigFiles: %v", err)
	}

	disp := &Dispatcher{
		contexts: contextStore,
		router:   routerEngine,
	}
	updates, err := disp.contextUpdatesForDispatch(context.Background(), router.Dispatch{
		PipelineName:    "chain",
		StepID:          "step_b",
		ParentContextID: root.ID,
		Event: protocol.Event{
			Type: "chain.start",
			Payload: map[string]any{
				"message":   "hello",
				"transient": "do not promote",
				"metadata":  map[string]any{"duration": float64(12)},
			},
		},
	})
	if err != nil {
		t.Fatalf("contextUpdatesForDispatch() error = %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(updates, &got); err != nil {
		t.Fatalf("unmarshal updates: %v", err)
	}
	if _, exists := got["message"]; exists {
		t.Fatalf("legacy message promoted into explicit baggage updates: %+v", got)
	}
	if _, exists := got["transient"]; exists {
		t.Fatalf("legacy transient key promoted into explicit baggage updates: %+v", got)
	}
	summary := got["summary"].(map[string]any)
	if summary["text"] != "hello" {
		t.Fatalf("summary.text = %v, want hello", summary["text"])
	}
	origin := got["origin"].(map[string]any)
	if origin["channel"] != "chan-1" {
		t.Fatalf("origin.channel = %v, want chan-1", origin["channel"])
	}
	whisper := got["whisper"].(map[string]any)
	metadata := whisper["metadata"].(map[string]any)
	if metadata["duration"] != float64(12) {
		t.Fatalf("whisper.metadata.duration = %v, want 12", metadata["duration"])
	}
}

func TestDispatcher_ExecuteJob_SkipsConditionalStepAndContinues(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "state.db")
	pluginsDir := filepath.Join(tmpDir, "plugins")
	if err := os.MkdirAll(pluginsDir, 0755); err != nil {
		t.Fatalf("MkdirAll(pluginsDir): %v", err)
	}

	db, err := storage.OpenSQLite(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Fatalf("db.Close(): %v", err)
		}
	}()

	q := queue.New(db)
	st := state.NewStore(db)
	contextStore := state.NewContextStore(db)
	registry := plugin.NewRegistry()
	hub := events.NewHub(128)
	workspaceBaseDir := filepath.Join(tmpDir, "workspaces")
	wsManager, err := workspace.NewFSManager(workspaceBaseDir)
	if err != nil {
		t.Fatalf("NewFSManager: %v", err)
	}

	scriptA := `#!/bin/bash
read input
echo '{"status":"ok","result":"start","events":[{"type":"chain.start","payload":{"status":"ok"}}]}'
`
	scriptB := `#!/bin/bash
read input
echo '{"status":"ok","result":"should-not-run"}'
`
	scriptC := `#!/bin/bash
read input
echo '{"status":"ok","result":"ran-c"}'
`
	plugA := createTestPlugin(t, pluginsDir, "plugin-a", scriptA)
	plugB := createTestPlugin(t, pluginsDir, "plugin-b", scriptB)
	plugC := createTestPlugin(t, pluginsDir, "plugin-c", scriptC)
	if err := registry.Add(plugA); err != nil {
		t.Fatalf("registry.Add(plugin-a): %v", err)
	}
	if err := registry.Add(plugB); err != nil {
		t.Fatalf("registry.Add(plugin-b): %v", err)
	}
	if err := registry.Add(plugC); err != nil {
		t.Fatalf("registry.Add(plugin-c): %v", err)
	}

	pipelineYAML := `pipelines:
  - name: chain
    on: chain.start
    steps:
      - id: step_b
        uses: plugin-b
        if:
          path: payload.status
          op: contains
          value: error
      - id: step_c
        uses: plugin-c
`
	pipelinePath := filepath.Join(tmpDir, "pipelines.yaml")
	if err := os.WriteFile(pipelinePath, []byte(pipelineYAML), 0644); err != nil {
		t.Fatalf("WriteFile(pipelines.yaml): %v", err)
	}

	routerEngine, err := router.LoadFromConfigFiles([]string{pipelinePath}, registry, nil)
	if err != nil {
		t.Fatalf("LoadFromConfigFiles: %v", err)
	}

	cfg := config.Defaults()
	cfg.PluginRoots = []string{pluginsDir}
	cfg.Plugins["plugin-a"] = config.PluginConf{Enabled: true, Timeouts: &config.TimeoutsConfig{Poll: 5 * time.Second}}
	cfg.Plugins["plugin-b"] = config.PluginConf{Enabled: true, Timeouts: &config.TimeoutsConfig{Handle: 5 * time.Second}}
	cfg.Plugins["plugin-c"] = config.PluginConf{Enabled: true, Timeouts: &config.TimeoutsConfig{Handle: 5 * time.Second}}

	disp := New(q, st, contextStore, wsManager, routerEngine, registry, hub, cfg)
	ctx := context.Background()

	rootJobID, err := q.Enqueue(ctx, queue.EnqueueRequest{Plugin: "plugin-a", Command: "poll", SubmittedBy: "test"})
	if err != nil {
		t.Fatalf("Enqueue(root): %v", err)
	}
	rootJob, err := q.Dequeue(ctx)
	if err != nil || rootJob == nil {
		t.Fatalf("Dequeue(root): job=%v err=%v", rootJob, err)
	}
	disp.executeJob(ctx, rootJob)

	stepBJob, err := q.Dequeue(ctx)
	if err != nil || stepBJob == nil {
		t.Fatalf("Dequeue(step_b): job=%v err=%v", stepBJob, err)
	}
	disp.executeJob(ctx, stepBJob)

	stepCJob, err := q.Dequeue(ctx)
	if err != nil || stepCJob == nil {
		t.Fatalf("Dequeue(step_c): job=%v err=%v", stepCJob, err)
	}
	if stepCJob.Plugin != "plugin-c" {
		t.Fatalf("step_c plugin = %q, want %q", stepCJob.Plugin, "plugin-c")
	}
	disp.executeJob(ctx, stepCJob)

	stepBResult, err := q.GetJobByID(ctx, stepBJob.ID)
	if err != nil {
		t.Fatalf("GetJobByID(step_b): %v", err)
	}
	if stepBResult.Status != queue.StatusSkipped {
		t.Fatalf("step_b status = %q, want %q", stepBResult.Status, queue.StatusSkipped)
	}
	if stepBResult.LastError == nil || *stepBResult.LastError != "if condition evaluated false" {
		t.Fatalf("step_b last_error = %#v", stepBResult.LastError)
	}
	if string(stepBResult.Result) == "" {
		t.Fatalf("expected skipped result payload")
	}

	stepCResult, err := q.GetJobByID(ctx, stepCJob.ID)
	if err != nil {
		t.Fatalf("GetJobByID(step_c): %v", err)
	}
	if stepCResult.Status != queue.StatusSucceeded {
		t.Fatalf("step_c status = %q, want %q", stepCResult.Status, queue.StatusSucceeded)
	}

	if _, err := os.Stat(filepath.Join(workspaceBaseDir, stepBJob.ID[:2], stepBJob.ID)); err != nil {
		t.Fatalf("stat skipped step workspace: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workspaceBaseDir, stepCJob.ID[:2], stepCJob.ID)); err != nil {
		t.Fatalf("stat successor workspace: %v", err)
	}

	_ = rootJobID
}

func filterEventsByType(eventsIn []events.Event, typ string) []events.Event {
	out := make([]events.Event, 0, len(eventsIn))
	for _, ev := range eventsIn {
		if ev.Type == typ {
			out = append(out, ev)
		}
	}
	return out
}

func TestDispatcher_Start_ParallelExecution(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "state.db")
	pluginsDir := filepath.Join(tmpDir, "plugins")
	if err := os.MkdirAll(pluginsDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	db, err := storage.OpenSQLite(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Fatalf("db.Close(): %v", err)
		}
	}()

	q := queue.New(db)
	st := state.NewStore(db)
	contextStore := state.NewContextStore(db)
	registry := plugin.NewRegistry()
	hub := events.NewHub(128)

	// Plugin that sleeps 1s then succeeds — long enough to observe concurrency
	script := `#!/bin/bash
read input
sleep 1
echo '{"status":"ok","result":"done"}'
`
	plug := createTestPlugin(t, pluginsDir, "slow", script)
	if err := registry.Add(plug); err != nil {
		t.Fatalf("registry.Add(%s): %v", plug.Name, err)
	}

	cfg := config.Defaults()
	cfg.PluginRoots = []string{pluginsDir}
	cfg.Service.MaxWorkers = 3
	cfg.Plugins["slow"] = config.PluginConf{
		Enabled:     true,
		Parallelism: 3,
		Timeouts:    &config.TimeoutsConfig{Poll: 10 * time.Second},
	}

	disp := New(q, st, contextStore, nil, nil, registry, hub, cfg)
	ctx, cancel := context.WithCancel(context.Background())

	// Enqueue 3 jobs
	for i := 0; i < 3; i++ {
		if _, err := q.Enqueue(ctx, queue.EnqueueRequest{
			Plugin: "slow", Command: "poll", SubmittedBy: "test",
		}); err != nil {
			t.Fatalf("Enqueue %d: %v", i, err)
		}
	}

	// Start dispatcher in background
	dispErr := make(chan error, 1)
	go func() { dispErr <- disp.Start(ctx) }()

	// Wait for all 3 to complete — if parallel, ~1s; if serial, ~3s
	start := time.Now()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			cancel()
			t.Fatal("timed out waiting for parallel jobs")
		case <-time.After(200 * time.Millisecond):
			var completed int
			if err := db.QueryRow("SELECT COUNT(*) FROM job_queue WHERE status = ?", queue.StatusSucceeded).Scan(&completed); err != nil {
				t.Fatalf("count succeeded jobs: %v", err)
			}
			if completed >= 3 {
				elapsed := time.Since(start)
				cancel()
				<-dispErr
				// If truly parallel: ~1s. If serial: ~3s. Accept up to 2.5s.
				if elapsed > 2500*time.Millisecond {
					t.Fatalf("jobs took %v — likely serial, not parallel", elapsed)
				}
				t.Logf("3 parallel jobs completed in %v", elapsed)
				return
			}
		}
	}
}

func TestDispatcher_Start_PerPluginParallelismCap(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "state.db")
	pluginsDir := filepath.Join(tmpDir, "plugins")
	if err := os.MkdirAll(pluginsDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	db, err := storage.OpenSQLite(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Fatalf("db.Close(): %v", err)
		}
	}()

	q := queue.New(db)
	st := state.NewStore(db)
	contextStore := state.NewContextStore(db)
	registry := plugin.NewRegistry()
	hub := events.NewHub(128)

	// Plugin that sleeps 1s
	script := `#!/bin/bash
read input
sleep 1
echo '{"status":"ok","result":"done"}'
`
	plug := createTestPlugin(t, pluginsDir, "capped", script)
	if err := registry.Add(plug); err != nil {
		t.Fatalf("registry.Add(%s): %v", plug.Name, err)
	}

	cfg := config.Defaults()
	cfg.PluginRoots = []string{pluginsDir}
	cfg.Service.MaxWorkers = 4
	cfg.Plugins["capped"] = config.PluginConf{
		Enabled:     true,
		Parallelism: 1, // Only 1 at a time despite 4 workers
		Timeouts:    &config.TimeoutsConfig{Poll: 10 * time.Second},
	}

	disp := New(q, st, contextStore, nil, nil, registry, hub, cfg)
	ctx, cancel := context.WithCancel(context.Background())

	// Enqueue 2 jobs
	for i := 0; i < 2; i++ {
		if _, err := q.Enqueue(ctx, queue.EnqueueRequest{
			Plugin: "capped", Command: "poll", SubmittedBy: "test",
		}); err != nil {
			t.Fatalf("Enqueue %d: %v", i, err)
		}
	}

	dispErr := make(chan error, 1)
	go func() { dispErr <- disp.Start(ctx) }()

	start := time.Now()
	deadline := time.After(8 * time.Second)
	for {
		select {
		case <-deadline:
			cancel()
			t.Fatal("timed out waiting for capped jobs")
		case <-time.After(200 * time.Millisecond):
			var completed int
			if err := db.QueryRow("SELECT COUNT(*) FROM job_queue WHERE status = ?", queue.StatusSucceeded).Scan(&completed); err != nil {
				t.Fatalf("count succeeded jobs: %v", err)
			}
			if completed >= 2 {
				elapsed := time.Since(start)
				cancel()
				<-dispErr
				// With parallelism=1, 2 jobs should take ~2s (serial)
				if elapsed < 1800*time.Millisecond {
					t.Fatalf("jobs took %v — should be serial with parallelism=1", elapsed)
				}
				t.Logf("2 serial-capped jobs completed in %v", elapsed)
				return
			}
		}
	}
}

func TestDispatcher_Start_SerialDefaultBackcompat(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "state.db")
	pluginsDir := filepath.Join(tmpDir, "plugins")
	if err := os.MkdirAll(pluginsDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	db, err := storage.OpenSQLite(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Fatalf("db.Close(): %v", err)
		}
	}()

	q := queue.New(db)
	st := state.NewStore(db)
	contextStore := state.NewContextStore(db)
	registry := plugin.NewRegistry()
	hub := events.NewHub(128)

	script := `#!/bin/bash
read input
sleep 1
echo '{"status":"ok","result":"done"}'
`
	plug := createTestPlugin(t, pluginsDir, "serial", script)
	if err := registry.Add(plug); err != nil {
		t.Fatalf("registry.Add(%s): %v", plug.Name, err)
	}

	// Default config: max_workers=1, parallelism=1
	cfg := config.Defaults()
	cfg.PluginRoots = []string{pluginsDir}
	cfg.Plugins["serial"] = config.PluginConf{
		Enabled:  true,
		Timeouts: &config.TimeoutsConfig{Poll: 10 * time.Second},
	}

	disp := New(q, st, contextStore, nil, nil, registry, hub, cfg)
	ctx, cancel := context.WithCancel(context.Background())

	// Enqueue 2 jobs
	for i := 0; i < 2; i++ {
		if _, err := q.Enqueue(ctx, queue.EnqueueRequest{
			Plugin: "serial", Command: "poll", SubmittedBy: "test",
		}); err != nil {
			t.Fatalf("Enqueue %d: %v", i, err)
		}
	}

	dispErr := make(chan error, 1)
	go func() { dispErr <- disp.Start(ctx) }()

	start := time.Now()
	deadline := time.After(8 * time.Second)
	for {
		select {
		case <-deadline:
			cancel()
			t.Fatal("timed out waiting for serial jobs")
		case <-time.After(200 * time.Millisecond):
			var completed int
			if err := db.QueryRow("SELECT COUNT(*) FROM job_queue WHERE status = ?", queue.StatusSucceeded).Scan(&completed); err != nil {
				t.Fatalf("count succeeded jobs: %v", err)
			}
			if completed >= 2 {
				elapsed := time.Since(start)
				cancel()
				<-dispErr
				// Default serial: 2 jobs at 1s each ≈ 2s minimum
				if elapsed < 1800*time.Millisecond {
					t.Fatalf("jobs took %v — should be serial with default config", elapsed)
				}
				t.Logf("2 default-serial jobs completed in %v", elapsed)
				return
			}
		}
	}
}

func TestDispatcher_PluginParallelism_RespectsConcurrencySafeHint(t *testing.T) {
	disp, _, _, cleanup := setupTestDispatcher(t)
	defer cleanup()

	unsafePlugin := &plugin.Plugin{Name: "unsafe", ConcurrencySafe: false}
	safePlugin := &plugin.Plugin{Name: "safe", ConcurrencySafe: true}
	if err := disp.registry.Add(unsafePlugin); err != nil {
		t.Fatalf("registry.Add(unsafe): %v", err)
	}
	if err := disp.registry.Add(safePlugin); err != nil {
		t.Fatalf("registry.Add(safe): %v", err)
	}

	disp.cfg.Service.MaxWorkers = 6
	disp.cfg.Plugins["unsafe"] = config.PluginConf{Enabled: true, Parallelism: 1}
	disp.cfg.Plugins["safe"] = config.PluginConf{Enabled: true, Parallelism: 6}

	if got := disp.pluginParallelism("unsafe"); got != 1 {
		t.Fatalf("unsafe plugin default should be serial, got %d", got)
	}

	if got := disp.pluginParallelism("safe"); got != 6 {
		t.Fatalf("safe plugin should use configured parallelism, got %d", got)
	}

	// Explicit operator override: allow unsafe plugin >1.
	pc := disp.cfg.Plugins["unsafe"]
	pc.Parallelism = 4
	disp.cfg.Plugins["unsafe"] = pc
	if got := disp.pluginParallelism("unsafe"); got != 4 {
		t.Fatalf("unsafe plugin override should be honored, got %d", got)
	}
}
