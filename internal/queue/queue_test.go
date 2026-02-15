package queue

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/mattjoyce/ductile/internal/storage"
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
	contextID := "ctx-123"

	id, err := q.Enqueue(context.Background(), EnqueueRequest{
		Plugin:         "echo",
		Command:        "poll",
		SubmittedBy:    "scheduler",
		EventContextID: &contextID,
	})
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if _, err := q.Dequeue(context.Background()); err != nil {
		t.Fatalf("Dequeue: %v", err)
	}

	stderr := "hello stderr"
	lastErr := "boom"
	result := json.RawMessage(`{"status":"error","error":"boom"}`)
	if err := q.CompleteWithResult(context.Background(), id, StatusFailed, result, &lastErr, &stderr); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM job_log WHERE plugin='echo';").Scan(&count); err != nil {
		t.Fatalf("count job_log: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 job_log row, got %d", count)
	}

	var gotContextID string
	if err := db.QueryRow("SELECT event_context_id FROM job_log WHERE id = ?;", id+"-1").Scan(&gotContextID); err != nil {
		t.Fatalf("select event_context_id: %v", err)
	}
	if gotContextID != contextID {
		t.Fatalf("event_context_id: got %q want %q", gotContextID, contextID)
	}
}

func TestGetJobByID(t *testing.T) {
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

	result := json.RawMessage(`{"status":"ok","logs":[]}`)
	if err := q.CompleteWithResult(context.Background(), id, StatusSucceeded, result, nil, nil); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	got, err := q.GetJobByID(context.Background(), id)
	if err != nil {
		t.Fatalf("GetJobByID: %v", err)
	}
	if got.JobID != id {
		t.Fatalf("JobID: got %q want %q", got.JobID, id)
	}
	if got.Status != StatusSucceeded {
		t.Fatalf("Status: got %q want %q", got.Status, StatusSucceeded)
	}
	if got.Plugin != "echo" || got.Command != "poll" {
		t.Fatalf("unexpected plugin/command: %#v", got)
	}
	if string(got.Result) != string(result) {
		t.Fatalf("Result: got %s want %s", string(got.Result), string(result))
	}
	if got.StartedAt == nil {
		t.Fatalf("expected StartedAt to be set")
	}
	if got.CompletedAt == nil {
		t.Fatalf("expected CompletedAt to be set")
	}
}

func TestGetJobByIDNotFound(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "state.db")
	db, err := storage.OpenSQLite(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	q := New(db)

	if _, err := q.GetJobByID(context.Background(), "nope"); err != ErrJobNotFound {
		t.Fatalf("expected ErrJobNotFound, got %v", err)
	}
}

func TestQueueCountOutstandingPollJobs(t *testing.T) {
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
		t.Fatalf("enqueue: %v", err)
	}

	n, err := q.CountOutstandingPollJobs(context.Background(), "echo")
	if err != nil {
		t.Fatalf("CountOutstandingPollJobs queued: %v", err)
	}
	if n != 1 {
		t.Fatalf("outstanding queued count=%d, want 1", n)
	}

	if _, err := q.Dequeue(context.Background()); err != nil {
		t.Fatalf("dequeue: %v", err)
	}
	n, err = q.CountOutstandingPollJobs(context.Background(), "echo")
	if err != nil {
		t.Fatalf("CountOutstandingPollJobs running: %v", err)
	}
	if n != 1 {
		t.Fatalf("outstanding running count=%d, want 1", n)
	}

	if err := q.CompleteWithResult(context.Background(), id, StatusSucceeded, json.RawMessage(`{"status":"ok"}`), nil, nil); err != nil {
		t.Fatalf("complete: %v", err)
	}
	n, err = q.CountOutstandingPollJobs(context.Background(), "echo")
	if err != nil {
		t.Fatalf("CountOutstandingPollJobs completed: %v", err)
	}
	if n != 0 {
		t.Fatalf("outstanding completed count=%d, want 0", n)
	}
}

