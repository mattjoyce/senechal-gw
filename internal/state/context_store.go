package state

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"strings"
	"time"

	"github.com/google/uuid"
)

const DefaultMaxContextBytes = DefaultMaxStateBytes

var (
	ErrEventContextNotFound = errors.New("event context not found")
	ErrBaggagePathImmutable = errors.New("baggage path is immutable")
	// ErrOriginAnchorImmutable is kept as a compatibility alias for callers that
	// still check the pre-Sprint-3 origin-anchor error.
	ErrOriginAnchorImmutable = ErrBaggagePathImmutable
)

// BaggagePathImmutableError reports an attempted mutation of an inherited
// baggage path.
type BaggagePathImmutableError struct {
	Path string
}

func (e *BaggagePathImmutableError) Error() string {
	if e == nil || strings.TrimSpace(e.Path) == "" {
		return "baggage path is immutable: inherited value differs from child value"
	}
	return fmt.Sprintf("baggage path %q is immutable: inherited value differs from child value", e.Path)
}

func (e *BaggagePathImmutableError) Is(target error) bool {
	return target == ErrBaggagePathImmutable || target == ErrOriginAnchorImmutable
}

// EventContext is one immutable ledger entry in the control plane.
type EventContext struct {
	ID              string
	ParentID        *string
	PipelineName    string
	StepID          string
	AccumulatedJSON json.RawMessage
	CreatedAt       time.Time
}

// ContextStore persists and retrieves event context lineage.
type ContextStore struct {
	db              *sql.DB
	maxContextBytes int
	now             func() time.Time
}

func NewContextStore(db *sql.DB) *ContextStore {
	return &ContextStore{
		db:              db,
		maxContextBytes: DefaultMaxContextBytes,
		now:             time.Now,
	}
}

// Create appends a new context row to the ledger.
//
// If parentID is nil, updates becomes the root accumulated context.
// If parentID is set, updates is deep-accreted into parent accumulated context.
// Existing baggage paths are immutable: descendants may repeat the same JSON
// value or add new paths, but may not revise inherited values.
func (s *ContextStore) Create(
	ctx context.Context,
	parentID *string,
	pipelineName string,
	stepID string,
	updates json.RawMessage,
) (*EventContext, error) {
	return s.create(ctx, parentID, pipelineName, stepID, updates)
}

func (s *ContextStore) create(
	ctx context.Context,
	parentID *string,
	pipelineName string,
	stepID string,
	updates json.RawMessage,
) (*EventContext, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	updateMap, err := decodeObjectOrEmpty(updates)
	if err != nil {
		return nil, fmt.Errorf("decode context updates: %w", err)
	}

	accumulated := map[string]json.RawMessage{}
	if parentID != nil {
		parent, err := s.Get(ctx, *parentID)
		if err != nil {
			return nil, fmt.Errorf("load parent context %q: %w", *parentID, err)
		}

		parentMap, err := decodeObjectOrEmpty(parent.AccumulatedJSON)
		if err != nil {
			return nil, fmt.Errorf("decode parent accumulated_json: %w", err)
		}
		accumulated = cloneRawMap(parentMap)

		if err := deepAccreteRawMap(accumulated, updateMap, ""); err != nil {
			return nil, err
		}
	} else {
		maps.Copy(accumulated, updateMap)
	}

	accumulatedJSON, err := json.Marshal(accumulated)
	if err != nil {
		return nil, fmt.Errorf("marshal accumulated context: %w", err)
	}
	if len(accumulatedJSON) > s.maxContextBytes {
		return nil, fmt.Errorf("accumulated context exceeds max size (%d bytes)", s.maxContextBytes)
	}

	id := uuid.NewString()
	now := s.now().UTC()
	nowS := now.Format(time.RFC3339Nano)

	_, err = s.db.ExecContext(ctx, `
INSERT INTO event_context(id, parent_id, pipeline_name, step_id, accumulated_json, created_at)
VALUES(?, ?, ?, ?, ?, ?);
`, id, parentID, pipelineName, stepID, string(accumulatedJSON), nowS)
	if err != nil {
		return nil, fmt.Errorf("insert event context: %w", err)
	}

	return &EventContext{
		ID:              id,
		ParentID:        parentID,
		PipelineName:    pipelineName,
		StepID:          stepID,
		AccumulatedJSON: accumulatedJSON,
		CreatedAt:       now,
	}, nil
}

