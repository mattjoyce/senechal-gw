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

	"github.com/mattjoyce/ductile/internal/config"
	"github.com/mattjoyce/ductile/internal/plugin"
	"github.com/mattjoyce/ductile/internal/protocol"
	"github.com/mattjoyce/ductile/internal/queue"
	"github.com/mattjoyce/ductile/internal/router"
	"github.com/mattjoyce/ductile/internal/router/dsl"
)

func TestDispatcher_ExecuteJob_HookReceivesCompletedEventEnvelope(t *testing.T) {
	disp, db, pluginsDir, cleanup := setupTestDispatcher(t)
	defer cleanup()

	capturePath := filepath.Join(t.TempDir(), "hook-request.json")
	setupHookFixture(t, disp, pluginsDir, capturePath, "job.completed", sourceSuccessScript())

	ctx := context.Background()
	jobID := enqueueTestJob(t, ctx, disp.queue, "source", "poll", 1)
	rootJob := dequeueTestJob(t, ctx, disp.queue)
	disp.executeJob(ctx, rootJob)

	assertJobStatus(t, db, jobID, queue.StatusSucceeded)

	hookJob := dequeueTestJob(t, ctx, disp.queue)
	if hookJob.Plugin != "hook" || hookJob.Command != "handle" {
		t.Fatalf("unexpected hook job: %+v", hookJob)
	}

	var event protocol.Event
	if err := json.Unmarshal(hookJob.Payload, &event); err != nil {
		t.Fatalf("unmarshal hook payload: %v", err)
	}
	if event.Type != "job.completed" {
		t.Fatalf("hook payload event.type = %q, want %q", event.Type, "job.completed")
	}

	disp.executeJob(ctx, hookJob)
	req := readCapturedHookRequest(t, capturePath)
	if req.Event == nil {
		t.Fatal("hook request event is nil")
	}
	if req.Event.Type != "job.completed" {
		t.Fatalf("hook request event.type = %q, want %q", req.Event.Type, "job.completed")
	}
	if got := req.Event.Payload["plugin"]; got != "source" {
		t.Fatalf("hook request payload plugin = %v, want source", got)
	}
	if got := req.Event.Payload["status"]; got != "succeeded" {
		t.Fatalf("hook request payload status = %v, want succeeded", got)
	}
}

func TestDispatcher_ExecuteJob_HookReceivesFailedEventEnvelope(t *testing.T) {
	disp, db, pluginsDir, cleanup := setupTestDispatcher(t)
	defer cleanup()

	capturePath := filepath.Join(t.TempDir(), "hook-request.json")
	setupHookFixture(t, disp, pluginsDir, capturePath, "job.failed", sourceFailureScript())

	ctx := context.Background()
	jobID := enqueueTestJob(t, ctx, disp.queue, "source", "poll", 1)
	rootJob := dequeueTestJob(t, ctx, disp.queue)
	disp.executeJob(ctx, rootJob)

	assertJobStatus(t, db, jobID, queue.StatusFailed)

	hookJob := dequeueTestJob(t, ctx, disp.queue)
	if hookJob.Plugin != "hook" || hookJob.Command != "handle" {
		t.Fatalf("unexpected hook job: %+v", hookJob)
	}

	var event protocol.Event
	if err := json.Unmarshal(hookJob.Payload, &event); err != nil {
		t.Fatalf("unmarshal hook payload: %v", err)
	}
	if event.Type != "job.failed" {
		t.Fatalf("hook payload event.type = %q, want %q", event.Type, "job.failed")
	}

	disp.executeJob(ctx, hookJob)
	req := readCapturedHookRequest(t, capturePath)
	if req.Event == nil {
		t.Fatal("hook request event is nil")
	}
	if req.Event.Type != "job.failed" {
		t.Fatalf("hook request event.type = %q, want %q", req.Event.Type, "job.failed")
	}
	if got := req.Event.Payload["status"]; got != string(queue.StatusFailed) {
		t.Fatalf("hook request payload status = %v, want %q", got, queue.StatusFailed)
	}
	if got := req.Event.Payload["error"]; got != "boom" {
		t.Fatalf("hook request payload error = %v, want boom", got)
	}
}

