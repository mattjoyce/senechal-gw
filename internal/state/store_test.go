package state

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/mattjoyce/senechal-gw/internal/storage"
)

func TestStoreGetMissingReturnsEmptyObject(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "state.db")
	db, err := storage.OpenSQLite(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	s := NewStore(db)
	raw, err := s.Get(context.Background(), "echo")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(raw) != "{}" {
		t.Fatalf("expected {}, got %s", string(raw))
	}
}

func TestStoreShallowMergeReplacesTopLevelKeys(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "state.db")
	db, err := storage.OpenSQLite(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	s := NewStore(db)

	if _, err := s.ShallowMerge(context.Background(), "p", json.RawMessage(`{"a":1,"b":{"x":1}}`)); err != nil {
		t.Fatalf("ShallowMerge (1): %v", err)
	}
	merged, err := s.ShallowMerge(context.Background(), "p", json.RawMessage(`{"b":{"y":2}}`))
	if err != nil {
		t.Fatalf("ShallowMerge (2): %v", err)
	}
	// "b" is replaced, not deep-merged.
	if string(merged) != `{"a":1,"b":{"y":2}}` && string(merged) != `{"b":{"y":2},"a":1}` {
		t.Fatalf("unexpected merged state: %s", string(merged))
	}
}

func TestStoreStateSizeLimit(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "state.db")
	db, err := storage.OpenSQLite(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	s := NewStore(db)

	// Create a ~1.1MiB string payload.
	big := make([]byte, DefaultMaxStateBytes+100_000)
	for i := range big {
		big[i] = 'a'
	}
	update := json.RawMessage(`{"blob":"` + string(big) + `"}`)
	if _, err := s.ShallowMerge(context.Background(), "p", update); err == nil {
		t.Fatalf("expected size limit error, got nil")
	}
}
