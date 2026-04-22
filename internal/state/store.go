package state

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"strings"
	"time"
)

const DefaultMaxStateBytes = 1 << 20 // 1 MiB (SPEC)

const (
	// FactTypeFileWatchSnapshot records the latest observed file_watch snapshot.
	FactTypeFileWatchSnapshot = "file_watch.snapshot"
	// FactTypeFolderWatchSnapshot records the latest observed folder_watch snapshot.
	FactTypeFolderWatchSnapshot = "folder_watch.snapshot"
	// FactTypePyGreetSnapshot records the latest py-greet greeting snapshot.
	FactTypePyGreetSnapshot = "py-greet.snapshot"
	// FactTypeTSBunGreetSnapshot records the latest ts-bun-greet greeting snapshot.
	FactTypeTSBunGreetSnapshot = "ts-bun-greet.snapshot"
	// FactTypeStressStateSnapshot records the latest stress counter snapshot.
	FactTypeStressStateSnapshot = "stress.state_snapshot"
)

// PluginFact is an append-only plugin fact row.
type PluginFact struct {
	ID         string
	PluginName string
	FactType   string
	JobID      string
	Command    string
	FactJSON   json.RawMessage
	CreatedAt  time.Time
}

type Store struct {
	db          *sql.DB
	maxStateBty int
}

func NewStore(db *sql.DB) *Store {
	return &Store{
		db:          db,
		maxStateBty: DefaultMaxStateBytes,
	}
}

// Get returns the full state blob for a plugin, or {} if missing.
func (s *Store) Get(ctx context.Context, plugin string) (json.RawMessage, error) {
	if plugin == "" {
		return nil, fmt.Errorf("plugin name is empty")
	}

	var raw string
	err := s.db.QueryRowContext(ctx, "SELECT state FROM plugin_state WHERE plugin_name = ?;", plugin).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return json.RawMessage(`{}`), nil
	}
	if err != nil {
		return nil, fmt.Errorf("read plugin state: %w", err)
	}
	if !json.Valid([]byte(raw)) {
		return nil, fmt.Errorf("stored plugin state is invalid JSON for plugin=%q", plugin)
	}
	return json.RawMessage(raw), nil
}

// Set replaces the full compatibility state blob for a plugin.
func (s *Store) Set(ctx context.Context, plugin string, state json.RawMessage) (json.RawMessage, error) {
	if plugin == "" {
		return nil, fmt.Errorf("plugin name is empty")
	}

	normalized, err := normalizeObjectJSON(state)
	if err != nil {
		return nil, fmt.Errorf("decode full state: %w", err)
	}
	if err := s.upsertState(ctx, s.db, plugin, normalized, time.Now().UTC()); err != nil {
		return nil, err
	}
	return normalized, nil
}

// ShallowMerge applies updates as a shallow merge (top-level keys replaced).
// The merged state is persisted and returned.
func (s *Store) ShallowMerge(ctx context.Context, plugin string, updates json.RawMessage) (json.RawMessage, error) {
	if plugin == "" {
		return nil, fmt.Errorf("plugin name is empty")
	}

	upd, err := decodeObjectOrEmpty(updates)
	if err != nil {
		return nil, fmt.Errorf("decode state_updates: %w", err)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Read current state (or {}).
	var curRaw string
	err = tx.QueryRowContext(ctx, "SELECT state FROM plugin_state WHERE plugin_name = ?;", plugin).Scan(&curRaw)
	if errors.Is(err, sql.ErrNoRows) {
		curRaw = "{}"
	} else if err != nil {
		return nil, fmt.Errorf("read plugin state: %w", err)
	}

	cur, err := decodeObjectOrEmpty(json.RawMessage(curRaw))
	if err != nil {
		return nil, fmt.Errorf("decode stored state: %w", err)
	}

	maps.Copy(cur, upd)

	merged, err := json.Marshal(cur)
	if err != nil {
		return nil, fmt.Errorf("marshal merged state: %w", err)
	}
	if err := s.upsertState(ctx, tx, plugin, merged, time.Now().UTC()); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}
	return json.RawMessage(merged), nil
}

