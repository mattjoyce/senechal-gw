package router

import (
	"context"
	"testing"

	"github.com/mattjoyce/ductile/internal/plugin"
	"github.com/mattjoyce/ductile/internal/protocol"
	"github.com/mattjoyce/ductile/internal/router/dsl"
)

func TestRouterNextRootTrigger(t *testing.T) {
	set, err := dsl.CompileSpecs([]dsl.PipelineSpec{
		{
			Name: "chain",
			On:   "event.start",
			Steps: []dsl.StepSpec{
				{ID: "step_b", Uses: "plugin-b"},
			},
		},
	})
	if err != nil {
		t.Fatalf("CompileSpecs: %v", err)
	}

	r := New(set, nil)
	out, err := r.Next(context.Background(), Request{
		SourceJobID: "job-a",
		Event: protocol.Event{
			Type:    "event.start",
			Payload: map[string]any{"origin_channel_id": "chan-1"},
		},
	})
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("dispatch count = %d, want 1", len(out))
	}
	if out[0].Plugin != "plugin-b" || out[0].Command != "handle" {
		t.Fatalf("unexpected dispatch: %+v", out[0])
	}
	if out[0].PipelineName != "chain" || out[0].StepID != "step_b" {
		t.Fatalf("unexpected pipeline metadata: %+v", out[0])
	}
}

func TestRouterNextStepSuccessorTransition(t *testing.T) {
	set, err := dsl.CompileSpecs([]dsl.PipelineSpec{
		{
			Name: "chain",
			On:   "event.start",
			Steps: []dsl.StepSpec{
				{ID: "step_b", Uses: "plugin-b"},
				{ID: "step_c", Uses: "plugin-c"},
			},
		},
	})
	if err != nil {
		t.Fatalf("CompileSpecs: %v", err)
	}

	r := New(set, nil)
	out, err := r.Next(context.Background(), Request{
		SourceJobID:    "job-b",
		SourcePipeline: "chain",
		SourceStepID:   "step_b",
		SourceEventID:  "evt-1",
		Event: protocol.Event{
			Type: "any.event",
		},
	})
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("dispatch count = %d, want 1", len(out))
	}
	if out[0].Plugin != "plugin-c" || out[0].StepID != "step_c" {
		t.Fatalf("unexpected dispatch: %+v", out[0])
	}
}

func TestAddAgenticAutoToolPipelines_GeneratesFromRegistry(t *testing.T) {
	reg := plugin.NewRegistry()
	_ = reg.Add(&plugin.Plugin{Name: "agentic-loop"})
	_ = reg.Add(&plugin.Plugin{Name: "jina-reader"})
	_ = reg.Add(&plugin.Plugin{Name: "fabric"})

	set := &dsl.Set{Pipelines: map[string]*dsl.Pipeline{}}
	if err := addAgenticAutoToolPipelines(set, reg, nil); err != nil {
		t.Fatalf("addAgenticAutoToolPipelines: %v", err)
	}

	if len(set.Pipelines) != 2 {
		t.Fatalf("pipeline count = %d, want 2", len(set.Pipelines))
	}

	foundFetch := false
	foundFabric := false
	for _, p := range set.Pipelines {
		switch p.Trigger {
		case "agentic.tool_request.jina-reader":
			foundFetch = true
		case "agentic.tool_request.fabric":
			foundFabric = true
		}
	}
	if !foundFetch || !foundFabric {
		t.Fatalf("missing expected triggers (fetch=%v fabric=%v)", foundFetch, foundFabric)
	}
}

func TestAddAgenticAutoToolPipelines_RespectsExistingTrigger(t *testing.T) {
	reg := plugin.NewRegistry()
	_ = reg.Add(&plugin.Plugin{Name: "agentic-loop"})
	_ = reg.Add(&plugin.Plugin{Name: "jina-reader"})
	_ = reg.Add(&plugin.Plugin{Name: "fabric"})

	set, err := dsl.CompileSpecs([]dsl.PipelineSpec{
		{
			Name: "custom-fabric-route",
			On:   "agentic.tool_request.fabric",
			Steps: []dsl.StepSpec{
				{ID: "tool", Uses: "fabric"},
				{ID: "resume", Uses: "agentic-loop"},
			},
		},
	})
	if err != nil {
		t.Fatalf("CompileSpecs: %v", err)
	}

	if err := addAgenticAutoToolPipelines(set, reg, nil); err != nil {
		t.Fatalf("addAgenticAutoToolPipelines: %v", err)
	}

	// One existing + one generated (jina-reader)
	if len(set.Pipelines) != 2 {
		t.Fatalf("pipeline count = %d, want 2", len(set.Pipelines))
	}
}

func TestAddAgenticAutoToolPipelines_NoAgenticLoopNoop(t *testing.T) {
	reg := plugin.NewRegistry()
	_ = reg.Add(&plugin.Plugin{Name: "jina-reader"})
	_ = reg.Add(&plugin.Plugin{Name: "fabric"})

	set := &dsl.Set{Pipelines: map[string]*dsl.Pipeline{}}
	if err := addAgenticAutoToolPipelines(set, reg, nil); err != nil {
		t.Fatalf("addAgenticAutoToolPipelines: %v", err)
	}
	if len(set.Pipelines) != 0 {
		t.Fatalf("pipeline count = %d, want 0", len(set.Pipelines))
	}
}
