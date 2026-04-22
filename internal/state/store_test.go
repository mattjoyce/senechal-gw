package state

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/mattjoyce/ductile/internal/storage"
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

func TestStoreSetReplacesFullState(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "state.db")
	db, err := storage.OpenSQLite(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	s := NewStore(db)

	if _, err := s.ShallowMerge(context.Background(), "file_watch", json.RawMessage(`{"watches":{"old":{"exists":true}},"last_health_check":"2026-01-01T00:00:00Z"}`)); err != nil {
		t.Fatalf("ShallowMerge: %v", err)
	}

	replaced, err := s.Set(context.Background(), "file_watch", json.RawMessage(`{"watches":{"current":{"exists":false}},"last_poll_at":"2026-04-22T00:00:00Z"}`))
	if err != nil {
		t.Fatalf("Set: %v", err)
	}

	if string(replaced) != `{"last_poll_at":"2026-04-22T00:00:00Z","watches":{"current":{"exists":false}}}` &&
		string(replaced) != `{"watches":{"current":{"exists":false}},"last_poll_at":"2026-04-22T00:00:00Z"}` {
		t.Fatalf("unexpected replaced state: %s", string(replaced))
	}

	got, err := s.Get(context.Background(), "file_watch")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(got) != string(replaced) {
		t.Fatalf("Get() = %s, want %s", string(got), string(replaced))
	}
}

func TestStoreRecordFactFileWatchSnapshotUpdatesCompatibilityState(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "state.db")
	db, err := storage.OpenSQLite(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	s := NewStore(db)
	createdAt := time.Date(2026, 4, 22, 1, 2, 3, 0, time.UTC)

	compatibilityState, updated, err := s.RecordFact(context.Background(), PluginFact{
		ID:         "fact-1",
		PluginName: "file_watch",
		FactType:   FactTypeFileWatchSnapshot,
		JobID:      "job-1",
		Command:    "poll",
		FactJSON:   json.RawMessage(`{"watches":{"single-file":{"exists":true,"fingerprint":"abc"}},"last_poll_at":"2026-04-22T01:02:03Z"}`),
		CreatedAt:  createdAt,
	})
	if err != nil {
		t.Fatalf("RecordFact: %v", err)
	}
	if !updated {
		t.Fatal("expected compatibility state update")
	}

	got, err := s.Get(context.Background(), "file_watch")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(got) != string(compatibilityState) {
		t.Fatalf("Get() = %s, want %s", string(got), string(compatibilityState))
	}

	facts, err := s.ListFacts(context.Background(), "file_watch", FactTypeFileWatchSnapshot, 10)
	if err != nil {
		t.Fatalf("ListFacts: %v", err)
	}
	if len(facts) != 1 {
		t.Fatalf("len(facts) = %d, want 1", len(facts))
	}
	if facts[0].ID != "fact-1" || facts[0].JobID != "job-1" || facts[0].Command != "poll" {
		t.Fatalf("unexpected fact row: %+v", facts[0])
	}
	if !facts[0].CreatedAt.Equal(createdAt) {
		t.Fatalf("CreatedAt = %s, want %s", facts[0].CreatedAt, createdAt)
	}
}

