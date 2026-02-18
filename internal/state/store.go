package state

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"time"
)

const DefaultMaxStateBytes = 1 << 20 // 1 MiB (SPEC)

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
	if len(merged) > s.maxStateBty {
		return nil, fmt.Errorf("plugin state exceeds max size (%d bytes)", s.maxStateBty)
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = tx.ExecContext(ctx, `
INSERT INTO plugin_state(plugin_name, state, updated_at)
VALUES(?, ?, ?)
ON CONFLICT(plugin_name) DO UPDATE SET
  state = excluded.state,
  updated_at = excluded.updated_at;
`, plugin, string(merged), now)
	if err != nil {
		return nil, fmt.Errorf("upsert plugin state: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}
	return json.RawMessage(merged), nil
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
