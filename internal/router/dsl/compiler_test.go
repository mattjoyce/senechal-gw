package dsl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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

func TestLoadAndCompileDirLoadsMultipleYAMLFiles(t *testing.T) {
	configDir := t.TempDir()
	pipelinesDir := filepath.Join(configDir, "pipelines")
	if err := os.MkdirAll(pipelinesDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(pipelines): %v", err)
	}

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

	if err := os.WriteFile(filepath.Join(pipelinesDir, "01-root.yaml"), []byte(rootYAML), 0o644); err != nil {
		t.Fatalf("WriteFile(01-root.yaml): %v", err)
	}
	if err := os.WriteFile(filepath.Join(pipelinesDir, "02-process.yaml"), []byte(processYAML), 0o644); err != nil {
		t.Fatalf("WriteFile(02-process.yaml): %v", err)
	}
	if err := os.WriteFile(filepath.Join(pipelinesDir, "README.txt"), []byte("ignored"), 0o644); err != nil {
		t.Fatalf("WriteFile(README.txt): %v", err)
	}

	set, err := LoadAndCompileDir(configDir)
	if err != nil {
		t.Fatalf("LoadAndCompileDir() error = %v", err)
	}
	if len(set.Pipelines) != 2 {
		t.Fatalf("pipeline count = %d, want 2", len(set.Pipelines))
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