func TestStoreListFactsFiltersAndOrdersNewestFirst(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "state.db")
	db, err := storage.OpenSQLite(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	s := NewStore(db)
	factsToInsert := []PluginFact{
		{
			ID:         "fact-older",
			PluginName: "file_watch",
			FactType:   FactTypeFileWatchSnapshot,
			JobID:      "job-older",
			Command:    "poll",
			FactJSON:   json.RawMessage(`{"last_poll_at":"2026-04-22T00:00:00Z","watches":{}}`),
			CreatedAt:  time.Date(2026, 4, 22, 0, 0, 0, 0, time.UTC),
		},
		{
			ID:         "fact-newer",
			PluginName: "file_watch",
			FactType:   FactTypeFileWatchSnapshot,
			JobID:      "job-newer",
			Command:    "poll",
			FactJSON:   json.RawMessage(`{"last_poll_at":"2026-04-22T00:01:00Z","watches":{}}`),
			CreatedAt:  time.Date(2026, 4, 22, 0, 1, 0, 0, time.UTC),
		},
		{
			ID:         "fact-other-type",
			PluginName: "file_watch",
			FactType:   "file_watch.other",
			JobID:      "job-other",
			Command:    "poll",
			FactJSON:   json.RawMessage(`{"value":1}`),
			CreatedAt:  time.Date(2026, 4, 22, 0, 2, 0, 0, time.UTC),
		},
	}
	for _, fact := range factsToInsert {
		if _, _, err := s.RecordFact(context.Background(), fact); err != nil {
			t.Fatalf("RecordFact(%s): %v", fact.ID, err)
		}
	}

	filtered, err := s.ListFacts(context.Background(), "file_watch", FactTypeFileWatchSnapshot, 10)
	if err != nil {
		t.Fatalf("ListFacts(filtered): %v", err)
	}
	if len(filtered) != 2 {
		t.Fatalf("len(filtered) = %d, want 2", len(filtered))
	}
	if filtered[0].ID != "fact-newer" || filtered[1].ID != "fact-older" {
		t.Fatalf("unexpected order: [%s %s]", filtered[0].ID, filtered[1].ID)
	}

	limited, err := s.ListFacts(context.Background(), "file_watch", "", 1)
	if err != nil {
		t.Fatalf("ListFacts(limited): %v", err)
	}
	if len(limited) != 1 {
		t.Fatalf("len(limited) = %d, want 1", len(limited))
	}
	if limited[0].ID != "fact-other-type" {
		t.Fatalf("limited[0].ID = %q, want fact-other-type", limited[0].ID)
	}
}

func TestStoreRecordFactMirrorsCompatibilityStateForMigratedPlugins(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		pluginName string
		factType   string
		factJSON   json.RawMessage
	}{
		{
			name:       "folder_watch snapshot",
			pluginName: "folder_watch",
			factType:   FactTypeFolderWatchSnapshot,
			factJSON:   json.RawMessage(`{"last_poll_at":"2026-04-22T00:00:00Z","watches":{"docs":{"file_count":2}}}`),
		},
		{
			name:       "py-greet snapshot",
			pluginName: "py-greet",
			factType:   FactTypePyGreetSnapshot,
			factJSON:   json.RawMessage(`{"last_run":"2026-04-22T00:00:00Z","last_greeting":"Hi, Ductile!"}`),
		},
		{
			name:       "ts-bun-greet snapshot",
			pluginName: "ts-bun-greet",
			factType:   FactTypeTSBunGreetSnapshot,
			factJSON:   json.RawMessage(`{"last_run":"2026-04-22T00:00:00Z","last_greeting":"Hi, Ductile!"}`),
		},
		{
			name:       "stress state snapshot",
			pluginName: "stress",
			factType:   FactTypeStressStateSnapshot,
			factJSON:   json.RawMessage(`{"count":7}`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dbPath := filepath.Join(t.TempDir(), "state.db")
			db, err := storage.OpenSQLite(context.Background(), dbPath)
			if err != nil {
				t.Fatalf("OpenSQLite: %v", err)
			}
			t.Cleanup(func() { _ = db.Close() })

			s := NewStore(db)
			compatibilityState, updated, err := s.RecordFact(context.Background(), PluginFact{
				ID:         "fact-1",
				PluginName: tt.pluginName,
				FactType:   tt.factType,
				JobID:      "job-1",
				Command:    "poll",
				FactJSON:   tt.factJSON,
				CreatedAt:  time.Date(2026, 4, 22, 0, 0, 0, 0, time.UTC),
			})
			if err != nil {
				t.Fatalf("RecordFact: %v", err)
			}
			if !updated {
				t.Fatal("expected compatibility state update")
			}

			got, err := s.Get(context.Background(), tt.pluginName)
			if err != nil {
				t.Fatalf("Get: %v", err)
			}
			if string(got) != string(compatibilityState) {
				t.Fatalf("Get() = %s, want %s", string(got), string(compatibilityState))
			}
		})
	}
}
