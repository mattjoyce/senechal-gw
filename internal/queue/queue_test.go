package queue

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/mattjoyce/ductile/internal/storage"
)

func statusPtr(status Status) *Status {
	return &status
}

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

	var (
		gotContextID string
		gotJobID     string
	)
	if err := db.QueryRow("SELECT event_context_id, job_id FROM job_log WHERE id = ?;", id+"-1").Scan(&gotContextID, &gotJobID); err != nil {
		t.Fatalf("select event_context_id/job_id: %v", err)
	}
	if gotContextID != contextID {
		t.Fatalf("event_context_id: got %q want %q", gotContextID, contextID)
	}
	if gotJobID != id {
		t.Fatalf("job_id: got %q want %q", gotJobID, id)
	}
}

func TestQueueRecordsConfigSnapshotIDs(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "state.db")
	db, err := storage.OpenSQLite(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()
	q := New(db, WithConfigSnapshotIDProvider(func() string { return "cfg_runtime" }))

	id, err := q.Enqueue(ctx, EnqueueRequest{
		Plugin:      "echo",
		Command:     "poll",
		SubmittedBy: "scheduler",
	})
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	var enqueuedID string
	if err := db.QueryRowContext(ctx, "SELECT enqueued_config_snapshot_id FROM job_queue WHERE id = ?;", id).Scan(&enqueuedID); err != nil {
		t.Fatalf("select enqueued snapshot: %v", err)
	}
	if enqueuedID != "cfg_runtime" {
		t.Fatalf("enqueued_config_snapshot_id = %q, want cfg_runtime", enqueuedID)
	}

	job, err := q.Dequeue(ctx)
	if err != nil {
		t.Fatalf("Dequeue: %v", err)
	}
	if job == nil {
		t.Fatal("expected dequeued job")
	}
	if job.EnqueuedConfigSnapshotID == nil || *job.EnqueuedConfigSnapshotID != "cfg_runtime" {
		t.Fatalf("job enqueued snapshot = %v", job.EnqueuedConfigSnapshotID)
	}
	if job.StartedConfigSnapshotID == nil || *job.StartedConfigSnapshotID != "cfg_runtime" {
		t.Fatalf("job started snapshot = %v", job.StartedConfigSnapshotID)
	}

	if err := q.Complete(ctx, id, StatusSucceeded, nil, nil); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	var logEnqueuedID, logStartedID string
	if err := db.QueryRowContext(ctx, `
SELECT enqueued_config_snapshot_id, started_config_snapshot_id
FROM job_log
WHERE job_id = ?;
`, id).Scan(&logEnqueuedID, &logStartedID); err != nil {
		t.Fatalf("select log snapshots: %v", err)
	}
	if logEnqueuedID != "cfg_runtime" || logStartedID != "cfg_runtime" {
		t.Fatalf("job_log snapshots = %q/%q, want cfg_runtime/cfg_runtime", logEnqueuedID, logStartedID)
	}
}

