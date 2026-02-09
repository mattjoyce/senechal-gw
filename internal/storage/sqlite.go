package storage

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// OpenSQLite opens (and creates if needed) the SQLite database at path and
// ensures required tables exist.
func OpenSQLite(ctx context.Context, path string) (*sql.DB, error) {
	if path == "" {
		return nil, fmt.Errorf("sqlite path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create sqlite directory: %w", err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	// Basic health check + apply a few safe pragmas.
	pctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if _, err := db.ExecContext(pctx, "PRAGMA foreign_keys = ON;"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("enable foreign_keys: %w", err)
	}
	if _, err := db.ExecContext(pctx, "PRAGMA busy_timeout = 5000;"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("set busy_timeout: %w", err)
	}
	if err := BootstrapSQLite(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

// BootstrapSQLite creates tables/indexes if missing (SPEC section 10).
func BootstrapSQLite(ctx context.Context, db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS job_queue (
  id              TEXT PRIMARY KEY,
  plugin          TEXT NOT NULL,
  command         TEXT NOT NULL,
  payload         JSON,
  status          TEXT NOT NULL,
  attempt         INTEGER NOT NULL DEFAULT 1,
  max_attempts    INTEGER NOT NULL DEFAULT 4,
  submitted_by    TEXT NOT NULL,
  dedupe_key      TEXT,
  created_at      TEXT NOT NULL,
  started_at      TEXT,
  completed_at    TEXT,
  next_retry_at   TEXT,
  last_error      TEXT,
  parent_job_id   TEXT,
  source_event_id TEXT
);`,
		`CREATE TABLE IF NOT EXISTS plugin_state (
  plugin_name TEXT PRIMARY KEY,
  state       JSON NOT NULL DEFAULT '{}',
  updated_at  TEXT
);`,
		`CREATE TABLE IF NOT EXISTS job_log (
  id              TEXT PRIMARY KEY,
  plugin          TEXT NOT NULL,
  command         TEXT NOT NULL,
  status          TEXT NOT NULL,
  result          TEXT,
  attempt         INTEGER NOT NULL,
  submitted_by    TEXT NOT NULL,
  created_at      TEXT NOT NULL,
  completed_at    TEXT NOT NULL,
  last_error      TEXT,
  stderr          TEXT,
  parent_job_id   TEXT,
  source_event_id TEXT
);`,
		`CREATE INDEX IF NOT EXISTS job_queue_status_created_at_idx ON job_queue(status, created_at);`,
		`CREATE INDEX IF NOT EXISTS job_queue_plugin_command_status_idx ON job_queue(plugin, command, status);`,
	}

	for _, stmt := range stmts {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("bootstrap sqlite: %w", err)
		}
	}

	// Migrations: CREATE TABLE IF NOT EXISTS doesn't add new columns.
	if err := ensureJobLogResultColumn(ctx, db); err != nil {
		return err
	}
	return nil
}

func ensureJobLogResultColumn(ctx context.Context, db *sql.DB) error {
	cols, err := db.QueryContext(ctx, "PRAGMA table_info(job_log);")
	if err != nil {
		return fmt.Errorf("bootstrap sqlite: inspect job_log columns: %w", err)
	}
	defer cols.Close()

	hasResult := false
	for cols.Next() {
		// cid, name, type, notnull, dflt_value, pk
		var (
			cid       int
			name      string
			typ       string
			notnull   int
			dfltValue any
			pk        int
		)
		if err := cols.Scan(&cid, &name, &typ, &notnull, &dfltValue, &pk); err != nil {
			return fmt.Errorf("bootstrap sqlite: scan job_log columns: %w", err)
		}
		if name == "result" {
			hasResult = true
			break
		}
	}
	if err := cols.Err(); err != nil {
		return fmt.Errorf("bootstrap sqlite: iterate job_log columns: %w", err)
	}
	if hasResult {
		return nil
	}

	if _, err := db.ExecContext(ctx, "ALTER TABLE job_log ADD COLUMN result TEXT;"); err != nil {
		return fmt.Errorf("bootstrap sqlite: add job_log.result column: %w", err)
	}
	return nil
}
