package storage

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaSQL string

// OpenSQLite opens (and creates if needed) the SQLite database at path and
// ensures required tables exist.
func OpenSQLite(ctx context.Context, path string) (*sql.DB, error) {
	if path == "" {
		return nil, fmt.Errorf("sqlite path is empty")
	}
	if err := validateSQLiteFilesystem(path); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
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
	// Execute the embedded mono-schema.
	// We split by semicolon to execute each statement separately.
	// Note: This is a simple parser that doesn't handle semicolons in strings/comments,
	// but for our controlled schema.sql it is sufficient.
	for _, stmt := range strings.Split(schemaSQL, ";") {
		trimmed := strings.TrimSpace(stmt)
		if trimmed == "" {
			continue
		}
		if _, err := db.ExecContext(ctx, trimmed); err != nil {
			return fmt.Errorf("bootstrap sqlite (schema.sql): %w", err)
		}
	}

	// Migrations: CREATE TABLE IF NOT EXISTS doesn't add new columns to existing tables.
	// We keep these helpers for backward compatibility with existing installations
	// that might be missing these specific columns.
	migrationHelpers := []struct {
		table  string
		column string
		def    string
	}{
		{"job_log", "result", "TEXT"},
		{"job_log", "job_id", "TEXT"},
		{"job_queue", "event_context_id", "TEXT"},
		{"job_log", "event_context_id", "TEXT"},
		{"schedule_entries", "last_success_job_id", "TEXT"},
		{"schedule_entries", "last_fired_at", "TEXT"},
		{"schedule_entries", "last_success_at", "TEXT"},
		{"schedule_entries", "next_run_at", "TEXT"},
	}

	for _, m := range migrationHelpers {
		if err := ensureColumnExists(ctx, db, m.table, m.column, m.def); err != nil {
			return err
		}
	}

	// Ensure specific indexes that might not be in the base schema of older installs.
	if err := ensureIndexExists(ctx, db, "job_log_job_id_attempt_idx", "CREATE INDEX IF NOT EXISTS job_log_job_id_attempt_idx ON job_log(job_id, attempt);"); err != nil {
		return err
	}

	return nil
}

func ensureColumnExists(ctx context.Context, db *sql.DB, table, column, columnDef string) error {
	cols, err := db.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%s);", table))
	if err != nil {
		return fmt.Errorf("bootstrap sqlite: inspect %s columns: %w", table, err)
	}
	defer func() { _ = cols.Close() }()

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

func ensureIndexExists(ctx context.Context, db *sql.DB, name, stmt string) error {
	if _, err := db.ExecContext(ctx, stmt); err != nil {
		return fmt.Errorf("bootstrap sqlite: ensure index %s: %w", name, err)
	}
	return nil
}
