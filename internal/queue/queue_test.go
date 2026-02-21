package queue

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
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

func TestListJobs_FilterSortAndLimit(t *testing.T) {
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
		SubmittedBy: "api",
	})
	if err != nil {
		t.Fatalf("enqueue id1: %v", err)
	}
	id2, err := q.Enqueue(context.Background(), EnqueueRequest{
		Plugin:      "echo",
		Command:     "handle",
		SubmittedBy: "api",
	})
	if err != nil {
		t.Fatalf("enqueue id2: %v", err)
	}
	id3, err := q.Enqueue(context.Background(), EnqueueRequest{
		Plugin:      "withings",
		Command:     "poll",
		SubmittedBy: "scheduler",
	})
	if err != nil {
		t.Fatalf("enqueue id3: %v", err)
	}

	if _, err := db.Exec(`
UPDATE job_queue SET status = ?, attempt = 1, created_at = ? WHERE id = ?;
`, StatusQueued, "2026-02-21T10:00:00Z", id1); err != nil {
		t.Fatalf("update id1: %v", err)
	}
	if _, err := db.Exec(`
UPDATE job_queue SET status = ?, attempt = 2, created_at = ?, started_at = ? WHERE id = ?;
`, StatusRunning, "2026-02-21T10:01:00Z", "2026-02-21T10:01:01Z", id2); err != nil {
		t.Fatalf("update id2: %v", err)
	}
	if _, err := db.Exec(`
UPDATE job_queue SET status = ?, attempt = 3, created_at = ?, started_at = ?, completed_at = ? WHERE id = ?;
`, StatusFailed, "2026-02-21T10:02:00Z", "2026-02-21T10:02:01Z", "2026-02-21T10:02:02Z", id3); err != nil {
		t.Fatalf("update id3: %v", err)
	}

	echoJobs, totalEcho, err := q.ListJobs(context.Background(), ListJobsFilter{
		Plugin: "echo",
		Limit:  1,
	})
	if err != nil {
		t.Fatalf("ListJobs echo: %v", err)
	}
	if totalEcho != 2 {
		t.Fatalf("totalEcho=%d want 2", totalEcho)
	}
	if len(echoJobs) != 1 {
		t.Fatalf("len(echoJobs)=%d want 1", len(echoJobs))
	}
	if echoJobs[0].JobID != id2 {
		t.Fatalf("first echo job id=%s want %s", echoJobs[0].JobID, id2)
	}

	status := StatusRunning
	runningJobs, totalRunning, err := q.ListJobs(context.Background(), ListJobsFilter{
		Status: &status,
		Limit:  50,
	})
	if err != nil {
		t.Fatalf("ListJobs running: %v", err)
	}
	if totalRunning != 1 {
		t.Fatalf("totalRunning=%d want 1", totalRunning)
	}
	if len(runningJobs) != 1 || runningJobs[0].JobID != id2 {
		t.Fatalf("unexpected running jobs: %+v", runningJobs)
	}

	allJobs, totalAll, err := q.ListJobs(context.Background(), ListJobsFilter{})
	if err != nil {
		t.Fatalf("ListJobs all: %v", err)
	}
	if totalAll != 3 {
		t.Fatalf("totalAll=%d want 3", totalAll)
	}
	if len(allJobs) != 3 {
		t.Fatalf("len(allJobs)=%d want 3", len(allJobs))
	}
	if allJobs[0].JobID != id3 || allJobs[1].JobID != id2 || allJobs[2].JobID != id1 {
		t.Fatalf("unexpected sort order: [%s %s %s]", allJobs[0].JobID, allJobs[1].JobID, allJobs[2].JobID)
	}
}

