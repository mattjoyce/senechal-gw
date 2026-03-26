package dsl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mattjoyce/ductile/internal/router/conditions"
)

func TestCompileSpecsValidNestedPipeline(t *testing.T) {
	specs := []PipelineSpec{
		{
			Name: "process-audio",
			On:   "internal.process_audio",
			Steps: []StepSpec{
				{ID: "transcribe", Uses: "transcriber"},
			},
		},
		{
			Name: "wisdom-chain",
			On:   "discord.video_link_received",
			Steps: []StepSpec{
				{ID: "downloader", Uses: "yt-dlp-plugin"},
				{ID: "processing", Call: "process-audio"},
				{
					ID: "delivery",
					Split: []StepSpec{
						{ID: "notify", Uses: "discord-notifier"},
						{
							Steps: []StepSpec{
								{ID: "archive", Uses: "s3-archiver"},
								{ID: "index", Uses: "db-indexer"},
							},
						},
					},
				},
			},
		},
	}

	set, err := CompileSpecs(specs)
	if err != nil {
		t.Fatalf("CompileSpecs() error = %v", err)
	}

	p := set.Pipelines["wisdom-chain"]
	if p == nil {
		t.Fatalf("compiled pipeline wisdom-chain not found")
	}
	if p.Trigger != "discord.video_link_received" {
		t.Fatalf("trigger = %q, want %q", p.Trigger, "discord.video_link_received")
	}
	if len(p.Nodes) != 5 {
		t.Fatalf("node count = %d, want 5", len(p.Nodes))
	}
	if !hasEdge(p.Edges, "downloader", "processing") {
		t.Fatalf("expected edge downloader -> processing")
	}
	if !hasEdge(p.Edges, "processing", "notify") {
		t.Fatalf("expected edge processing -> notify")
	}
	if !hasEdge(p.Edges, "processing", "archive") {
		t.Fatalf("expected edge processing -> archive")
	}
	if !hasEdge(p.Edges, "archive", "index") {
		t.Fatalf("expected edge archive -> index")
	}
	if len(p.EntryNodeIDs) != 1 || p.EntryNodeIDs[0] != "downloader" {
		t.Fatalf("entry nodes = %v, want [downloader]", p.EntryNodeIDs)
	}
	if len(p.TerminalNodeIDs) != 2 || p.TerminalNodeIDs[0] != "index" || p.TerminalNodeIDs[1] != "notify" {
		t.Fatalf("terminal nodes = %v, want [index notify]", p.TerminalNodeIDs)
	}
	if len(p.CalledPipelines) != 1 || p.CalledPipelines[0] != "process-audio" {
		t.Fatalf("called pipelines = %v, want [process-audio]", p.CalledPipelines)
	}
	if !strings.HasPrefix(p.Fingerprint, "blake3:") {
		t.Fatalf("fingerprint = %q, want prefix blake3:", p.Fingerprint)
	}

	// Recompile to confirm deterministic pipeline fingerprint.
	set2, err := CompileSpecs(specs)
	if err != nil {
		t.Fatalf("CompileSpecs() second run error = %v", err)
	}
	if p.Fingerprint != set2.Pipelines["wisdom-chain"].Fingerprint {
		t.Fatalf("fingerprint changed across compile runs: %q vs %q", p.Fingerprint, set2.Pipelines["wisdom-chain"].Fingerprint)
	}
}

func TestCompileSpecsRejectsPipelineCallCycle(t *testing.T) {
	specs := []PipelineSpec{
		{
			Name: "a",
			On:   "a.start",
			Steps: []StepSpec{
				{ID: "a1", Call: "b"},
			},
		},
		{
			Name: "b",
			On:   "b.start",
			Steps: []StepSpec{
				{ID: "b1", Call: "a"},
			},
		},
	}

	_, err := CompileSpecs(specs)
	if err == nil {
		t.Fatalf("expected cycle detection error, got nil")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("expected cycle error, got %v", err)
	}
}

func TestCompileSpecsRejectsUnknownCallTarget(t *testing.T) {
	specs := []PipelineSpec{
		{
			Name: "a",
			On:   "a.start",
			Steps: []StepSpec{
				{ID: "a1", Call: "missing"},
			},
		},
	}

	_, err := CompileSpecs(specs)
	if err == nil {
		t.Fatalf("expected unknown call target error, got nil")
	}
	if !strings.Contains(err.Error(), "unknown pipeline") {
		t.Fatalf("expected unknown pipeline error, got %v", err)
	}
}

func TestCompileSpecsAcceptsStructuredIfCondition(t *testing.T) {
	specs := []PipelineSpec{{
		Name: "conditional",
		On:   "event.start",
		Steps: []StepSpec{{
			ID:   "step_a",
			Uses: "plugin-a",
			If: &conditions.Condition{
				All: []conditions.Condition{
					{Path: "payload.kind", Op: conditions.OpContains, Value: "vid"},
					{Path: "context.origin_user", Op: conditions.OpExists},
				},
			},
		}},
	}}

	set, err := CompileSpecs(specs)
	if err != nil {
		t.Fatalf("CompileSpecs() error = %v", err)
	}
	if set.Pipelines["conditional"].Nodes["step_a"].Condition == nil {
		t.Fatalf("expected compiled node condition")
	}
}