// RecordFact appends a plugin fact row and updates compatibility state when the
// fact has a known reducer.
func (s *Store) RecordFact(ctx context.Context, fact PluginFact) (json.RawMessage, bool, error) {
	if strings.TrimSpace(fact.ID) == "" {
		return nil, false, fmt.Errorf("fact id is empty")
	}
	if strings.TrimSpace(fact.PluginName) == "" {
		return nil, false, fmt.Errorf("plugin name is empty")
	}
	if strings.TrimSpace(fact.FactType) == "" {
		return nil, false, fmt.Errorf("fact type is empty")
	}
	if strings.TrimSpace(fact.JobID) == "" {
		return nil, false, fmt.Errorf("job id is empty")
	}
	if strings.TrimSpace(fact.Command) == "" {
		return nil, false, fmt.Errorf("command is empty")
	}

	normalizedFact, err := normalizeObjectJSON(fact.FactJSON)
	if err != nil {
		return nil, false, fmt.Errorf("decode fact_json: %w", err)
	}
	fact.FactJSON = normalizedFact

	createdAt := fact.CreatedAt.UTC()
	if fact.CreatedAt.IsZero() {
		createdAt = time.Now().UTC()
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, false, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.ExecContext(ctx, `
INSERT INTO plugin_facts(id, plugin_name, fact_type, job_id, command, fact_json, created_at)
VALUES(?, ?, ?, ?, ?, ?, ?);
`, fact.ID, fact.PluginName, fact.FactType, fact.JobID, fact.Command, string(fact.FactJSON), createdAt.Format(time.RFC3339Nano))
	if err != nil {
		return nil, false, fmt.Errorf("insert plugin fact: %w", err)
	}

	compatibilityState, hasCompatibilityState, err := reduceCompatibilityState(fact.PluginName, fact.FactType, fact.FactJSON)
	if err != nil {
		return nil, false, err
	}
	if hasCompatibilityState {
		if err := s.upsertState(ctx, tx, fact.PluginName, compatibilityState, createdAt); err != nil {
			return nil, false, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, false, fmt.Errorf("commit tx: %w", err)
	}
	return compatibilityState, hasCompatibilityState, nil
}

// ListFacts returns recent facts for a plugin in reverse chronological order.
func (s *Store) ListFacts(ctx context.Context, plugin string, factType string, limit int) ([]PluginFact, error) {
	if strings.TrimSpace(plugin) == "" {
		return nil, fmt.Errorf("plugin name is empty")
	}
	if limit <= 0 {
		limit = 20
	}

	baseQuery := `
SELECT id, plugin_name, fact_type, job_id, command, fact_json, created_at
FROM plugin_facts
WHERE plugin_name = ?`
	args := []any{plugin}
	if strings.TrimSpace(factType) != "" {
		baseQuery += " AND fact_type = ?"
		args = append(args, factType)
	}
	baseQuery += " ORDER BY created_at DESC LIMIT ?;"
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, baseQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("query plugin facts: %w", err)
	}
	defer func() { _ = rows.Close() }()

	facts := make([]PluginFact, 0, limit)
	for rows.Next() {
		var (
			fact      PluginFact
			rawJSON   string
			createdAt string
		)
		if err := rows.Scan(&fact.ID, &fact.PluginName, &fact.FactType, &fact.JobID, &fact.Command, &rawJSON, &createdAt); err != nil {
			return nil, fmt.Errorf("scan plugin fact: %w", err)
		}
		if !json.Valid([]byte(rawJSON)) {
			return nil, fmt.Errorf("stored plugin fact is invalid JSON for plugin=%q fact_id=%q", plugin, fact.ID)
		}
		fact.FactJSON = json.RawMessage(rawJSON)
		if createdAt != "" {
			parsed, err := time.Parse(time.RFC3339Nano, createdAt)
			if err != nil {
				return nil, fmt.Errorf("parse plugin fact created_at for fact_id=%q: %w", fact.ID, err)
			}
			fact.CreatedAt = parsed
		}
		facts = append(facts, fact)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate plugin facts: %w", err)
	}
	return facts, nil
}

func (s *Store) upsertState(ctx context.Context, execer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}, plugin string, state json.RawMessage, updatedAt time.Time) error {
	if len(state) > s.maxStateBty {
		return fmt.Errorf("plugin state exceeds max size (%d bytes)", s.maxStateBty)
	}

	_, err := execer.ExecContext(ctx, `
INSERT INTO plugin_state(plugin_name, state, updated_at)
VALUES(?, ?, ?)
ON CONFLICT(plugin_name) DO UPDATE SET
  state = excluded.state,
  updated_at = excluded.updated_at;
`, plugin, string(state), updatedAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("upsert plugin state: %w", err)
	}
	return nil
}

func reduceCompatibilityState(pluginName, factType string, factJSON json.RawMessage) (json.RawMessage, bool, error) {
	switch {
	case pluginName == "file_watch" && factType == FactTypeFileWatchSnapshot:
		return mirrorFactAsCompatibilityState(FactTypeFileWatchSnapshot, factJSON)
	case pluginName == "folder_watch" && factType == FactTypeFolderWatchSnapshot:
		return mirrorFactAsCompatibilityState(FactTypeFolderWatchSnapshot, factJSON)
	case pluginName == "py-greet" && factType == FactTypePyGreetSnapshot:
		return mirrorFactAsCompatibilityState(FactTypePyGreetSnapshot, factJSON)
	case pluginName == "ts-bun-greet" && factType == FactTypeTSBunGreetSnapshot:
		return mirrorFactAsCompatibilityState(FactTypeTSBunGreetSnapshot, factJSON)
	case pluginName == "stress" && factType == FactTypeStressStateSnapshot:
		return mirrorFactAsCompatibilityState(FactTypeStressStateSnapshot, factJSON)
	default:
		return nil, false, nil
	}
}

func mirrorFactAsCompatibilityState(factType string, factJSON json.RawMessage) (json.RawMessage, bool, error) {
	normalized, err := normalizeObjectJSON(factJSON)
	if err != nil {
		return nil, false, fmt.Errorf("normalize %s compatibility state: %w", factType, err)
	}
	return normalized, true, nil
}

func normalizeObjectJSON(b json.RawMessage) (json.RawMessage, error) {
	obj, err := decodeObjectOrEmpty(b)
	if err != nil {
		return nil, err
	}
	normalized, err := json.Marshal(obj)
	if err != nil {
		return nil, fmt.Errorf("marshal JSON object: %w", err)
	}
	return json.RawMessage(normalized), nil
}

func decodeObjectOrEmpty(b json.RawMessage) (map[string]json.RawMessage, error) {
	if len(b) == 0 {
		return map[string]json.RawMessage{}, nil
	}
	if !json.Valid(b) {
		return nil, fmt.Errorf("invalid JSON")
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	if m == nil {
		m = map[string]json.RawMessage{}
	}
	return m, nil
}
