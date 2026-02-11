package inspect

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mattjoyce/senechal-gw/internal/queue"
	"github.com/mattjoyce/senechal-gw/internal/state"
	"github.com/mattjoyce/senechal-gw/internal/storage"
)

func TestBuildReportRendersLineageAndArtifacts(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "state.db")
	db, err := storage.OpenSQLite(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()
	ctxStore := state.NewContextStore(db)
	q := queue.New(db)

	rootCtx, err := ctxStore.Create(ctx, nil, "chain", "step_a", json.RawMessage(`{"origin_channel_id":"chan-1","message":"hello"}`))
	if err != nil {
		t.Fatalf("Create(root context): %v", err)
	}
	rootJobID, err := q.Enqueue(ctx, queue.EnqueueRequest{
		Plugin:         "plugin-a",
		Command:        "poll",
		SubmittedBy:    "test",
		EventContextID: &rootCtx.ID,
	})
	if err != nil {
		t.Fatalf("Enqueue(root): %v", err)
	}

	childCtx, err := ctxStore.Create(ctx, &rootCtx.ID, "chain", "step_b", json.RawMessage(`{"message":"goodbye"}`))
	if err != nil {
		t.Fatalf("Create(child context): %v", err)
	}
	childJobID, err := q.Enqueue(ctx, queue.EnqueueRequest{
		Plugin:         "plugin-b",
		Command:        "handle",
		SubmittedBy:    "route",
		ParentJobID:    &rootJobID,
		EventContextID: &childCtx.ID,
	})
	if err != nil {
		t.Fatalf("Enqueue(child): %v", err)
	}

	workspaceBase := filepath.Join(tmpDir, "workspaces")
	if err := os.MkdirAll(filepath.Join(workspaceBase, rootJobID), 0o755); err != nil {
		t.Fatalf("MkdirAll(root workspace): %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspaceBase, rootJobID, "root.txt"), []byte("root"), 0o644); err != nil {
		t.Fatalf("WriteFile(root artifact): %v", err)
	}
	if err := os.MkdirAll(filepath.Join(workspaceBase, childJobID, "nested"), 0o755); err != nil {
		t.Fatalf("MkdirAll(child workspace): %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspaceBase, childJobID, "nested", "child.txt"), []byte("child"), 0o644); err != nil {
		t.Fatalf("WriteFile(child artifact): %v", err)
	}

	out, err := BuildReport(ctx, db, dbPath, childJobID)
	if err != nil {
		t.Fatalf("BuildReport: %v", err)
	}

	for _, needle := range []string{
		"Lineage Report",
		"chain :: step_a",
		"chain :: step_b",
		"origin_channel_id",
		"chan-1",
		"root.txt",
		"nested/child.txt",
		childJobID,
	} {
		if !strings.Contains(out, needle) {
			t.Fatalf("output missing %q:\n%s", needle, out)
		}
	}
}

func TestBuildJSONReport(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "state.db")
	db, err := storage.OpenSQLite(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()
	ctxStore := state.NewContextStore(db)
	q := queue.New(db)

	rootCtx, _ := ctxStore.Create(ctx, nil, "chain", "step_a", json.RawMessage(`{"origin":"test"}`))
	rootJobID, _ := q.Enqueue(ctx, queue.EnqueueRequest{
		Plugin:         "p",
		Command:        "c",
		SubmittedBy:    "test",
		EventContextID: &rootCtx.ID,
	})

	out, err := BuildJSONReport(ctx, db, dbPath, rootJobID)
	if err != nil {
		t.Fatalf("BuildJSONReport: %v", err)
	}

	var report Report
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("failed to unmarshal JSON output: %v", err)
	}

	if report.JobID != rootJobID {
		t.Errorf("job_id = %s, want %s", report.JobID, rootJobID)
	}
	if len(report.Steps) != 1 {
		t.Errorf("expected 1 step, got %d", len(report.Steps))
	}
	if report.Steps[0].StepID != "step_a" {
		t.Errorf("step_id = %s, want %s", report.Steps[0].StepID, "step_a")
	}
}
