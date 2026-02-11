package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mattjoyce/senechal-gw/internal/config"
	"github.com/mattjoyce/senechal-gw/internal/dispatch"
	"github.com/mattjoyce/senechal-gw/internal/log"
	"github.com/mattjoyce/senechal-gw/internal/plugin"
	"github.com/mattjoyce/senechal-gw/internal/queue"
	"github.com/mattjoyce/senechal-gw/internal/router"
	"github.com/mattjoyce/senechal-gw/internal/state"
	"github.com/mattjoyce/senechal-gw/internal/storage"
	"github.com/mattjoyce/senechal-gw/internal/workspace"
)

func TestEndToEndPipeline(t *testing.T) {
	// 1. Setup Environment
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "senechal.db")
	pluginsDir := filepath.Join(tmpDir, "plugins")
	pipelinesDir := filepath.Join(tmpDir, "pipelines")
	workspacesDir := filepath.Join(tmpDir, "workspaces")

	for _, dir := range []string{pluginsDir, pipelinesDir, workspacesDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("failed to create dir %s: %v", dir, err)
		}
	}

	log.Setup("ERROR") // Keep logs clean
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	db, err := storage.OpenSQLite(ctx, dbPath)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	q := queue.New(db)
	st := state.NewStore(db)
	contextStore := state.NewContextStore(db)
	wsManager, _ := workspace.NewFSManager(workspacesDir)

	// 2. Create Real Bash Plugins
	// Hop 1: Trigger (Emits event with origin metadata)
	triggerScript := `#!/bin/bash
echo '{"status":"ok","events":[{"type":"test.triggered","event_id":"stable-id","payload":{"origin_user":"matt","video_url":"https://yt.com/123"}}]}'
`
	createPlugin(t, pluginsDir, "trigger", triggerScript)

	// Hop 2: Processor (Reads baggage, writes an artifact)
	processScript := `#!/bin/bash
input=$(cat)
ws_dir=$(echo "$input" | sed -n 's/.*"workspace_dir":"\([^"]*\)".*/\1/p')

if [ -n "$ws_dir" ]; then
  mkdir -p "$ws_dir"
  echo "processed-content" > "$ws_dir/result.txt"
fi

echo '{"status":"ok","events":[{"type":"test.processed","payload":{"status":"complete"}}]}'
`
	createPlugin(t, pluginsDir, "processor", processScript)

	// Hop 3: Notifier (Verifies original baggage AND the artifact)
	notifyScript := `#!/bin/bash
input=$(cat)
ws_dir=$(echo "$input" | sed -n 's/.*"workspace_dir":"\([^"]*\)".*/\1/p')

# Check for original baggage (origin_user)
if [[ "$input" != *"matt"* ]]; then
  echo '{"status":"error","error":"missing baggage: origin_user"}'
  exit 0
fi

# Check for artifact from Hop 2
if [ ! -f "$ws_dir/result.txt" ]; then
  echo '{"status":"error","error":"missing artifact: result.txt"}'
  exit 0
fi

echo '{"status":"ok","logs":[{"level":"info","message":"verified all hops"}]}'
`
	createPlugin(t, pluginsDir, "notifier", notifyScript)

	// 3. Define Pipeline
	pipelineYAML := `pipelines:
  - name: e2e-chain
    on: test.triggered
    steps:
      - id: step_process
        uses: processor
      - id: step_notify
        uses: notifier
`
	if err := os.WriteFile(filepath.Join(pipelinesDir, "chain.yaml"), []byte(pipelineYAML), 0644); err != nil {
		t.Fatalf("failed to write pipeline: %v", err)
	}

	// 4. Discover and Load
	registry, _ := plugin.Discover(pluginsDir, func(l, m string, a ...interface{}) {})
	routerEngine, err := router.LoadFromConfigDir(tmpDir, registry)
	if err != nil {
		t.Fatalf("failed to load router: %v", err)
	}

	cfg := config.Defaults()
	cfg.PluginsDir = pluginsDir
	for _, p := range []string{"trigger", "processor", "notifier"} {
		cfg.Plugins[p] = config.PluginConf{Enabled: true}
	}

	disp := dispatch.New(q, st, contextStore, wsManager, routerEngine, registry, cfg)

	// 5. Execution Loop
	// Step A: Manually enqueue the root job
	rootID, _ := q.Enqueue(ctx, queue.EnqueueRequest{
		Plugin: "trigger", Command: "poll", SubmittedBy: "test",
	})

	var rootJob *queue.Job
	// Run until queue is empty (max 3 jobs expected)
	for i := 0; i < 5; i++ {
		job, _ := q.Dequeue(ctx)
		if job == nil {
			time.Sleep(100 * time.Millisecond)
			continue
		}
		if job.ID == rootID {
			rootJob = job
		}
		disp.ExecuteJob(ctx, job)
	}

	// 6. Assertions
	var notifierJobID string
	db.QueryRow("SELECT id FROM job_queue WHERE plugin = 'notifier' LIMIT 1").Scan(&notifierJobID)
	if notifierJobID == "" {
		t.Fatalf("notifier job never ran")
	}

	var notifierStatus string
	db.QueryRow("SELECT status FROM job_queue WHERE id = ?", notifierJobID).Scan(&notifierStatus)
	if notifierStatus != "succeeded" {
		t.Fatalf("notifier job failed: %s", notifierStatus)
	}

	var contextID string
	db.QueryRow("SELECT event_context_id FROM job_queue WHERE id = ?", notifierJobID).Scan(&contextID)
	lineage, err := contextStore.Lineage(ctx, contextID)
	if err != nil {
		t.Fatalf("failed to load lineage: %v", err)
	}
	// Step 1: Processor (Context 1, Parent nil)
	// Step 2: Notifier (Context 2, Parent Context 1)
	if len(lineage) != 2 {
		t.Errorf("expected 2 context hops, got %d", len(lineage))
	}

	// Verify Baggage Flow
	var finalBaggage map[string]any
	json.Unmarshal(lineage[len(lineage)-1].AccumulatedJSON, &finalBaggage)
	if finalBaggage["origin_user"] != "matt" {
		t.Errorf("origin_user baggage lost, got: %v", finalBaggage)
	}

	// 7. Verify Idempotency (Parent Retry)
	// If we run the trigger job again, it should NOT create new child jobs
	// because the (parent_job_id, source_event_id) unique constraint will trigger INSERT OR IGNORE.
	disp.ExecuteJob(ctx, rootJob)

	var childCount int
	db.QueryRow("SELECT COUNT(*) FROM job_queue WHERE parent_job_id = ?", rootID).Scan(&childCount)
	if childCount != 1 {
		t.Errorf("expected exactly 1 child job after parent retry, got %d", childCount)
	}
}

func createPlugin(t *testing.T, dir, name, script string) {
	pDir := filepath.Join(dir, name)
	os.MkdirAll(pDir, 0755)
	manifest := fmt.Sprintf("name: %s\nversion: 1.0.0\nprotocol: 1\nentrypoint: ./run.sh\ncommands: [poll, handle]", name)
	os.WriteFile(filepath.Join(pDir, "manifest.yaml"), []byte(manifest), 0644)
	os.WriteFile(filepath.Join(pDir, "run.sh"), []byte(script), 0755)
}