func TestQueueRecordsJobLineageLifecycle(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "state.db")
	db, err := storage.OpenSQLite(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()
	q := New(db)

	id, err := q.Enqueue(ctx, EnqueueRequest{
		Plugin:      "echo",
		Command:     "poll",
		SubmittedBy: "scheduler",
	})
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	first, err := q.Dequeue(ctx)
	if err != nil {
		t.Fatalf("Dequeue first: %v", err)
	}
	if first == nil {
		t.Fatal("expected first dequeue")
	}

	retryReason := "temporary failure"
	if err := q.UpdateJobForRecovery(ctx, id, StatusQueued, 2, nil, retryReason); err != nil {
		t.Fatalf("UpdateJobForRecovery: %v", err)
	}

	second, err := q.Dequeue(ctx)
	if err != nil {
		t.Fatalf("Dequeue second: %v", err)
	}
	if second == nil {
		t.Fatal("expected second dequeue")
	}
	if second.Attempt != 2 {
		t.Fatalf("second attempt=%d want 2", second.Attempt)
	}

	if err := q.CompleteWithResult(ctx, id, StatusSucceeded, json.RawMessage(`{"status":"ok"}`), nil, nil); err != nil {
		t.Fatalf("CompleteWithResult: %v", err)
	}

	lineage, err := q.GetJobLineage(ctx, id)
	if err != nil {
		t.Fatalf("GetJobLineage: %v", err)
	}

	wantTransitions := []struct {
		from *Status
		to   Status
	}{
		{nil, StatusQueued},
		{statusPtr(StatusQueued), StatusRunning},
		{statusPtr(StatusRunning), StatusQueued},
		{statusPtr(StatusQueued), StatusRunning},
		{statusPtr(StatusRunning), StatusSucceeded},
	}
	if len(lineage.Transitions) != len(wantTransitions) {
		t.Fatalf("transitions len=%d want %d: %+v", len(lineage.Transitions), len(wantTransitions), lineage.Transitions)
	}
	for i, want := range wantTransitions {
		got := lineage.Transitions[i]
		if want.from == nil {
			if got.FromStatus != nil {
				t.Fatalf("transition %d from=%v want nil", i, *got.FromStatus)
			}
		} else if got.FromStatus == nil || *got.FromStatus != *want.from {
			t.Fatalf("transition %d from=%v want %v", i, got.FromStatus, *want.from)
		}
		if got.ToStatus != want.to {
			t.Fatalf("transition %d to=%q want %q", i, got.ToStatus, want.to)
		}
	}
	if lineage.Transitions[2].Reason == nil || *lineage.Transitions[2].Reason != retryReason {
		t.Fatalf("retry transition reason=%v want %q", lineage.Transitions[2].Reason, retryReason)
	}

	if len(lineage.Attempts) != 2 {
		t.Fatalf("attempts len=%d want 2: %+v", len(lineage.Attempts), lineage.Attempts)
	}
	if lineage.Attempts[0].Attempt != 1 || lineage.Attempts[1].Attempt != 2 {
		t.Fatalf("attempt facts=%+v want attempts 1,2", lineage.Attempts)
	}
	if lineage.CachedStatus != StatusSucceeded {
		t.Fatalf("cached status=%q want %q", lineage.CachedStatus, StatusSucceeded)
	}
	if lineage.CachedAttempt != 2 {
		t.Fatalf("cached attempt=%d want 2", lineage.CachedAttempt)
	}
	if !lineage.StatusMatchesLatest {
		t.Fatal("expected cached status to match latest transition")
	}
	if !lineage.AttemptFactsMatch {
		t.Fatal("expected cached attempt to match latest attempt fact")
	}
	if lineage.HasLegacyMissingData {
		t.Fatal("did not expect legacy missing data")
	}
}

