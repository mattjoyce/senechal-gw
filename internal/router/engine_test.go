package router

import (
	"context"
	"testing"
	"time"

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

func TestRouterGetNode(t *testing.T) {
	set, err := dsl.CompileSpecs([]dsl.PipelineSpec{{
		Name:  "chain",
		On:    "event.start",
		Steps: []dsl.StepSpec{{ID: "step_b", Uses: "plugin-b"}},
	}})
	if err != nil {
		t.Fatalf("CompileSpecs: %v", err)
	}

	r := New(set, nil)
	node, ok := r.GetNode("chain", "step_b")
	if !ok {
		t.Fatalf("expected node lookup success")
	}
	if node.ID != "step_b" {
		t.Fatalf("node.ID = %q, want %q", node.ID, "step_b")
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

func TestCloneEvent(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name string
		ev   protocol.Event
	}{
		{
			name: "full event",
			ev: protocol.Event{
				Type:      "test.event",
				DedupeKey: "key-1",
				Source:    "plugin-1",
				Timestamp: now,
				EventID:   "evt-123",
				Payload:   map[string]any{"foo": "bar", "num": 42},
			},
		},
		{
			name: "nil payload",
			ev: protocol.Event{
				Type:      "test.event",
				Timestamp: now,
			},
		},
		{
			name: "empty payload",
			ev: protocol.Event{
				Type:      "test.event",
				Timestamp: now,
				Payload:   map[string]any{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cloned := cloneEvent(tt.ev)

			if cloned.Type != tt.ev.Type {
				t.Errorf("Type mismatch: got %q, want %q", cloned.Type, tt.ev.Type)
			}
			if cloned.DedupeKey != tt.ev.DedupeKey {
				t.Errorf("DedupeKey mismatch: got %q, want %q", cloned.DedupeKey, tt.ev.DedupeKey)
			}
			if cloned.Source != tt.ev.Source {
				t.Errorf("Source mismatch: got %q, want %q", cloned.Source, tt.ev.Source)
			}
			if !cloned.Timestamp.Equal(tt.ev.Timestamp) {
				t.Errorf("Timestamp mismatch: got %v, want %v", cloned.Timestamp, tt.ev.Timestamp)
			}
			if cloned.EventID != tt.ev.EventID {
				t.Errorf("EventID mismatch: got %q, want %q", cloned.EventID, tt.ev.EventID)
			}

			if tt.ev.Payload == nil {
				if cloned.Payload != nil {
					t.Error("Payload should be nil")
				}
			} else {
				if cloned.Payload == nil {
					t.Fatal("Payload should not be nil")
				}
				if len(cloned.Payload) != len(tt.ev.Payload) {
					t.Errorf("Payload length mismatch: got %d, want %d", len(cloned.Payload), len(tt.ev.Payload))
				}
				for k, v := range tt.ev.Payload {
					if cloned.Payload[k] != v {
						t.Errorf("Payload key %q mismatch: got %v, want %v", k, cloned.Payload[k], v)
					}
				}

				// Verify it's a new map
				cloned.Payload["new_key"] = "new_val"
				if _, ok := tt.ev.Payload["new_key"]; ok {
					t.Error("Modifying cloned payload affected original payload")
				}
			}
		})
	}
}
