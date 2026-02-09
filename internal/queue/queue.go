package queue

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

const maxStderrBytes = 64 * 1024

type Queue struct {
	db *sql.DB
}

func New(db *sql.DB) *Queue {
	return &Queue{db: db}
}

func (q *Queue) Enqueue(ctx context.Context, req EnqueueRequest) (string, error) {
	if req.Plugin == "" {
		return "", fmt.Errorf("plugin is empty")
	}
	if req.Command == "" {
		return "", fmt.Errorf("command is empty")
	}
	if req.SubmittedBy == "" {
		return "", fmt.Errorf("submitted_by is empty")
	}

	id := uuid.NewString()
	now := time.Now().UTC().Format(time.RFC3339Nano)

	maxAttempts := req.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 4
	}

	var payload any
	if len(req.Payload) > 0 {
		payload = string(req.Payload)
	}

	_, err := q.db.ExecContext(ctx, `
INSERT INTO job_queue(
  id, plugin, command, payload, status, attempt, max_attempts, submitted_by, dedupe_key,
  created_at, parent_job_id
)
VALUES(?, ?, ?, ?, ?, 1, ?, ?, ?, ?, ?);
`, id, req.Plugin, req.Command, payload, StatusQueued, maxAttempts, req.SubmittedBy, req.DedupeKey, now, req.ParentJobID)
	if err != nil {
		return "", fmt.Errorf("enqueue job: %w", err)
	}
	return id, nil
}

// Dequeue claims the oldest queued job and marks it running. Returns (nil, nil)
// if the queue is empty.
func (q *Queue) Dequeue(ctx context.Context) (*Job, error) {
	now := time.Now().UTC()
	nowS := now.Format(time.RFC3339Nano)

	row := q.db.QueryRowContext(ctx, `
WITH next AS (
  SELECT id
  FROM job_queue
  WHERE status = ? AND (next_retry_at IS NULL OR next_retry_at <= ?)
  ORDER BY created_at ASC, rowid ASC
  LIMIT 1
)
UPDATE job_queue
SET status = ?, started_at = ?
WHERE id IN (SELECT id FROM next)
RETURNING
  id, plugin, command, payload, status, attempt, max_attempts, submitted_by, dedupe_key,
  created_at, started_at, completed_at, next_retry_at, last_error, parent_job_id, source_event_id;
`, StatusQueued, nowS, StatusRunning, nowS)

	var (
		j             Job
		payload       sql.NullString
		dedupeKey     sql.NullString
		createdAtS    string
		startedAtS    sql.NullString
		completedAtS  sql.NullString
		nextRetryAtS  sql.NullString
		lastError     sql.NullString
		parentJobID   sql.NullString
		sourceEventID sql.NullString
		statusS       string
	)
	err := row.Scan(
		&j.ID, &j.Plugin, &j.Command, &payload, &statusS, &j.Attempt, &j.MaxAttempts, &j.SubmittedBy, &dedupeKey,
		&createdAtS, &startedAtS, &completedAtS, &nextRetryAtS, &lastError, &parentJobID, &sourceEventID,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("dequeue job: %w", err)
	}

	j.Status = Status(statusS)
	if payload.Valid {
		j.Payload = []byte(payload.String)
	}
	if dedupeKey.Valid {
		j.DedupeKey = &dedupeKey.String
	}
	if t, err := time.Parse(time.RFC3339Nano, createdAtS); err == nil {
		j.CreatedAt = t
	}
	if startedAtS.Valid {
		if t, err := time.Parse(time.RFC3339Nano, startedAtS.String); err == nil {
			j.StartedAt = &t
		}
	}
	if completedAtS.Valid {
		if t, err := time.Parse(time.RFC3339Nano, completedAtS.String); err == nil {
			j.CompletedAt = &t
		}
	}
	if nextRetryAtS.Valid {
		if t, err := time.Parse(time.RFC3339Nano, nextRetryAtS.String); err == nil {
			j.NextRetryAt = &t
		}
	}
	if lastError.Valid {
		j.LastError = &lastError.String
	}
	if parentJobID.Valid {
		j.ParentJobID = &parentJobID.String
	}
	if sourceEventID.Valid {
		j.SourceEventID = &sourceEventID.String
	}
	return &j, nil
}