func TestQueueDequeueRespectsNextRetryAtStrictly(t *testing.T) {
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

	future := time.Now().UTC().Add(2 * time.Minute)
	if err := q.UpdateJobForRecovery(context.Background(), id, StatusQueued, 2, &future, "temporary"); err != nil {
		t.Fatalf("UpdateJobForRecovery future: %v", err)
	}

	job, err := q.Dequeue(context.Background())
	if err != nil {
		t.Fatalf("dequeue before next_retry_at: %v", err)
	}
	if job != nil {
		t.Fatalf("expected no dequeued job before next_retry_at, got %+v", job)
	}

	past := time.Now().UTC().Add(-2 * time.Minute)
	if err := q.UpdateJobForRecovery(context.Background(), id, StatusQueued, 2, &past, "temporary"); err != nil {
		t.Fatalf("UpdateJobForRecovery past: %v", err)
	}

	job, err = q.Dequeue(context.Background())
	if err != nil {
		t.Fatalf("dequeue after next_retry_at: %v", err)
	}
	if job == nil {
		t.Fatal("expected job dequeue after next_retry_at passed")
	}
	if job.ID != id {
		t.Fatalf("dequeued job id = %s, want %s", job.ID, id)
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

func TestQueueEnqueueDedupeMissBeforeSuccess(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "state.db")
	db, err := storage.OpenSQLite(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	q := New(db, WithLogger(slog.Default()), WithDedupeTTL(24*time.Hour))
	key := "poll:echo"

	firstID, err := q.Enqueue(context.Background(), EnqueueRequest{
		Plugin:      "echo",
		Command:     "poll",
		SubmittedBy: "scheduler",
		DedupeKey:   &key,
	})
	if err != nil {
		t.Fatalf("first enqueue: %v", err)
	}
	if firstID == "" {
		t.Fatal("first enqueue returned empty job id")
	}

	secondID, err := q.Enqueue(context.Background(), EnqueueRequest{
		Plugin:      "echo",
		Command:     "poll",
		SubmittedBy: "scheduler",
		DedupeKey:   &key,
	})
	if err != nil {
		t.Fatalf("second enqueue before success should not dedupe: %v", err)
	}
	if secondID == "" {
		t.Fatal("second enqueue returned empty job id")
	}
	if secondID == firstID {
		t.Fatalf("expected distinct job IDs, got duplicate %s", secondID)
	}
}

func TestQueueEnqueueDedupeHitAfterSuccess(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "state.db")
	db, err := storage.OpenSQLite(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	q := New(db, WithLogger(slog.Default()), WithDedupeTTL(24*time.Hour))
	key := "poll:echo"

	firstID, err := q.Enqueue(context.Background(), EnqueueRequest{
		Plugin:      "echo",
		Command:     "poll",
		SubmittedBy: "scheduler",
		DedupeKey:   &key,
	})
	if err != nil {
		t.Fatalf("first enqueue: %v", err)
	}
	if _, err := q.Dequeue(context.Background()); err != nil {
		t.Fatalf("dequeue: %v", err)
	}
	if err := q.CompleteWithResult(context.Background(), firstID, StatusSucceeded, json.RawMessage(`{"status":"ok"}`), nil, nil); err != nil {
		t.Fatalf("complete success: %v", err)
	}

	dupID, err := q.Enqueue(context.Background(), EnqueueRequest{
		Plugin:      "echo",
		Command:     "poll",
		SubmittedBy: "scheduler",
		DedupeKey:   &key,
	})
	if err == nil {
		t.Fatalf("expected dedupe drop error, got nil with id=%q", dupID)
	}
	var dedupeErr *DedupeDropError
	if !errors.As(err, &dedupeErr) {
		t.Fatalf("expected DedupeDropError, got %T: %v", err, err)
	}
	if dedupeErr.ExistingJobID != firstID {
		t.Fatalf("existing job id = %q, want %q", dedupeErr.ExistingJobID, firstID)
	}

	var queuedCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM job_queue WHERE status = ?`, StatusQueued).Scan(&queuedCount); err != nil {
		t.Fatalf("count queued jobs: %v", err)
	}
	if queuedCount != 0 {
		t.Fatalf("expected no queued jobs after dedupe hit, got %d", queuedCount)
	}
}

func TestQueueEnqueueDedupeTTLExpiredAllowsNewJob(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "state.db")
	db, err := storage.OpenSQLite(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	q := New(db, WithLogger(slog.Default()), WithDedupeTTL(24*time.Hour))
	key := "poll:echo"

	firstID, err := q.Enqueue(context.Background(), EnqueueRequest{
		Plugin:      "echo",
		Command:     "poll",
		SubmittedBy: "scheduler",
		DedupeKey:   &key,
	})
	if err != nil {
		t.Fatalf("first enqueue: %v", err)
	}
	if _, err := q.Dequeue(context.Background()); err != nil {
		t.Fatalf("dequeue: %v", err)
	}
	if err := q.CompleteWithResult(context.Background(), firstID, StatusSucceeded, json.RawMessage(`{"status":"ok"}`), nil, nil); err != nil {
		t.Fatalf("complete success: %v", err)
	}

	expiredCompletedAt := time.Now().UTC().Add(-48 * time.Hour).Format(time.RFC3339Nano)
	if _, err := db.Exec(`UPDATE job_queue SET completed_at = ? WHERE id = ?`, expiredCompletedAt, firstID); err != nil {
		t.Fatalf("expire completed_at: %v", err)
	}

	secondID, err := q.Enqueue(context.Background(), EnqueueRequest{
		Plugin:      "echo",
		Command:     "poll",
		SubmittedBy: "scheduler",
		DedupeKey:   &key,
	})
	if err != nil {
		t.Fatalf("enqueue after ttl expiry should succeed: %v", err)
	}
	if secondID == "" {
		t.Fatal("second enqueue returned empty job id")
	}
}
