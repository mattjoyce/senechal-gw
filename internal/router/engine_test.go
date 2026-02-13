package router

import (
	"context"
	"testing"

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