// FindJobsByStatus retrieves all jobs with the given status.
func (q *Queue) FindJobsByStatus(ctx context.Context, status Status) ([]*Job, error) {
	rows, err := q.db.QueryContext(ctx, `
SELECT
  id, plugin, command, payload, status, attempt, max_attempts, submitted_by, dedupe_key,
  created_at, started_at, completed_at, next_retry_at, last_error, parent_job_id, source_event_id
FROM job_queue
WHERE status = ?;
`, status)
	if err != nil {
		return nil, fmt.Errorf("query jobs by status: %w", err)
	}
	defer rows.Close()

	var jobs []*Job
	for rows.Next() {
		var (
			j             Job
			payload       sql.NullString
			dedupeKey     sql.NullString
			createdAtS    string
			startedAtS    sql.NullString
			completedAtS  sql.NullString
			nextRetryAtS  sql.NullString
			lastError     sql.NullString
			parentJobID   sql.NullString
			sourceEventID sql.NullString
			statusS       string
		)
		if err := rows.Scan(
			&j.ID, &j.Plugin, &j.Command, &payload, &statusS, &j.Attempt, &j.MaxAttempts, &j.SubmittedBy, &dedupeKey,
			&createdAtS, &startedAtS, &completedAtS, &nextRetryAtS, &lastError, &parentJobID, &sourceEventID,
		); err != nil {
			return nil, fmt.Errorf("scan job: %w", err)
		}

		j.Status = Status(statusS)
		if payload.Valid {
			j.Payload = []byte(payload.String)
		}
		if dedupeKey.Valid {
			j.DedupeKey = &dedupeKey.String
		}
		if t, err := time.Parse(time.RFC3339Nano, createdAtS); err == nil {
			j.CreatedAt = t
		}
		if startedAtS.Valid {
			if t, err := time.Parse(time.RFC3339Nano, startedAtS.String); err == nil {
				j.StartedAt = &t
			}
		}
		if completedAtS.Valid {
			if t, err := time.Parse(time.RFC3339Nano, completedAtS.String); err == nil {
				j.CompletedAt = &t
			}
		}
		if nextRetryAtS.Valid {
			if t, err := time.Parse(time.RFC3339Nano, nextRetryAtS.String); err == nil {
				j.NextRetryAt = &t
			}
		}
		if lastError.Valid {
			j.LastError = &lastError.String
		}
		if parentJobID.Valid {
			j.ParentJobID = &parentJobID.String
		}
		if sourceEventID.Valid {
			j.SourceEventID = &sourceEventID.String
		}
		jobs = append(jobs, &j)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate job rows: %w", err)
	}

	return jobs, nil
}

// UpdateJobForRecovery updates a job's status, attempt count, next retry time, and last error.
// This is used for crash recovery to re-queue jobs with backoff or mark them as dead.
func (q *Queue) UpdateJobForRecovery(ctx context.Context, jobID string, newStatus Status, newAttempt int, nextRetryAt *time.Time, lastError string) error {
	var nextRetryAtS *string
	if nextRetryAt != nil {
		s := nextRetryAt.Format(time.RFC3339Nano)
		nextRetryAtS = &s
	}

	var lastErrorS *string
	if lastError != "" {
		lastErrorS = &lastError
	}

	_, err := q.db.ExecContext(ctx, `
UPDATE job_queue
SET status = ?, attempt = ?, next_retry_at = ?, last_error = ?
WHERE id = ?;
`, newStatus, newAttempt, nextRetryAtS, lastErrorS, jobID)
	if err != nil {
		return fmt.Errorf("update job %s for recovery: %w", jobID, err)
	}
	return nil
}

// PruneJobLogs deletes job log entries older than the specified retention duration.
func (q *Queue) PruneJobLogs(ctx context.Context, retention time.Duration) error {
	if retention <= 0 {
		return nil // No retention, do nothing
	}

	cutoff := time.Now().UTC().Add(-retention)
	_, err := q.db.ExecContext(ctx, `
DELETE FROM job_log
WHERE completed_at < ?;
`, cutoff.Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("prune job logs: %w", err)
	}
	return nil
}
// Complete marks a job complete and writes a log row. This signature is kept
// stable since other sprint work may call it directly.
func (q *Queue) Complete(ctx context.Context, jobID string, status Status, lastError, stderr *string) error {
	return q.CompleteWithResult(ctx, jobID, status, nil, lastError, stderr)
}