func TestQueueLineageToleratesLegacyJobWithoutFacts(t *testing.T) {
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
	if _, err := db.Exec("DELETE FROM job_transitions WHERE job_id = ?", id); err != nil {
		t.Fatalf("delete transitions: %v", err)
	}

	lineage, err := q.GetJobLineage(context.Background(), id)
	if err != nil {
		t.Fatalf("GetJobLineage: %v", err)
	}
	if len(lineage.Transitions) != 0 {
		t.Fatalf("transitions len=%d want 0", len(lineage.Transitions))
	}
	if !lineage.HasLegacyMissingData {
		t.Fatal("expected legacy missing data")
	}
	if lineage.LatestStatus != nil {
		t.Fatalf("latest status=%q want nil", *lineage.LatestStatus)
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

	result := json.RawMessage(`{"status":"ok","result":"ok","logs":[]}`)
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

func TestGetJobByIDUsesJobLogJobIDOnRetryAttempts(t *testing.T) {
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
	if _, err := db.Exec(`UPDATE job_queue SET attempt = 2 WHERE id = ?`, id); err != nil {
		t.Fatalf("update attempt: %v", err)
	}
	if _, err := q.Dequeue(context.Background()); err != nil {
		t.Fatalf("Dequeue: %v", err)
	}

	result := json.RawMessage(`{"status":"ok","result":"retry-ok","logs":[]}`)
	if err := q.CompleteWithResult(context.Background(), id, StatusSucceeded, result, nil, nil); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	got, err := q.GetJobByID(context.Background(), id)
	if err != nil {
		t.Fatalf("GetJobByID: %v", err)
	}
	if string(got.Result) != string(result) {
		t.Fatalf("Result: got %s want %s", string(got.Result), string(result))
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

func TestListJobLogsFilters(t *testing.T) {
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
		SubmittedBy: "api",
	})
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if _, err := q.Dequeue(context.Background()); err != nil {
		t.Fatalf("Dequeue: %v", err)
	}

	stderr := "stderr output"
	lastErr := "Boom"
	result := json.RawMessage(`{"status":"error","error":"Boom"}`)
	if err := q.CompleteWithResult(context.Background(), id, StatusFailed, result, &lastErr, &stderr); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	since := time.Now().UTC().Add(-1 * time.Minute)
	until := time.Now().UTC().Add(1 * time.Minute)
	status := StatusFailed

	filter := JobLogFilter{
		Plugin:        "echo",
		Status:        &status,
		Query:         "boom",
		Since:         &since,
		Until:         &until,
		Limit:         10,
		IncludeResult: true,
	}

	logs, total, err := q.ListJobLogs(context.Background(), filter)
	if err != nil {
		t.Fatalf("ListJobLogs: %v", err)
	}
	if total != 1 || len(logs) != 1 {
		t.Fatalf("expected 1 log, got total=%d len=%d", total, len(logs))
	}
	if logs[0].JobID != id {
		t.Fatalf("job_id mismatch: got %q want %q", logs[0].JobID, id)
	}
	if logs[0].LastError == nil || *logs[0].LastError != lastErr {
		t.Fatalf("last_error mismatch: %#v", logs[0].LastError)
	}
	if logs[0].Stderr == nil || *logs[0].Stderr != stderr {
		t.Fatalf("stderr mismatch: %#v", logs[0].Stderr)
	}
	if string(logs[0].Result) != string(result) {
		t.Fatalf("result mismatch: got %s want %s", string(logs[0].Result), string(result))
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
	n, err = q.CountOutstandingJobs(context.Background(), "echo", "poll")
	if err != nil {
		t.Fatalf("CountOutstandingJobs queued: %v", err)
	}
	if n != 1 {
		t.Fatalf("generic outstanding queued count=%d, want 1", n)
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
	n, err = q.CountOutstandingJobs(context.Background(), "echo", "poll")
	if err != nil {
		t.Fatalf("CountOutstandingJobs running: %v", err)
	}
	if n != 1 {
		t.Fatalf("generic outstanding running count=%d, want 1", n)
	}

	if err := q.CompleteWithResult(context.Background(), id, StatusSucceeded, json.RawMessage(`{"status":"ok","result":"ok"}`), nil, nil); err != nil {
		t.Fatalf("complete: %v", err)
	}
	n, err = q.CountOutstandingPollJobs(context.Background(), "echo")
	if err != nil {
		t.Fatalf("CountOutstandingPollJobs completed: %v", err)
	}
	if n != 0 {
		t.Fatalf("outstanding completed count=%d, want 0", n)
	}
	n, err = q.CountOutstandingJobs(context.Background(), "echo", "poll")
	if err != nil {
		t.Fatalf("CountOutstandingJobs completed: %v", err)
	}
	if n != 0 {
		t.Fatalf("generic outstanding completed count=%d, want 0", n)
	}
}

func TestQueueCancelOutstandingJobs(t *testing.T) {
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
	if _, err := q.Dequeue(context.Background()); err != nil {
		t.Fatalf("dequeue: %v", err)
	}

	cancelled, err := q.CancelOutstandingJobs(context.Background(), "echo", "poll", "cancelled by test")
	if err != nil {
		t.Fatalf("CancelOutstandingJobs: %v", err)
	}
	if cancelled != 1 {
		t.Fatalf("cancelled=%d want 1", cancelled)
	}

	var (
		status    string
		lastError sql.NullString
		completed sql.NullString
	)
	if err := db.QueryRow(`SELECT status, last_error, completed_at FROM job_queue WHERE id = ?`, id).Scan(&status, &lastError, &completed); err != nil {
		t.Fatalf("query cancelled job: %v", err)
	}
	if status != string(StatusDead) {
		t.Fatalf("status=%q want %q", status, StatusDead)
	}
	if !lastError.Valid || lastError.String != "cancelled by test" {
		t.Fatalf("last_error=%v want %q", lastError, "cancelled by test")
	}
	if !completed.Valid || completed.String == "" {
		t.Fatalf("completed_at=%v want non-empty", completed)
	}

	lineage, err := q.GetJobLineage(context.Background(), id)
	if err != nil {
		t.Fatalf("GetJobLineage: %v", err)
	}
	if len(lineage.Transitions) != 3 {
		t.Fatalf("transitions len=%d want 3: %+v", len(lineage.Transitions), lineage.Transitions)
	}
	last := lineage.Transitions[len(lineage.Transitions)-1]
	if last.FromStatus == nil || *last.FromStatus != StatusRunning || last.ToStatus != StatusDead {
		t.Fatalf("last transition=%+v want running -> dead", last)
	}
	if last.Reason == nil || *last.Reason != "cancelled by test" {
		t.Fatalf("last reason=%v want cancelled by test", last.Reason)
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

	generic, err := q.LatestCompletedCommandResult(context.Background(), "echo", "poll", "ductile")
	if err != nil {
		t.Fatalf("LatestCompletedCommandResult: %v", err)
	}
	if generic == nil {
		t.Fatal("expected latest completed command result")
	}
	if generic.JobID != id {
		t.Fatalf("generic job id=%q want %q", generic.JobID, id)
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

	transitions, err := q.ListCircuitBreakerTransitions(context.Background(), "echo", "poll", 10)
	if err != nil {
		t.Fatalf("ListCircuitBreakerTransitions: %v", err)
	}
	if len(transitions) != 1 {
		t.Fatalf("transitions len=%d want 1", len(transitions))
	}
	if transitions[0].Reason != CircuitTransitionManualReset {
		t.Fatalf("transition reason=%q want %q", transitions[0].Reason, CircuitTransitionManualReset)
	}
	if transitions[0].FromState == nil || *transitions[0].FromState != CircuitOpen {
		t.Fatalf("from_state=%v want %q", transitions[0].FromState, CircuitOpen)
	}
	if transitions[0].ToState != CircuitClosed {
		t.Fatalf("to_state=%q want %q", transitions[0].ToState, CircuitClosed)
	}
}

func TestQueueCircuitBreakerTransitionsAreAppendOnly(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "state.db")
	db, err := storage.OpenSQLite(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	q := New(db)
	ctx := context.Background()
	from := CircuitClosed
	jobID := "job-open"
	if err := q.RecordCircuitBreakerTransition(ctx, CircuitBreakerTransition{
		Plugin:       "echo",
		Command:      "poll",
		FromState:    &from,
		ToState:      CircuitOpen,
		FailureCount: 3,
		Reason:       CircuitTransitionFailureThreshold,
		JobID:        &jobID,
	}); err != nil {
		t.Fatalf("RecordCircuitBreakerTransition open: %v", err)
	}
	if err := q.RecordCircuitBreakerTransition(ctx, CircuitBreakerTransition{
		Plugin:       "echo",
		Command:      "poll",
		FromState:    nil,
		ToState:      CircuitHalfOpen,
		FailureCount: 3,
		Reason:       CircuitTransitionCooldownElapsed,
	}); err != nil {
		t.Fatalf("RecordCircuitBreakerTransition half-open: %v", err)
	}

	transitions, err := q.ListCircuitBreakerTransitions(ctx, "echo", "poll", 10)
	if err != nil {
		t.Fatalf("ListCircuitBreakerTransitions: %v", err)
	}
	if len(transitions) != 2 {
		t.Fatalf("transitions len=%d want 2", len(transitions))
	}
	if transitions[0].Reason != CircuitTransitionCooldownElapsed {
		t.Fatalf("newest reason=%q want %q", transitions[0].Reason, CircuitTransitionCooldownElapsed)
	}
	if transitions[1].Reason != CircuitTransitionFailureThreshold {
		t.Fatalf("oldest reason=%q want %q", transitions[1].Reason, CircuitTransitionFailureThreshold)
	}
	if transitions[1].JobID == nil || *transitions[1].JobID != jobID {
		t.Fatalf("job_id=%v want %q", transitions[1].JobID, jobID)
	}
}

func TestQueueScheduleEntryStateRoundTrip(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "state.db")
	db, err := storage.OpenSQLite(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	q := New(db)

	got, err := q.GetScheduleEntryState(context.Background(), "echo", "default")
	if err != nil {
		t.Fatalf("GetScheduleEntryState initial: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil initial state, got %#v", got)
	}

	reason := "command_not_supported"
	lastFiredAt := time.Now().UTC().Add(-30 * time.Minute).Truncate(time.Microsecond)
	lastSuccessJobID := "job-123"
	lastSuccessAt := time.Now().UTC().Add(-10 * time.Minute).Truncate(time.Microsecond)
	nextRunAt := time.Now().UTC().Add(50 * time.Minute).Truncate(time.Microsecond)
	if err := q.UpsertScheduleEntryState(context.Background(), ScheduleEntryState{
		Plugin:           "echo",
		ScheduleID:       "default",
		Command:          "token_refresh",
		Status:           ScheduleEntryPausedInvalid,
		Reason:           &reason,
		LastFiredAt:      &lastFiredAt,
		LastSuccessJobID: &lastSuccessJobID,
		LastSuccessAt:    &lastSuccessAt,
		NextRunAt:        &nextRunAt,
	}); err != nil {
		t.Fatalf("UpsertScheduleEntryState insert: %v", err)
	}

	got, err = q.GetScheduleEntryState(context.Background(), "echo", "default")
	if err != nil {
		t.Fatalf("GetScheduleEntryState after insert: %v", err)
	}
	if got == nil {
		t.Fatal("expected persisted schedule state")
	}
	if got.Status != ScheduleEntryPausedInvalid {
		t.Fatalf("status=%q want %q", got.Status, ScheduleEntryPausedInvalid)
	}
	if got.Reason == nil || *got.Reason != reason {
		t.Fatalf("reason=%v want %q", got.Reason, reason)
	}
	if got.LastFiredAt == nil || !got.LastFiredAt.Equal(lastFiredAt) {
		t.Fatalf("last_fired_at=%v want %v", got.LastFiredAt, lastFiredAt)
	}
	if got.LastSuccessJobID == nil || *got.LastSuccessJobID != lastSuccessJobID {
		t.Fatalf("last_success_job_id=%v want %q", got.LastSuccessJobID, lastSuccessJobID)
	}
	if got.LastSuccessAt == nil || !got.LastSuccessAt.Equal(lastSuccessAt) {
		t.Fatalf("last_success_at=%v want %v", got.LastSuccessAt, lastSuccessAt)
	}
	if got.NextRunAt == nil || !got.NextRunAt.Equal(nextRunAt) {
		t.Fatalf("next_run_at=%v want %v", got.NextRunAt, nextRunAt)
	}

	if err := q.UpsertScheduleEntryState(context.Background(), ScheduleEntryState{
		Plugin:     "echo",
		ScheduleID: "default",
		Command:    "token_refresh",
		Status:     ScheduleEntryActive,
	}); err != nil {
		t.Fatalf("UpsertScheduleEntryState update: %v", err)
	}

	got, err = q.GetScheduleEntryState(context.Background(), "echo", "default")
	if err != nil {
		t.Fatalf("GetScheduleEntryState after update: %v", err)
	}
	if got == nil {
		t.Fatal("expected state after update")
	}
	if got.Status != ScheduleEntryActive {
		t.Fatalf("status=%q want %q", got.Status, ScheduleEntryActive)
	}
	if got.Reason != nil {
		t.Fatalf("expected nil reason after activate, got %v", *got.Reason)
	}
	if got.LastFiredAt != nil || got.LastSuccessJobID != nil || got.LastSuccessAt != nil || got.NextRunAt != nil {
		t.Fatalf("expected timing fields cleared after activate update, got %#v", got)
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

func TestQueueEnqueueDedupeOverrideBlocksOutstanding(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "state.db")
	db, err := storage.OpenSQLite(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	q := New(db, WithLogger(slog.Default()), WithDedupeTTL(24*time.Hour))
	key := "poll:echo"
	overrideTTL := 1 * time.Minute

	firstID, err := q.Enqueue(context.Background(), EnqueueRequest{
		Plugin:      "echo",
		Command:     "poll",
		SubmittedBy: "scheduler",
		DedupeKey:   &key,
		DedupeTTL:   &overrideTTL,
	})
	if err != nil {
		t.Fatalf("first enqueue: %v", err)
	}
	if firstID == "" {
		t.Fatal("first enqueue returned empty job id")
	}

	dupID, err := q.Enqueue(context.Background(), EnqueueRequest{
		Plugin:      "echo",
		Command:     "poll",
		SubmittedBy: "scheduler",
		DedupeKey:   &key,
		DedupeTTL:   &overrideTTL,
	})
	if err == nil {
		t.Fatalf("expected dedupe drop error for outstanding job, got nil with id=%q", dupID)
	}
	var dedupeErr *DedupeDropError
	if !errors.As(err, &dedupeErr) {
		t.Fatalf("expected DedupeDropError, got %T: %v", err, err)
	}
	if dedupeErr.ExistingJobID != firstID {
		t.Fatalf("existing job id = %q, want %q", dedupeErr.ExistingJobID, firstID)
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
	if err := q.CompleteWithResult(context.Background(), firstID, StatusSucceeded, json.RawMessage(`{"status":"ok","result":"ok"}`), nil, nil); err != nil {
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
	if err := q.CompleteWithResult(context.Background(), firstID, StatusSucceeded, json.RawMessage(`{"status":"ok","result":"ok"}`), nil, nil); err != nil {
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

func TestQueueEnqueueDedupeTTLOverride(t *testing.T) {
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
	if err := q.CompleteWithResult(context.Background(), firstID, StatusSucceeded, json.RawMessage(`{"status":"ok","result":"ok"}`), nil, nil); err != nil {
		t.Fatalf("complete success: %v", err)
	}

	completedAt := time.Now().UTC().Add(-2 * time.Minute).Format(time.RFC3339Nano)
	if _, err := db.Exec(`UPDATE job_queue SET completed_at = ? WHERE id = ?`, completedAt, firstID); err != nil {
		t.Fatalf("set completed_at: %v", err)
	}

	_, err = q.Enqueue(context.Background(), EnqueueRequest{
		Plugin:      "echo",
		Command:     "poll",
		SubmittedBy: "scheduler",
		DedupeKey:   &key,
	})
	var dedupeErr *DedupeDropError
	if !errors.As(err, &dedupeErr) {
		t.Fatalf("expected global dedupe hit, got %v", err)
	}

	overrideTTL := 1 * time.Minute
	secondID, err := q.Enqueue(context.Background(), EnqueueRequest{
		Plugin:      "echo",
		Command:     "poll",
		SubmittedBy: "scheduler",
		DedupeKey:   &key,
		DedupeTTL:   &overrideTTL,
	})
	if err != nil {
		t.Fatalf("enqueue with dedupe ttl override: %v", err)
	}
	if secondID == "" {
		t.Fatal("second enqueue returned empty job id")
	}
}

func TestDequeueEligibleSkipsPlugins(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "state.db")
	db, err := storage.OpenSQLite(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	q := New(db)
	ctx := context.Background()

	// Enqueue jobs for two plugins in order: alpha, beta, alpha
	for _, p := range []string{"alpha", "beta", "alpha"} {
		if _, err := q.Enqueue(ctx, EnqueueRequest{
			Plugin: p, Command: "poll", SubmittedBy: "test",
		}); err != nil {
			t.Fatalf("Enqueue %s: %v", p, err)
		}
	}

	// Skip alpha — should get beta
	j, err := q.DequeueEligible(ctx, []string{"alpha"}, nil)
	if err != nil {
		t.Fatalf("DequeueEligible: %v", err)
	}
	if j == nil || j.Plugin != "beta" {
		t.Fatalf("expected beta, got %v", j)
	}

	// Skip alpha again — no eligible jobs left (beta already claimed)
	j2, err := q.DequeueEligible(ctx, []string{"alpha"}, nil)
	if err != nil {
		t.Fatalf("DequeueEligible 2: %v", err)
	}
	if j2 != nil {
		t.Fatalf("expected nil, got %v", j2)
	}

	// No skip — should get first alpha
	j3, err := q.DequeueEligible(ctx, nil, nil)
	if err != nil {
		t.Fatalf("DequeueEligible 3: %v", err)
	}
	if j3 == nil || j3.Plugin != "alpha" {
		t.Fatalf("expected alpha, got %v", j3)
	}
}

func TestDequeueEligibleSkipsConcurrencyKeys(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "state.db")
	db, err := storage.OpenSQLite(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	q := New(db)
	ctx := context.Background()

	keyA := "git_repo_sync:owner/repoA"
	keyB := "git_repo_sync:owner/repoB"

	// Enqueue: repoA first, then repoB
	if _, err := q.Enqueue(ctx, EnqueueRequest{
		Plugin: "git_repo_sync", Command: "handle", SubmittedBy: "test", DedupeKey: &keyA,
	}); err != nil {
		t.Fatalf("Enqueue A: %v", err)
	}
	if _, err := q.Enqueue(ctx, EnqueueRequest{
		Plugin: "git_repo_sync", Command: "handle", SubmittedBy: "test", DedupeKey: &keyB,
	}); err != nil {
		t.Fatalf("Enqueue B: %v", err)
	}

	// Block keyA — should get repoB
	j, err := q.DequeueEligible(ctx, nil, []string{keyA})
	if err != nil {
		t.Fatalf("DequeueEligible: %v", err)
	}
	if j == nil || j.DedupeKey == nil || *j.DedupeKey != keyB {
		t.Fatalf("expected repoB key, got %v", j)
	}

	// Block keyA still — no eligible (repoB already claimed)
	j2, err := q.DequeueEligible(ctx, nil, []string{keyA})
	if err != nil {
		t.Fatalf("DequeueEligible 2: %v", err)
	}
	if j2 != nil {
		t.Fatalf("expected nil, got %v", j2)
	}
}

func TestDequeueEligibleSkipsRunningDedupeKeyFromQueueTruth(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "state.db")
	db, err := storage.OpenSQLite(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	q := New(db)
	ctx := context.Background()

	keyA := "repo:alpha"
	keyB := "repo:beta"
	idA, err := q.Enqueue(ctx, EnqueueRequest{
		Plugin: "sync", Command: "handle", SubmittedBy: "test", DedupeKey: &keyA,
	})
	if err != nil {
		t.Fatalf("Enqueue A: %v", err)
	}
	idA2, err := q.Enqueue(ctx, EnqueueRequest{
		Plugin: "sync", Command: "handle", SubmittedBy: "test", DedupeKey: &keyA,
	})
	if err != nil {
		t.Fatalf("Enqueue A2: %v", err)
	}
	idB, err := q.Enqueue(ctx, EnqueueRequest{
		Plugin: "sync", Command: "handle", SubmittedBy: "test", DedupeKey: &keyB,
	})
	if err != nil {
		t.Fatalf("Enqueue B: %v", err)
	}

	first, err := q.DequeueEligible(ctx, nil, nil)
	if err != nil {
		t.Fatalf("DequeueEligible first: %v", err)
	}
	if first == nil || first.ID != idA {
		t.Fatalf("first=%v want %s", first, idA)
	}

	second, err := q.DequeueEligible(ctx, nil, nil)
	if err != nil {
		t.Fatalf("DequeueEligible second: %v", err)
	}
	if second == nil || second.ID != idB {
		t.Fatalf("second=%v want %s", second, idB)
	}

	lastErr := "done"
	if err := q.Complete(ctx, idA, StatusSucceeded, &lastErr, nil); err != nil {
		t.Fatalf("Complete A: %v", err)
	}
	third, err := q.DequeueEligible(ctx, nil, nil)
	if err != nil {
		t.Fatalf("DequeueEligible third: %v", err)
	}
	if third == nil || third.ID != idA2 {
		t.Fatalf("third=%v want %s", third, idA2)
	}
}

func TestDequeueEligibleEmptyFiltersMatchesDequeue(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "state.db")
	db, err := storage.OpenSQLite(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	q := New(db)
	ctx := context.Background()

	id, err := q.Enqueue(ctx, EnqueueRequest{
		Plugin: "echo", Command: "poll", SubmittedBy: "test",
	})
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	j, err := q.DequeueEligible(ctx, nil, nil)
	if err != nil {
		t.Fatalf("DequeueEligible: %v", err)
	}
	if j == nil || j.ID != id {
		t.Fatalf("expected job %s, got %v", id, j)
	}
}

func TestDequeueEligibleCombinedFilters(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "state.db")
	db, err := storage.OpenSQLite(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	q := New(db)
	ctx := context.Background()

	keyX := "sync:repoX"

	// Job 1: plugin alpha (will be skipped)
	if _, err := q.Enqueue(ctx, EnqueueRequest{
		Plugin: "alpha", Command: "poll", SubmittedBy: "test",
	}); err != nil {
		t.Fatal(err)
	}
	// Job 2: plugin beta with keyX (key will be blocked)
	if _, err := q.Enqueue(ctx, EnqueueRequest{
		Plugin: "beta", Command: "handle", SubmittedBy: "test", DedupeKey: &keyX,
	}); err != nil {
		t.Fatal(err)
	}
	// Job 3: plugin beta without key (should be picked)
	id3, err := q.Enqueue(ctx, EnqueueRequest{
		Plugin: "beta", Command: "poll", SubmittedBy: "test",
	})
	if err != nil {
		t.Fatal(err)
	}

	j, err := q.DequeueEligible(ctx, []string{"alpha"}, []string{keyX})
	if err != nil {
		t.Fatalf("DequeueEligible: %v", err)
	}
	if j == nil || j.ID != id3 {
		t.Fatalf("expected job %s (beta no-key), got %v", id3, j)
	}
}