func setupHookFixture(t *testing.T, disp *Dispatcher, pluginsDir, capturePath, signal, sourceScript string) {
	t.Helper()

	sourcePlugin := createTestPlugin(t, pluginsDir, "source", sourceScript)
	hookPlugin := createTestPlugin(t, pluginsDir, "hook", hookCaptureScript(capturePath))
	addTestPlugin(t, disp, sourcePlugin)
	addTestPlugin(t, disp, hookPlugin)

	notify := true
	disp.cfg.Plugins["source"] = config.PluginConf{
		Enabled:          true,
		NotifyOnComplete: &notify,
		Timeouts: &config.TimeoutsConfig{
			Poll: 5 * time.Second,
		},
	}
	disp.cfg.Plugins["hook"] = config.PluginConf{
		Enabled: true,
		Timeouts: &config.TimeoutsConfig{
			Handle: 5 * time.Second,
		},
	}
	disp.router = buildHookRouter(t, signal)
}

func addTestPlugin(t *testing.T, disp *Dispatcher, plug *plugin.Plugin) {
	t.Helper()
	if err := disp.registry.Add(plug); err != nil {
		t.Fatalf("registry.Add(%s): %v", plug.Name, err)
	}
}

func buildHookRouter(t *testing.T, signal string) router.Engine {
	t.Helper()

	set, err := dsl.CompileSpecs([]dsl.PipelineSpec{{
		Name:   "hook-pipeline",
		OnHook: signal,
		Steps:  []dsl.StepSpec{{ID: "notify", Uses: "hook"}},
	}})
	if err != nil {
		t.Fatalf("CompileSpecs: %v", err)
	}
	return router.New(set, nil)
}

func enqueueTestJob(t *testing.T, ctx context.Context, q *queue.Queue, pluginName, command string, maxAttempts int) string {
	t.Helper()

	jobID, err := q.Enqueue(ctx, queue.EnqueueRequest{
		Plugin:      pluginName,
		Command:     command,
		MaxAttempts: maxAttempts,
		SubmittedBy: "test",
	})
	if err != nil {
		t.Fatalf("Enqueue(%s): %v", pluginName, err)
	}
	return jobID
}

func dequeueTestJob(t *testing.T, ctx context.Context, q *queue.Queue) *queue.Job {
	t.Helper()

	job, err := q.Dequeue(ctx)
	if err != nil {
		t.Fatalf("Dequeue(): %v", err)
	}
	if job == nil {
		t.Fatal("expected queued job")
	}
	return job
}

func assertJobStatus(t *testing.T, db *sql.DB, jobID string, want queue.Status) {
	t.Helper()

	var got string
	if err := db.QueryRow("SELECT status FROM job_queue WHERE id = ?", jobID).Scan(&got); err != nil {
		t.Fatalf("query job status: %v", err)
	}
	if got != string(want) {
		t.Fatalf("job status = %q, want %q", got, want)
	}
}

func readCapturedHookRequest(t *testing.T, capturePath string) protocol.Request {
	t.Helper()

	raw, err := os.ReadFile(capturePath)
	if err != nil {
		t.Fatalf("read hook capture: %v", err)
	}
	var req protocol.Request
	if err := json.Unmarshal(raw, &req); err != nil {
		t.Fatalf("unmarshal hook request: %v", err)
	}
	return req
}

func sourceSuccessScript() string {
	return `#!/bin/bash
read input
echo '{"status":"ok","result":"source ok"}'
`
}

func sourceFailureScript() string {
	return `#!/bin/bash
read input
echo '{"status":"error","error":"boom","retry":false}'
`
}

func hookCaptureScript(capturePath string) string {
	return fmt.Sprintf(`#!/bin/bash
cat > %q
echo '{"status":"ok","result":"hook ok"}'
`, capturePath)
}
