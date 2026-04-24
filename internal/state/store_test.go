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
	}, "mirror_object")
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
	if facts[0].Seq == nil || *facts[0].Seq != 1 {
		t.Fatalf("Seq = %v, want 1", facts[0].Seq)
	}
	if !facts[0].CreatedAt.Equal(createdAt) {
		t.Fatalf("CreatedAt = %s, want %s", facts[0].CreatedAt, createdAt)
	}
}

func TestStoreListFactsFiltersAndOrdersBySequenceNewestFirst(t *testing.T) {
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
			CreatedAt:  time.Date(2026, 4, 22, 0, 0, 0, 0, time.UTC),
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
		if _, _, err := s.RecordFact(context.Background(), fact, ""); err != nil {
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

func TestStoreListFactsHandlesLegacyRowsWithoutSeq(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "state.db")
	db, err := storage.OpenSQLite(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.ExecContext(context.Background(), `
INSERT INTO plugin_facts(id, plugin_name, fact_type, job_id, command, fact_json, created_at)
VALUES(?, ?, ?, ?, ?, ?, ?);
`, "legacy-newer-clock", "file_watch", FactTypeFileWatchSnapshot, "job-legacy", "poll", `{"legacy":true}`, "2026-04-23T00:00:00Z")
	if err != nil {
		t.Fatalf("insert legacy plugin fact: %v", err)
	}

	s := NewStore(db)
	if _, _, err := s.RecordFact(context.Background(), PluginFact{
		ID:         "sequenced-older-clock",
		PluginName: "file_watch",
		FactType:   FactTypeFileWatchSnapshot,
		JobID:      "job-sequenced",
		Command:    "poll",
		FactJSON:   json.RawMessage(`{"sequenced":true}`),
		CreatedAt:  time.Date(2026, 4, 22, 0, 0, 0, 0, time.UTC),
	}, "mirror_object"); err != nil {
		t.Fatalf("RecordFact: %v", err)
	}

	facts, err := s.ListFacts(context.Background(), "file_watch", FactTypeFileWatchSnapshot, 10)
	if err != nil {
		t.Fatalf("ListFacts: %v", err)
	}
	if len(facts) != 2 {
		t.Fatalf("len(facts) = %d, want 2", len(facts))
	}
	if facts[0].ID != "sequenced-older-clock" {
		t.Fatalf("facts[0].ID = %q, want sequenced-older-clock", facts[0].ID)
	}
	if facts[0].Seq == nil || *facts[0].Seq != 1 {
		t.Fatalf("sequenced fact Seq = %v, want 1", facts[0].Seq)
	}
	if facts[1].ID != "legacy-newer-clock" {
		t.Fatalf("facts[1].ID = %q, want legacy-newer-clock", facts[1].ID)
	}
	if facts[1].Seq != nil {
		t.Fatalf("legacy fact Seq = %v, want nil", facts[1].Seq)
	}
}

func TestStoreRecordFactRollsBackSequenceOnInsertFailure(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "state.db")
	db, err := storage.OpenSQLite(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	s := NewStore(db)
	record := func(id string) error {
		_, _, err := s.RecordFact(context.Background(), PluginFact{
			ID:         id,
			PluginName: "file_watch",
			FactType:   FactTypeFileWatchSnapshot,
			JobID:      "job-" + id,
			Command:    "poll",
			FactJSON:   json.RawMessage(`{"watches":{},"last_poll_at":"2026-04-22T00:00:00Z"}`),
			CreatedAt:  time.Date(2026, 4, 22, 0, 0, 0, 0, time.UTC),
		}, "mirror_object")
		return err
	}

	if err := record("fact-1"); err != nil {
		t.Fatalf("RecordFact(fact-1): %v", err)
	}
	if err := record("fact-2"); err != nil {
		t.Fatalf("RecordFact(fact-2): %v", err)
	}
	if err := record("fact-2"); err == nil {
		t.Fatal("expected duplicate fact insert to fail")
	}

	var sequenceValue int64
	if err := db.QueryRowContext(context.Background(), `
SELECT value
FROM storage_sequences
WHERE name = ?;
`, pluginFactsSequenceName).Scan(&sequenceValue); err != nil {
		t.Fatalf("query plugin fact sequence: %v", err)
	}
	if sequenceValue != 2 {
		t.Fatalf("sequence value = %d, want 2", sequenceValue)
	}

	if err := record("fact-3"); err != nil {
		t.Fatalf("RecordFact(fact-3): %v", err)
	}
	facts, err := s.ListFacts(context.Background(), "file_watch", FactTypeFileWatchSnapshot, 10)
	if err != nil {
		t.Fatalf("ListFacts: %v", err)
	}
	if len(facts) != 3 {
		t.Fatalf("len(facts) = %d, want 3", len(facts))
	}
	if facts[0].ID != "fact-3" || facts[0].Seq == nil || *facts[0].Seq != 3 {
		t.Fatalf("newest fact = %+v, want fact-3 seq 3", facts[0])
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
			}, "mirror_object")
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