// CompleteWithResult is like Complete but also stores the raw protocol response
// (plugin stdout JSON) in job_log.result for audit/debugging and API retrieval.
func (q *Queue) CompleteWithResult(ctx context.Context, jobID string, status Status, result json.RawMessage, lastError, stderr *string) error {
	if jobID == "" {
		return fmt.Errorf("jobID is empty")
	}
	if status != StatusSucceeded && status != StatusFailed && status != StatusTimedOut && status != StatusDead {
		return fmt.Errorf("invalid terminal status: %q", status)
	}

	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var (
		plugin        string
		command       string
		attempt       int
		submittedBy   string
		createdAt     string
		parentJobID   sql.NullString
		sourceEventID sql.NullString
	)
	if err := tx.QueryRowContext(ctx, `
SELECT plugin, command, attempt, submitted_by, created_at, parent_job_id, source_event_id
FROM job_queue
WHERE id = ?;
`, jobID).Scan(&plugin, &command, &attempt, &submittedBy, &createdAt, &parentJobID, &sourceEventID); err != nil {
		return fmt.Errorf("load job for completion: %w", err)
	}

	completedAt := time.Now().UTC().Format(time.RFC3339Nano)

	_, err = tx.ExecContext(ctx, `
UPDATE job_queue
SET status = ?, completed_at = ?, last_error = ?
WHERE id = ?;
`, status, completedAt, lastError, jobID)
	if err != nil {
		return fmt.Errorf("update job completion: %w", err)
	}

	logID := fmt.Sprintf("%s-%d", jobID, attempt)

	var resultVal any
	if len(result) > 0 {
		// Store JSON as a string blob to keep it queryable/debuggable in SQLite.
		resultVal = string(result)
	}

	var stderrVal any
	if stderr != nil {
		s := *stderr
		if len(s) > maxStderrBytes {
			s = s[:maxStderrBytes]
		}
		stderrVal = s
	}

	_, err = tx.ExecContext(ctx, `
INSERT INTO job_log(
  id, plugin, command, status, result, attempt, submitted_by, created_at, completed_at, last_error, stderr, parent_job_id, source_event_id
)
VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);
`, logID, plugin, command, status, resultVal, attempt, submittedBy, createdAt, completedAt, lastError, stderrVal, parentJobID, sourceEventID)
	if err != nil {
		return fmt.Errorf("insert job_log: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

// GetJobByID retrieves a job by its ID with result from job_log
func (q *Queue) GetJobByID(ctx context.Context, jobID string) (*JobResult, error) {
	if jobID == "" {
		return nil, fmt.Errorf("jobID is empty")
	}

	row := q.db.QueryRowContext(ctx, `
SELECT
  q.id, q.status, q.plugin, q.command, q.last_error, q.started_at, q.completed_at,
  l.result
FROM job_queue q
LEFT JOIN job_log l ON l.id = (q.id || '-' || q.attempt)
WHERE q.id = ?;
`, jobID)

	var (
		id          string
		statusS     string
		plugin      string
		command     string
		lastErrS    sql.NullString
		startedAtS  sql.NullString
		completedAt sql.NullString
		resultS     sql.NullString
	)
	if err := row.Scan(&id, &statusS, &plugin, &command, &lastErrS, &startedAtS, &completedAt, &resultS); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrJobNotFound
		}
		return nil, fmt.Errorf("get job by id: %w", err)
	}

	var lastErr *string
	if lastErrS.Valid {
		lastErr = &lastErrS.String
	}

	var startedAt *time.Time
	if startedAtS.Valid {
		if t, err := time.Parse(time.RFC3339Nano, startedAtS.String); err == nil {
			startedAt = &t
		}
	}

	var completedAtT *time.Time
	if completedAt.Valid {
		if t, err := time.Parse(time.RFC3339Nano, completedAt.String); err == nil {
			completedAtT = &t
		}
	}

	var result json.RawMessage
	if resultS.Valid {
		result = json.RawMessage(resultS.String)
	}

	return &JobResult{
		JobID:       id,
		Status:      Status(statusS),
		Plugin:      plugin,
		Command:     command,
		Result:      result,
		LastError:   lastErr,
		StartedAt:   startedAt,
		CompletedAt: completedAtT,
	}, nil
}
