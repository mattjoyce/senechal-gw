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
	if err := validateSQLiteFilesystem(path); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create sqlite directory: %w", err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	// Basic health check + apply safe performance pragmas.
	pctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	pragmas := []string{
		"PRAGMA foreign_keys = ON;",
		"PRAGMA busy_timeout = 5000;",
		"PRAGMA journal_mode = WAL;",
		"PRAGMA synchronous = NORMAL;",
	}
	for _, p := range pragmas {
		if _, err := db.ExecContext(pctx, p); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("apply pragma %q: %w", p, err)
		}
	}

	// Restrict to 1 connection to avoid SQLITE_BUSY errors during concurrent writes.
	// This ensures only one writer at a time while busy_timeout handles waiting.
	db.SetMaxOpenConns(1)

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
  source_event_id TEXT,
  event_context_id TEXT
);`,
		`CREATE TABLE IF NOT EXISTS plugin_state (
  plugin_name TEXT PRIMARY KEY,
  state       JSON NOT NULL DEFAULT '{}',
  updated_at  TEXT
);`,
		`CREATE TABLE IF NOT EXISTS event_context (
  id               TEXT PRIMARY KEY,
  parent_id        TEXT,
  pipeline_name    TEXT,
  step_id          TEXT,
  accumulated_json JSON NOT NULL,
  created_at       TEXT NOT NULL
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
  source_event_id TEXT,
  event_context_id TEXT
);`,
		`CREATE TABLE IF NOT EXISTS circuit_breakers (
  plugin          TEXT NOT NULL,
  command         TEXT NOT NULL,
  state           TEXT NOT NULL DEFAULT 'closed',
  failure_count   INTEGER NOT NULL DEFAULT 0,
  opened_at       TEXT,
  last_failure_at TEXT,
  last_job_id     TEXT,
  updated_at      TEXT NOT NULL,
  PRIMARY KEY(plugin, command)
);`,
		`CREATE TABLE IF NOT EXISTS schedule_entries (
  plugin          TEXT NOT NULL,
  schedule_id     TEXT NOT NULL,
  command         TEXT NOT NULL,
  status          TEXT NOT NULL DEFAULT 'active',
  reason          TEXT,
  updated_at      TEXT NOT NULL,
  PRIMARY KEY(plugin, schedule_id)
);`,
		`CREATE INDEX IF NOT EXISTS job_queue_status_created_at_idx ON job_queue(status, created_at);`,
		`CREATE INDEX IF NOT EXISTS job_queue_plugin_command_status_idx ON job_queue(plugin, command, status);`,
		`CREATE INDEX IF NOT EXISTS event_context_parent_id_idx ON event_context(parent_id);`,
		`CREATE INDEX IF NOT EXISTS schedule_entries_status_idx ON schedule_entries(status);`,
		`CREATE UNIQUE INDEX IF NOT EXISTS job_queue_event_source_idx ON job_queue(parent_job_id, source_event_id) WHERE source_event_id IS NOT NULL;`,
	}

	for _, stmt := range stmts {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("bootstrap sqlite: %w", err)
		}
	}

	// Migrations: CREATE TABLE IF NOT EXISTS doesn't add new columns.
	if err := ensureColumnExists(ctx, db, "job_log", "result", "TEXT"); err != nil {
		return err
	}
	if err := ensureColumnExists(ctx, db, "job_queue", "event_context_id", "TEXT"); err != nil {
		return err
	}
	if err := ensureColumnExists(ctx, db, "job_log", "event_context_id", "TEXT"); err != nil {
		return err
	}
	return nil
}

func ensureColumnExists(ctx context.Context, db *sql.DB, table, column, columnDef string) error {
	cols, err := db.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%s);", table))
	if err != nil {
		return fmt.Errorf("bootstrap sqlite: inspect %s columns: %w", table, err)
	}
	defer cols.Close()

	hasColumn := false
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
			return fmt.Errorf("bootstrap sqlite: scan %s columns: %w", table, err)
		}
		if name == column {
			hasColumn = true
			break
		}
	}
	if err := cols.Err(); err != nil {
		return fmt.Errorf("bootstrap sqlite: iterate %s columns: %w", table, err)
	}
	if hasColumn {
		return nil
	}

	stmt := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s;", table, column, columnDef)
	if _, err := db.ExecContext(ctx, stmt); err != nil {
		return fmt.Errorf("bootstrap sqlite: add %s.%s column: %w", table, column, err)
	}
	return nil
}
