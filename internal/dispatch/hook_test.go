package dispatch

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/mattjoyce/ductile/internal/config"
	"github.com/mattjoyce/ductile/internal/protocol"
	"github.com/mattjoyce/ductile/internal/queue"
	"github.com/mattjoyce/ductile/internal/router"
	"github.com/mattjoyce/ductile/internal/router/dsl"
)

func TestDispatcher_NotifyOnComplete_Hooks(t *testing.T) {
	disp, db, pluginsDir, cleanup := setupTestDispatcher(t)
	defer cleanup()

	// 1. Create plugins
	echoScript := `#!/bin/bash
read input
echo '{"status": "ok", "result": "echo-done"}'
`
	echoPlug := createTestPlugin(t, pluginsDir, "echo", echoScript)
	if err := disp.registry.Add(echoPlug); err != nil {
		t.Fatalf("registry.Add(echo): %v", err)
	}

	notifierScript := `#!/bin/bash
read input
echo '{"status": "ok", "result": "notified"}'
`
	notifierPlug := createTestPlugin(t, pluginsDir, "notifier", notifierScript)
	if err := disp.registry.Add(notifierPlug); err != nil {
		t.Fatalf("registry.Add(notifier): %v", err)
	}

	// 2. Set up router with a hook pipeline
	set, err := dsl.CompileSpecs([]dsl.PipelineSpec{
		{
			Name:   "notify-on-complete",
			OnHook: "job.completed",
			Steps:  []dsl.StepSpec{{ID: "notify", Uses: "notifier"}},
		},
	})
	if err != nil {
		t.Fatalf("CompileSpecs: %v", err)
	}
	disp.router = router.New(set, nil)

	// 3. Configure plugins
	trueVal := true
	disp.cfg.Plugins["echo"] = config.PluginConf{
		Enabled:          true,
		NotifyOnComplete: &trueVal,
		Timeouts:         &config.TimeoutsConfig{Poll: 5 * time.Second},
	}
	disp.cfg.Plugins["notifier"] = config.PluginConf{
		Enabled:  true,
		Timeouts: &config.TimeoutsConfig{Handle: 5 * time.Second},
	}

	ctx := context.Background()

	// Test Case A: Root job with NotifyOnComplete=true fires hook
	t.Run("RootJobFiresHook", func(t *testing.T) {
		jobID, err := disp.queue.Enqueue(ctx, queue.EnqueueRequest{
			Plugin:      "echo",
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

		// Verify echo job succeeded
		var status string
		err = db.QueryRow("SELECT status FROM job_queue WHERE id = ?", jobID).Scan(&status)
		if err != nil {
			t.Fatalf("failed to query echo job status: %v", err)
		}
		if status != "succeeded" {
			t.Fatalf("expected status=succeeded, got %s", status)
		}

		// Verify hook job was enqueued
		hookJob, err := disp.queue.Dequeue(ctx)
		if err != nil {
			t.Fatalf("failed to dequeue hook job: %v", err)
		}
		if hookJob == nil {
			t.Fatal("expected hook job to be enqueued, but got none")
		}
		if hookJob.Plugin != "notifier" {
			t.Errorf("hook job plugin = %q, want %q", hookJob.Plugin, "notifier")
		}
		if hookJob.SubmittedBy != "hook" {
			t.Errorf("hook job submitted_by = %q, want %q", hookJob.SubmittedBy, "hook")
		}

		// Verify hook payload contains job details
		var ev protocol.Event
		if err := json.Unmarshal(hookJob.Payload, &ev); err != nil {
			t.Fatalf("failed to unmarshal hook payload: %v", err)
		}
		if ev.Type != "job.completed" {
			t.Errorf("hook event type = %q, want %q", ev.Type, "job.completed")
		}
		if ev.Payload["plugin"] != "echo" {
			t.Errorf("hook payload plugin = %v, want echo", ev.Payload["plugin"])
		}
		if ev.Payload["status"] != "succeeded" {
			t.Errorf("hook payload status = %v, want succeeded", ev.Payload["status"])
		}
	})

	// Test Case B: Pipeline step DOES NOT fire hook (to avoid recursion/noise)
	t.Run("PipelineStepDoesNotFireHook", func(t *testing.T) {
		ctxID := "ctx-1"
		jobID, err := disp.queue.Enqueue(ctx, queue.EnqueueRequest{
			Plugin:         "echo",
			Command:        "poll",
			EventContextID: &ctxID, // Makes it look like a pipeline step
			SubmittedBy:    "test",
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
			t.Fatalf("failed to query job status: %v", err)
		}
		if status != "succeeded" {
			t.Fatalf("expected status=succeeded, got %s", status)
		}

		// Verify NO hook job was enqueued
		hookJob, err := disp.queue.Dequeue(ctx)
		if err != nil {
			t.Fatalf("failed to dequeue (expected none): %v", err)
		}
		if hookJob != nil {
			t.Fatalf("expected no hook job for pipeline step, but got one for %q", hookJob.Plugin)
		}
	})

	// Test Case C: Opt-out (NotifyOnComplete=false) does not fire hook
	t.Run("OptOutDoesNotFireHook", func(t *testing.T) {
		falseVal := false
		pc := disp.cfg.Plugins["echo"]
		pc.NotifyOnComplete = &falseVal
		disp.cfg.Plugins["echo"] = pc

		_, err := disp.queue.Enqueue(ctx, queue.EnqueueRequest{
			Plugin:      "echo",
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

		// Verify NO hook job was enqueued
		hookJob, err := disp.queue.Dequeue(ctx)
		if err != nil {
			t.Fatalf("failed to dequeue (expected none): %v", err)
		}
		if hookJob != nil {
			t.Fatalf("expected no hook job when disabled, but got one for %q", hookJob.Plugin)
		}
	})
}

func TestDispatcher_NotifyOnComplete_FailedJob(t *testing.T) {
	disp, db, pluginsDir, cleanup := setupTestDispatcher(t)
	defer cleanup()

	// 1. Create plugins
	failScript := `#!/bin/bash
read input
echo '{"status": "error", "error": "failed intentionally"}'
`
	failPlug := createTestPlugin(t, pluginsDir, "failer", failScript)
	if err := disp.registry.Add(failPlug); err != nil {
		t.Fatalf("registry.Add(failer): %v", err)
	}

	notifierScript := `#!/bin/bash
read input
echo '{"status": "ok", "result": "notified"}'
`
	notifierPlug := createTestPlugin(t, pluginsDir, "notifier", notifierScript)
	if err := disp.registry.Add(notifierPlug); err != nil {
		t.Fatalf("registry.Add(notifier): %v", err)
	}

	// 2. Set up router with hook pipeline
	set, err := dsl.CompileSpecs([]dsl.PipelineSpec{
		{
			Name:   "notify-on-complete",
			OnHook: "job.completed",
			Steps:  []dsl.StepSpec{{ID: "notify", Uses: "notifier"}},
		},
	})
	if err != nil {
		t.Fatalf("CompileSpecs: %v", err)
	}
	disp.router = router.New(set, nil)

	// 3. Configure plugins
	trueVal := true
	disp.cfg.Plugins["failer"] = config.PluginConf{
		Enabled:          true,
		NotifyOnComplete: &trueVal,
		Timeouts:         &config.TimeoutsConfig{Poll: 5 * time.Second},
	}
	disp.cfg.Plugins["notifier"] = config.PluginConf{
		Enabled:  true,
		Timeouts: &config.TimeoutsConfig{Handle: 5 * time.Second},
	}

	ctx := context.Background()

	// 4. Enqueue and execute failing job
	jobID, err := disp.queue.Enqueue(ctx, queue.EnqueueRequest{
		Plugin:      "failer",
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
	var status string
	err = db.QueryRow("SELECT status FROM job_queue WHERE id = ?", jobID).Scan(&status)
	if err != nil {
		t.Fatalf("failed to query job status: %v", err)
	}
	if status != "failed" {
		t.Fatalf("expected status=failed, got %s", status)
	}

	// Verify hook job was enqueued despite failure
	hookJob, err := disp.queue.Dequeue(ctx)
	if err != nil {
		t.Fatalf("failed to dequeue hook job: %v", err)
	}
	if hookJob == nil {
		t.Fatal("expected hook job for failed job, but got none")
	}

	var ev protocol.Event
	if err := json.Unmarshal(hookJob.Payload, &ev); err != nil {
		t.Fatalf("failed to unmarshal hook payload: %v", err)
	}
	if ev.Payload["status"] != "failed" {
		t.Errorf("hook payload status = %v, want failed", ev.Payload["status"])
	}
	if ev.Payload["error"] != "failed intentionally" {
		t.Errorf("hook payload error = %v, want 'failed intentionally'", ev.Payload["error"])
	}
}
