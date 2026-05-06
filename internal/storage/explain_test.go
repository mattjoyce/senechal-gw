package storage

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// updateGoldenEnv is read at test time to refresh the snapshot files under
// testdata/explain/ after an intentional query or schema change.
const updateGoldenEnv = "UPDATE_GOLDEN"

// TestExplainQueryPlanRegression pins the SQLite query plan for each hot
// production query so a future schema change (dropped index, renamed column,
// new SCAN) fails the test instead of becoming a silent production slowdown.
func TestExplainQueryPlanRegression(t *testing.T) {
	t.Parallel()

	cases := []explainCase{
		{
			name:        "claim_next",
			hotTable:    "job_queue",
			expectIndex: "job_queue_status_created_at_idx",
			args:        []any{"queued", "2099-01-01T00:00:00Z", "running", "running", "2099-01-01T00:00:00Z", nil},
			sql: `
WITH next AS (
  SELECT candidate.id
  FROM job_queue AS candidate
  WHERE candidate.status = ? AND (candidate.next_retry_at IS NULL OR candidate.next_retry_at = '' OR candidate.next_retry_at <= ?)
    AND (
      candidate.dedupe_key IS NULL
      OR NOT EXISTS (
        SELECT 1
        FROM job_queue AS running
        WHERE running.status = ?
          AND running.dedupe_key = candidate.dedupe_key
      )
    )
  ORDER BY candidate.created_at ASC, candidate.rowid ASC
  LIMIT 1
)
UPDATE job_queue
SET status = ?, started_at = ?, started_config_snapshot_id = ?
WHERE id IN (SELECT id FROM next)
RETURNING
  id, plugin, command, payload, status, attempt, max_attempts, submitted_by, dedupe_key,
  created_at, started_at, completed_at, next_retry_at, last_error, parent_job_id, source_event_id, event_context_id,
  enqueued_config_snapshot_id, started_config_snapshot_id;`,
		},
		{
			name:        "list_jobs_by_status",
			hotTable:    "job_queue",
			expectIndex: "job_queue_status_created_at_idx",
			args:        []any{"queued", 50},
			sql: `
SELECT
  id, plugin, command, status, created_at, started_at, completed_at, attempt
FROM job_queue
WHERE 1=1 AND status = ?
ORDER BY created_at DESC, rowid DESC
LIMIT ?;`,
		},
		{
			name:        "list_job_logs",
			hotTable:    "job_log",
			expectIndex: "job_log_completed_at_idx",
			args:        []any{"2026-01-01T00:00:00Z", 50},
			sql: `
SELECT
  COALESCE(l.job_id, COALESCE(q.id, '')) AS job_id, l.id, l.plugin, l.command, l.status, l.attempt, l.submitted_by, l.created_at, l.completed_at, l.last_error, l.stderr, NULL
FROM job_log l
LEFT JOIN job_queue q ON l.job_id = q.id AND l.attempt = q.attempt
WHERE 1=1 AND l.completed_at >= ?
ORDER BY l.completed_at DESC, l.rowid DESC
LIMIT ?;`,
		},
		{
			name:        "breaker_lookup",
			hotTable:    "circuit_breakers",
			expectIndex: "sqlite_autoindex_circuit_breakers_1",
			args:        []any{"plugin-x", "command-y"},
			sql: `
SELECT plugin, command, state, failure_count, opened_at, last_failure_at, last_job_id, updated_at
FROM circuit_breakers
WHERE plugin = ? AND command = ?;`,
		},
		{
			name:        "schedule_lookup",
			hotTable:    "schedule_entries",
			expectIndex: "sqlite_autoindex_schedule_entries_1",
			args:        []any{"plugin-x", "schedule-y"},
			sql: `
SELECT plugin, schedule_id, command, status, reason, last_fired_at, last_success_job_id, last_success_at, next_run_at, updated_at
FROM schedule_entries
WHERE plugin = ? AND schedule_id = ?;`,
		},
		{
			name:        "plugin_facts_by_plugin_seq",
			hotTable:    "plugin_facts",
			expectIndex: "plugin_facts_plugin_seq_idx",
			args:        []any{"file_watch", 20},
			sql: `
SELECT id, seq, plugin_name, fact_type, job_id, command, fact_json, created_at
FROM plugin_facts
WHERE plugin_name = ?
ORDER BY
  CASE WHEN seq IS NULL THEN 1 ELSE 0 END ASC,
  seq DESC,
  created_at DESC
LIMIT ?;`,
		},
	}

	dbPath := filepath.Join(t.TempDir(), "explain.db")
	db, err := OpenSQLite(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			plan, err := captureExplainQueryPlan(db, c.sql, c.args)
			if err != nil {
				t.Fatalf("EXPLAIN QUERY PLAN: %v", err)
			}

			scanRE := regexp.MustCompile(`\bSCAN ` + regexp.QuoteMeta(c.hotTable) + `\b`)
			if scanRE.MatchString(plan) {
				t.Fatalf("plan unexpectedly contains SCAN of hot table %q\n--- plan ---\n%s", c.hotTable, plan)
			}

			if !strings.Contains(plan, c.expectIndex) {
				t.Fatalf("plan missing expected index %q\n--- plan ---\n%s", c.expectIndex, plan)
			}

			goldenPath := filepath.Join("testdata", "explain", c.name+".golden")
			if os.Getenv(updateGoldenEnv) == "1" {
				if err := os.WriteFile(goldenPath, []byte(plan), 0o644); err != nil {
					t.Fatalf("write golden: %v", err)
				}
				return
			}

			want, err := os.ReadFile(goldenPath)
			if err != nil {
				t.Fatalf("read golden %s: %v (run with UPDATE_GOLDEN=1 to create)", goldenPath, err)
			}
			if string(want) != plan {
				t.Errorf("plan does not match golden %s\n--- want ---\n%s\n--- got ---\n%s", goldenPath, string(want), plan)
			}
		})
	}
}

type explainCase struct {
	name        string
	hotTable    string
	expectIndex string
	sql         string
	args        []any
}

// captureExplainQueryPlan runs EXPLAIN QUERY PLAN against query+args and
// returns the rows formatted as `id|parent|detail` lines. The fourth column
// (`notused` in current SQLite) is dropped to keep the snapshot stable across
// SQLite minor versions.
func captureExplainQueryPlan(db *sql.DB, query string, args []any) (string, error) {
	rows, err := db.Query("EXPLAIN QUERY PLAN "+query, args...)
	if err != nil {
		return "", err
	}
	defer func() { _ = rows.Close() }()

	var out strings.Builder
	for rows.Next() {
		var (
			id      int64
			parent  int64
			notused int64
			detail  string
		)
		if err := rows.Scan(&id, &parent, &notused, &detail); err != nil {
			return "", err
		}
		fmt.Fprintf(&out, "%d|%d|%s\n", id, parent, detail)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	return out.String(), nil
}
