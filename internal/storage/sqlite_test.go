package storage

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenSQLiteBootstrapsTables(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "state.db")
	db, err := OpenSQLite(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	for _, table := range []string{"job_queue", "job_transitions", "job_attempts", "plugin_state", "plugin_facts", "event_context", "job_log", "circuit_breakers", "circuit_breaker_transitions"} {
		var name string
		if err := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?;", table).Scan(&name); err != nil {
			t.Fatalf("table %q missing: %v", table, err)
		}
	}

	var jobIDColumn string
	if err := db.QueryRow("SELECT name FROM pragma_table_info('job_log') WHERE name = 'job_id';").Scan(&jobIDColumn); err != nil {
		t.Fatalf("job_log.job_id missing: %v", err)
	}
	if jobIDColumn != "job_id" {
		t.Fatalf("job_log column = %q, want job_id", jobIDColumn)
	}
}

func TestOpenSQLiteRejectsExistingOutdatedSchema(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "state.db")
	db, err := OpenSQLite(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("OpenSQLite bootstrap: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("db.Close(): %v", err)
	}

	rawDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { _ = rawDB.Close() })

	if _, err := rawDB.Exec("DROP TABLE plugin_facts;"); err != nil {
		t.Fatalf("drop plugin_facts: %v", err)
	}

	_, err = OpenSQLite(context.Background(), dbPath)
	if err == nil {
		t.Fatal("expected schema validation failure, got nil")
	}
	if !strings.Contains(err.Error(), "sqlite schema validation failed") {
		t.Fatalf("expected schema validation error, got %v", err)
	}
	if !strings.Contains(err.Error(), "migrate-hickey-sprint-7-plugin-facts.py") {
		t.Fatalf("expected migration guidance, got %v", err)
	}

	var name string
	queryErr := rawDB.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='plugin_facts';").Scan(&name)
	if queryErr != sql.ErrNoRows {
		t.Fatalf("expected plugin_facts to remain absent after failed open, got name=%q err=%v", name, queryErr)
	}
}
