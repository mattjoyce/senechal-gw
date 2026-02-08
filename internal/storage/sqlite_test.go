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

	for _, table := range []string{"job_queue", "plugin_state", "job_log"} {
		var name string
		if err := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?;", table).Scan(&name); err != nil {
			t.Fatalf("table %q missing: %v", table, err)
		}
	}
}