func TestCompileSpecsRejectsWithOnCallStep(t *testing.T) {
	specs := []PipelineSpec{{
		Name: "invalid-with",
		On:   "event.start",
		Steps: []StepSpec{{
			ID:   "call-step",
			Call: "other-pipeline",
			With: map[string]string{"message": "{payload.message}"},
		}},
	}}

	_, err := CompileSpecs(specs)
	if err == nil {
		t.Fatalf("expected invalid with usage error")
	}
	if !strings.Contains(err.Error(), "with is only supported on uses steps") {
		t.Fatalf("error = %v, want with usage validation", err)
	}
}

func TestCompileSpecsRejectsInvalidIfCondition(t *testing.T) {
	specs := []PipelineSpec{{
		Name: "conditional",
		On:   "event.start",
		Steps: []StepSpec{{
			ID:   "step_a",
			Uses: "plugin-a",
			If:   &conditions.Condition{Path: "state.flag", Op: conditions.OpEq, Value: true},
		}},
	}}

	_, err := CompileSpecs(specs)
	if err == nil {
		t.Fatalf("expected invalid if condition error")
	}
	if !strings.Contains(err.Error(), "unsupported root") {
		t.Fatalf("error = %v, want unsupported root", err)
	}
}

func TestLoadAndCompileFilesLoadsMultipleYAMLFiles(t *testing.T) {
	configDir := t.TempDir()

	rootYAML := `pipelines:
  - name: wisdom-chain
    on: discord.video_link_received
    steps:
      - id: start
        call: process-audio
`
	processYAML := `pipelines:
  - name: process-audio
    on: internal.process_audio
    steps:
      - id: transcribe
        uses: transcriber
`

	rootPath := filepath.Join(configDir, "01-root.yaml")
	processPath := filepath.Join(configDir, "02-process.yaml")
	if err := os.WriteFile(rootPath, []byte(rootYAML), 0o644); err != nil {
		t.Fatalf("WriteFile(01-root.yaml): %v", err)
	}
	if err := os.WriteFile(processPath, []byte(processYAML), 0o644); err != nil {
		t.Fatalf("WriteFile(02-process.yaml): %v", err)
	}

	set, err := LoadAndCompileFiles([]string{rootPath, processPath})
	if err != nil {
		t.Fatalf("LoadAndCompileFiles() error = %v", err)
	}
	if len(set.Pipelines) != 2 {
		t.Fatalf("pipeline count = %d, want 2", len(set.Pipelines))
	}
}

func TestCompileSpecsOnHookAccepted(t *testing.T) {
	specs := []PipelineSpec{{
		Name:   "notify-on-done",
		OnHook: "job.completed",
		Steps:  []StepSpec{{ID: "notify", Uses: "discord-notifier"}},
	}}

	set, err := CompileSpecs(specs)
	if err != nil {
		t.Fatalf("CompileSpecs() error = %v, want nil", err)
	}

	p := set.Pipelines["notify-on-done"]
	if p == nil {
		t.Fatalf("pipeline not found")
	}
	if !p.IsHook {
		t.Fatalf("IsHook = false, want true")
	}
	if p.Trigger != "job.completed" {
		t.Fatalf("Trigger = %q, want %q", p.Trigger, "job.completed")
	}
}

func TestCompileSpecsOnHookAndOnMutuallyExclusive(t *testing.T) {
	specs := []PipelineSpec{{
		Name:   "bad-pipeline",
		On:     "some.event",
		OnHook: "job.completed",
		Steps:  []StepSpec{{ID: "step", Uses: "plugin-a"}},
	}}

	_, err := CompileSpecs(specs)
	if err == nil {
		t.Fatalf("expected error for on+on-hook, got nil")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("error = %v, want mutually exclusive", err)
	}
}

func TestCompileSpecsRequiresOnOrOnHook(t *testing.T) {
	specs := []PipelineSpec{{
		Name:  "missing-trigger",
		Steps: []StepSpec{{ID: "step", Uses: "plugin-a"}},
	}}

	_, err := CompileSpecs(specs)
	if err == nil {
		t.Fatalf("expected error when neither on nor on-hook set, got nil")
	}
	if !strings.Contains(err.Error(), "on or on-hook is required") {
		t.Fatalf("error = %v, want 'on or on-hook is required'", err)
	}
}

func TestCompileSpecsRegularPipelineIsNotHook(t *testing.T) {
	specs := []PipelineSpec{{
		Name:  "regular",
		On:    "plugin.event",
		Steps: []StepSpec{{ID: "step", Uses: "plugin-a"}},
	}}

	set, err := CompileSpecs(specs)
	if err != nil {
		t.Fatalf("CompileSpecs() error = %v", err)
	}
	p := set.Pipelines["regular"]
	if p.IsHook {
		t.Fatalf("IsHook = true for regular pipeline, want false")
	}
}

func hasEdge(edges []Edge, from, to string) bool {
	for _, edge := range edges {
		if edge.From == from && edge.To == to {
			return true
		}
	}
	return false
}
