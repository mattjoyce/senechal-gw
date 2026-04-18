package storage

import (
	"context"
	"path/filepath"
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

	for _, table := range []string{"job_queue", "job_transitions", "job_attempts", "plugin_state", "event_context", "job_log", "circuit_breakers"} {
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