// Get returns one event context row by ID.
func (s *ContextStore) Get(ctx context.Context, id string) (*EventContext, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(id) == "" {
		return nil, fmt.Errorf("event context id is empty")
	}

	row := s.db.QueryRowContext(ctx, `
SELECT id, parent_id, pipeline_name, step_id, accumulated_json, created_at
FROM event_context
WHERE id = ?;
`, id)

	return scanEventContextRow(row)
}

// Lineage returns the context chain from root -> leaf for the given context ID.
func (s *ContextStore) Lineage(ctx context.Context, id string) ([]*EventContext, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(id) == "" {
		return nil, fmt.Errorf("event context id is empty")
	}

	rows, err := s.db.QueryContext(ctx, `
WITH RECURSIVE lineage AS (
  SELECT id, parent_id, pipeline_name, step_id, accumulated_json, created_at, 0 AS depth
  FROM event_context
  WHERE id = ?
  UNION ALL
  SELECT ec.id, ec.parent_id, ec.pipeline_name, ec.step_id, ec.accumulated_json, ec.created_at, lineage.depth + 1
  FROM event_context ec
  JOIN lineage ON lineage.parent_id = ec.id
)
SELECT id, parent_id, pipeline_name, step_id, accumulated_json, created_at
FROM lineage
ORDER BY depth DESC;
`, id)
	if err != nil {
		return nil, fmt.Errorf("query context lineage: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var contexts []*EventContext
	for rows.Next() {
		ctxRow, err := scanEventContext(rows)
		if err != nil {
			return nil, err
		}
		contexts = append(contexts, ctxRow)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate context lineage rows: %w", err)
	}
	if len(contexts) == 0 {
		return nil, ErrEventContextNotFound
	}
	return contexts, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanEventContextRow(row rowScanner) (*EventContext, error) {
	ctxRow, err := scanEventContext(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrEventContextNotFound
		}
		return nil, err
	}
	return ctxRow, nil
}

func scanEventContext(row rowScanner) (*EventContext, error) {
	var (
		ctxRow          EventContext
		parentID        sql.NullString
		accumulatedJSON string
		createdAtS      string
	)
	if err := row.Scan(
		&ctxRow.ID,
		&parentID,
		&ctxRow.PipelineName,
		&ctxRow.StepID,
		&accumulatedJSON,
		&createdAtS,
	); err != nil {
		return nil, err
	}

	if parentID.Valid {
		ctxRow.ParentID = &parentID.String
	}

	if !json.Valid([]byte(accumulatedJSON)) {
		return nil, fmt.Errorf("stored accumulated_json is invalid JSON for context=%q", ctxRow.ID)
	}
	ctxRow.AccumulatedJSON = json.RawMessage(accumulatedJSON)

	createdAt, err := time.Parse(time.RFC3339Nano, createdAtS)
	if err != nil {
		return nil, fmt.Errorf("parse event_context.created_at: %w", err)
	}
	ctxRow.CreatedAt = createdAt
	return &ctxRow, nil
}

func cloneRawMap(in map[string]json.RawMessage) map[string]json.RawMessage {
	out := make(map[string]json.RawMessage, len(in))
	maps.Copy(out, in)
	return out
}

func jsonValuesEqual(a, b json.RawMessage) bool {
	var av any
	if err := json.Unmarshal(a, &av); err != nil {
		return false
	}
	var bv any
	if err := json.Unmarshal(b, &bv); err != nil {
		return false
	}
	return reflect.DeepEqual(av, bv)
}

func deepAccreteRawMap(dst, updates map[string]json.RawMessage, prefix string) error {
	for key, updateValue := range updates {
		path := joinBaggagePath(prefix, key)
		currentValue, exists := dst[key]
		if !exists {
			dst[key] = updateValue
			continue
		}

		if jsonValuesEqual(currentValue, updateValue) {
			continue
		}

		if path == PipelineInstanceNamespace+"."+RouteDepthField {
			dst[key] = updateValue
			continue
		}

		currentObject, currentIsObject := decodeRawObject(currentValue)
		updateObject, updateIsObject := decodeRawObject(updateValue)
		if currentIsObject && updateIsObject {
			if err := deepAccreteRawMap(currentObject, updateObject, path); err != nil {
				return err
			}
			merged, err := json.Marshal(currentObject)
			if err != nil {
				return fmt.Errorf("marshal merged baggage path %q: %w", path, err)
			}
			dst[key] = merged
			continue
		}

		return &BaggagePathImmutableError{Path: path}
	}
	return nil
}

func decodeRawObject(raw json.RawMessage) (map[string]json.RawMessage, bool) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, false
	}
	return obj, true
}

func joinBaggagePath(prefix, key string) string {
	if prefix == "" {
		return key
	}
	return prefix + "." + key
}
