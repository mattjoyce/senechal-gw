package dispatch

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mattjoyce/senechal-gw/internal/config"
	"github.com/mattjoyce/senechal-gw/internal/plugin"
	"github.com/mattjoyce/senechal-gw/internal/queue"
	"github.com/mattjoyce/senechal-gw/internal/router"
)

func TestDispatcher_WaitForJobTree(t *testing.T) {
	disp, _, pluginsDir, cleanup := setupTestDispatcher(t)
	defer cleanup()

	// 1. Setup a two-hop pipeline
	// Plugin A emits an event that triggers Plugin B
	scriptA := `#!/bin/bash
read input
echo '{"status":"ok","events":[{"type":"test.event"}]}'
`
	scriptB := `#!/bin/bash
read input
echo '{"status":"ok","logs":[{"level":"info","message":"done"}]}'
`

	registry := plugin.NewRegistry()
	plugA := createTestPlugin(t, pluginsDir, "plugin-a", scriptA)
	plugB := createTestPlugin(t, pluginsDir, "plugin-b", scriptB)
	registry.Add(plugA)
	registry.Add(plugB)

	// Create pipeline
	tmpDir := filepath.Dir(pluginsDir)
	pipelinesDir := filepath.Join(tmpDir, "pipelines")
	os.MkdirAll(pipelinesDir, 0755)
	pipelineYAML := `pipelines:
  - name: test-pipeline
    on: test.event
    steps:
      - id: step_b
        uses: plugin-b
`
	os.WriteFile(filepath.Join(pipelinesDir, "test.yaml"), []byte(pipelineYAML), 0644)

	routerEngine, err := router.LoadFromConfigDir(tmpDir, registry, nil)
	if err != nil {
		t.Fatalf("LoadFromConfigDir: %v", err)
	}
	disp.router = routerEngine
	disp.registry = registry
	disp.cfg.Plugins["plugin-a"] = config.PluginConf{Enabled: true}
	disp.cfg.Plugins["plugin-b"] = config.PluginConf{Enabled: true}

	ctx := context.Background()

	// 2. Enqueue root job
	rootJobID, err := disp.queue.Enqueue(ctx, queue.EnqueueRequest{
		Plugin:      "plugin-a",
		Command:     "poll",
		SubmittedBy: "test",
	})
	if err != nil {
		t.Fatalf("Enqueue(root): %v", err)
	}

	// 3. Start a goroutine to wait for the tree
	done := make(chan struct{})
	var treeResults []*queue.JobResult
	var waitErr error
	go func() {
		treeResults, waitErr = disp.WaitForJobTree(ctx, rootJobID, 5*time.Second)
		close(done)
	}()

	// Give the goroutine a moment to start and enter wait
	time.Sleep(100 * time.Millisecond)

	// 4. Process the jobs one by one
	// Root job
	jobA, err := disp.queue.Dequeue(ctx)
	if err != nil {
		t.Fatalf("Dequeue(a): %v", err)
	}
	disp.executeJob(ctx, jobA)

	// Child job should be enqueued now
	jobB, err := disp.queue.Dequeue(ctx)
	if err != nil {
		t.Fatalf("Dequeue(b): %v", err)
	}
	if jobB == nil {
		t.Fatal("expected child job B to be enqueued")
	}
	disp.executeJob(ctx, jobB)

	// 5. Wait for the sync call to return
	select {
	case <-done:
		if waitErr != nil {
			t.Fatalf("WaitForJobTree failed: %v", waitErr)
		}
		if len(treeResults) != 2 {
			t.Errorf("expected 2 jobs in tree, got %d", len(treeResults))
		}
		// Root job should be first or second, results are not strictly ordered by the CTE
		foundA := false
		foundB := false
		for _, res := range treeResults {
			if res.Plugin == "plugin-a" {
				foundA = true
			}
			if res.Plugin == "plugin-b" {
				foundB = true
			}
		}
		if !foundA || !foundB {
			t.Errorf("did not find both plugins in results: A=%v, B=%v", foundA, foundB)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("WaitForJobTree timed out")
	}
}
