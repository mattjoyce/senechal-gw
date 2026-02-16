package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mattjoyce/ductile/internal/protocol"
)

func TestHandleStartEmitsToolRequestAndCreatesWorkspaceFiles(t *testing.T) {
	ws := t.TempDir()
	req := protocol.Request{
		Command:      "handle",
		WorkspaceDir: ws,
		Config: map[string]any{
			"allowed_plugins": []any{"jina-reader", "fabric"},
			"skills_command":  "printf 'skills-ok'",
		},
		State: map[string]any{},
		Event: &protocol.Event{
			Type: "agentic.start",
			Payload: map[string]any{
				"goal": "fetch web https://example.com and critique it",
			},
		},
	}

	resp := handleEvent(req, parseConfig(req.Config), parseState(req.State))
	if resp.Status != "ok" {
		t.Fatalf("status = %q, want ok (error=%s)", resp.Status, resp.Error)
	}
	if len(resp.Events) != 1 {
		t.Fatalf("events len = %d, want 1", len(resp.Events))
	}
	if got := resp.Events[0].Type; got != "agentic.tool_request.jina-reader" {
		t.Fatalf("event type = %q, want agentic.tool_request.jina-reader", got)
	}

	for _, name := range []string{"context.md", "memory.md", "skills.md", "plan.md", "decisions.md"} {
		if _, err := os.Stat(filepath.Join(ws, name)); err != nil {
			t.Fatalf("%s not created: %v", name, err)
		}
	}
}

func TestResumeViaContextAdvancesToFabric(t *testing.T) {
	runID := "run-1"
	state := pluginState{
		Runs: map[string]*runState{
			runID: {
				Status:       "running",
				Goal:         "goal",
				Step:         1,
				MaxLoops:     10,
				MaxReframes:  2,
				PendingStep:  1,
				PendingTool:  "jina-reader",
				PendingSince: nowISO(),
			},
		},
		LastRunID: runID,
	}

	req := protocol.Request{
		Command: "handle",
		Config: map[string]any{
			"allowed_plugins": []any{"jina-reader", "fabric"},
		},
		State: stateToMap(state),
		Context: map[string]any{
			"run_id": runID,
			"step":   1,
			"tool":   "jina-reader",
		},
		Event: &protocol.Event{
			Type: "content_ready",
			Payload: map[string]any{
				"content": "source text",
			},
		},
	}

	resp := handleEvent(req, parseConfig(req.Config), parseState(req.State))
	if resp.Status != "ok" {
		t.Fatalf("status = %q, want ok (error=%s)", resp.Status, resp.Error)
	}
	if len(resp.Events) != 1 {
		t.Fatalf("events len = %d, want 1", len(resp.Events))
	}
	if got := resp.Events[0].Type; got != "agentic.tool_request.fabric" {
		t.Fatalf("event type = %q, want agentic.tool_request.fabric", got)
	}

	nextState := parseState(resp.StateUpdates)
	run := nextState.Runs[runID]
	if run == nil {
		t.Fatalf("run %q missing from state_updates", runID)
	}
	if run.PendingStep != 2 || run.PendingTool != "fabric" {
		t.Fatalf("pending = (%d,%q), want (2,\"fabric\")", run.PendingStep, run.PendingTool)
	}
}

func TestStepMismatchEscalatesRun(t *testing.T) {
	runID := "run-2"
	state := pluginState{
		Runs: map[string]*runState{
			runID: {
				Status:      "running",
				Goal:        "goal",
				Step:        2,
				MaxLoops:    10,
				MaxReframes: 2,
				PendingStep: 2,
				PendingTool: "fabric",
			},
		},
	}

	req := protocol.Request{
		Command: "handle",
		State:   stateToMap(state),
		Event: &protocol.Event{
			Type: "agentic.tool_result",
			Payload: map[string]any{
				"run_id": runID,
				"step":   3,
				"tool":   "fabric",
				"status": "ok",
				"result": map[string]any{},
			},
		},
	}

	resp := handleEvent(req, parseConfig(req.Config), parseState(req.State))
	if resp.Status != "ok" {
		t.Fatalf("status = %q, want ok (error=%s)", resp.Status, resp.Error)
	}
	if len(resp.Events) != 1 || resp.Events[0].Type != "agent.escalated" {
		t.Fatalf("events = %#v, want single agent.escalated", resp.Events)
	}

	nextState := parseState(resp.StateUpdates)
	if got := nextState.Runs[runID].Status; got != "escalated" {
		t.Fatalf("run status = %q, want escalated", got)
	}
}