func TestQueueLatestCompletedPollResult(t *testing.T) {
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
		SubmittedBy: "ductile",
	})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if _, err := q.Dequeue(context.Background()); err != nil {
		t.Fatalf("dequeue: %v", err)
	}
	lastErr := "boom"
	if err := q.CompleteWithResult(context.Background(), id, StatusFailed, json.RawMessage(`{"status":"error"}`), &lastErr, nil); err != nil {
		t.Fatalf("complete: %v", err)
	}

	res, err := q.LatestCompletedPollResult(context.Background(), "echo", "ductile")
	if err != nil {
		t.Fatalf("LatestCompletedPollResult: %v", err)
	}
	if res == nil {
		t.Fatal("expected latest completed poll result")
	}
	if res.JobID != id {
		t.Fatalf("job id=%q want %q", res.JobID, id)
	}
	if res.Status != StatusFailed {
		t.Fatalf("status=%q want %q", res.Status, StatusFailed)
	}
	if res.CompletedAt.IsZero() {
		t.Fatal("expected completed_at timestamp")
	}

	res, err = q.LatestCompletedPollResult(context.Background(), "echo", "scheduler")
	if err != nil {
		t.Fatalf("LatestCompletedPollResult (no match): %v", err)
	}
	if res != nil {
		t.Fatalf("expected nil for unmatched submitted_by, got %#v", res)
	}
}

func TestQueueCircuitBreakerRoundTripAndReset(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "state.db")
	db, err := storage.OpenSQLite(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	q := New(db)

	got, err := q.GetCircuitBreaker(context.Background(), "echo", "poll")
	if err != nil {
		t.Fatalf("GetCircuitBreaker initial: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil initial breaker, got %#v", got)
	}

	openedAt := time.Now().UTC().Add(-2 * time.Minute)
	lastFailure := time.Now().UTC().Add(-1 * time.Minute)
	lastJobID := "job-123"
	if err := q.UpsertCircuitBreaker(context.Background(), CircuitBreaker{
		Plugin:       "echo",
		Command:      "poll",
		State:        CircuitOpen,
		FailureCount: 3,
		OpenedAt:     &openedAt,
		LastFailure:  &lastFailure,
		LastJobID:    &lastJobID,
	}); err != nil {
		t.Fatalf("UpsertCircuitBreaker: %v", err)
	}

	got, err = q.GetCircuitBreaker(context.Background(), "echo", "poll")
	if err != nil {
		t.Fatalf("GetCircuitBreaker after upsert: %v", err)
	}
	if got == nil {
		t.Fatal("expected breaker row after upsert")
	}
	if got.State != CircuitOpen {
		t.Fatalf("state=%q want %q", got.State, CircuitOpen)
	}
	if got.FailureCount != 3 {
		t.Fatalf("failure_count=%d want 3", got.FailureCount)
	}
	if got.LastJobID == nil || *got.LastJobID != lastJobID {
		t.Fatalf("last_job_id=%v want %q", got.LastJobID, lastJobID)
	}
	if got.OpenedAt == nil || got.LastFailure == nil {
		t.Fatalf("expected opened_at and last_failure to be set, got %#v", got)
	}

	if err := q.ResetCircuitBreaker(context.Background(), "echo", "poll"); err != nil {
		t.Fatalf("ResetCircuitBreaker: %v", err)
	}
	got, err = q.GetCircuitBreaker(context.Background(), "echo", "poll")
	if err != nil {
		t.Fatalf("GetCircuitBreaker after reset: %v", err)
	}
	if got == nil {
		t.Fatal("expected breaker row after reset")
	}
	if got.State != CircuitClosed {
		t.Fatalf("state=%q want %q", got.State, CircuitClosed)
	}
	if got.FailureCount != 0 {
		t.Fatalf("failure_count=%d want 0", got.FailureCount)
	}
	if got.OpenedAt != nil || got.LastFailure != nil || got.LastJobID != nil {
		t.Fatalf("expected cleared breaker timing fields after reset, got %#v", got)
	}
}
