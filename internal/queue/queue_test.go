package queue

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/mattjoyce/senechal-gw/internal/storage"
)

func TestQueueEnqueueDequeueFIFO(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "state.db")
	db, err := storage.OpenSQLite(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	q := New(db)

	id1, err := q.Enqueue(context.Background(), EnqueueRequest{
		Plugin:      "echo",
		Command:     "poll",
		SubmittedBy: "scheduler",
	})
	if err != nil {
		t.Fatalf("Enqueue 1: %v", err)
	}
	id2, err := q.Enqueue(context.Background(), EnqueueRequest{
		Plugin:      "echo",
		Command:     "poll",
		SubmittedBy: "scheduler",
	})
	if err != nil {
		t.Fatalf("Enqueue 2: %v", err)
	}

	j1, err := q.Dequeue(context.Background())
	if err != nil {
		t.Fatalf("Dequeue 1: %v", err)
	}
	if j1 == nil || j1.ID != id1 || j1.Status != StatusRunning || j1.StartedAt == nil {
		t.Fatalf("unexpected job1: %#v", j1)
	}

	j2, err := q.Dequeue(context.Background())
	if err != nil {
		t.Fatalf("Dequeue 2: %v", err)
	}
	if j2 == nil || j2.ID != id2 {
		t.Fatalf("unexpected job2: %#v", j2)
	}

	j3, err := q.Dequeue(context.Background())
	if err != nil {
		t.Fatalf("Dequeue 3: %v", err)
	}
	if j3 != nil {
		t.Fatalf("expected empty queue, got %#v", j3)
	}
}

func TestQueueCompleteWritesJobLog(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "state.db")
	db, err := storage.OpenSQLite(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	q := New(db)

	id, err := q.Enqueue(context.Background(), EnqueueRequest{
		Plugin:      "echo",
		Command:     "poll",
		SubmittedBy: "scheduler",
	})
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if _, err := q.Dequeue(context.Background()); err != nil {
		t.Fatalf("Dequeue: %v", err)
	}

	stderr := "hello stderr"
	lastErr := "boom"
	if err := q.Complete(context.Background(), id, StatusFailed, &lastErr, &stderr); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM job_log WHERE plugin='echo';").Scan(&count); err != nil {
		t.Fatalf("count job_log: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 job_log row, got %d", count)
	}
}
