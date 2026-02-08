package queue

import (
	"context"
	"database/sql"
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

// Complete marks a job terminal and appends a row to job_log.
func (q *Queue) Complete(ctx context.Context, jobID string, status Status, lastError, stderr *string) error {
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
  id, plugin, command, status, attempt, submitted_by, created_at, completed_at, last_error, stderr, parent_job_id, source_event_id
)
VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);
`, logID, plugin, command, status, attempt, submittedBy, createdAt, completedAt, lastError, stderrVal, parentJobID, sourceEventID)
	if err != nil {
		return fmt.Errorf("insert job_log: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}
