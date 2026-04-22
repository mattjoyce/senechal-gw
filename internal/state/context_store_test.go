package state

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"github.com/mattjoyce/ductile/internal/storage"
)

func TestContextStoreCreateMergeAndLineage(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "state.db")
	db, err := storage.OpenSQLite(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	store := NewContextStore(db)

	root, err := store.Create(
		context.Background(),
		nil,
		"wisdom-chain",
		"root",
		json.RawMessage(`{"origin_plugin":"discord","channel_id":"123","video_url":"https://example.com/v"}`),
	)
	if err != nil {
		t.Fatalf("Create(root): %v", err)
	}

	child, err := store.Create(
		context.Background(),
		&root.ID,
		"wisdom-chain",
		"downloader",
		json.RawMessage(`{"filename":"lecture.mp4","size_bytes":1234}`),
	)
	if err != nil {
		t.Fatalf("Create(child): %v", err)
	}

	var accumulated map[string]any
	if err := json.Unmarshal(child.AccumulatedJSON, &accumulated); err != nil {
		t.Fatalf("Unmarshal(child accumulated): %v", err)
	}

	if accumulated["origin_plugin"] != "discord" {
		t.Fatalf("origin_plugin = %#v, want %q", accumulated["origin_plugin"], "discord")
	}
	if accumulated["channel_id"] != "123" {
		t.Fatalf("channel_id = %#v, want %q", accumulated["channel_id"], "123")
	}
	if accumulated["filename"] != "lecture.mp4" {
		t.Fatalf("filename = %#v, want %q", accumulated["filename"], "lecture.mp4")
	}

	lineage, err := store.Lineage(context.Background(), child.ID)
	if err != nil {
		t.Fatalf("Lineage(child): %v", err)
	}
	if len(lineage) != 2 {
		t.Fatalf("Lineage length = %d, want 2", len(lineage))
	}
	if lineage[0].ID != root.ID || lineage[1].ID != child.ID {
		t.Fatalf("unexpected lineage order: got [%s, %s], want [%s, %s]", lineage[0].ID, lineage[1].ID, root.ID, child.ID)
	}
}

func TestContextStoreOriginAnchorImmutable(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "state.db")
	db, err := storage.OpenSQLite(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	store := NewContextStore(db)

	root, err := store.Create(
		context.Background(),
		nil,
		"wisdom-chain",
		"root",
		json.RawMessage(`{"origin_channel_id":"abc"}`),
	)
	if err != nil {
		t.Fatalf("Create(root): %v", err)
	}

	_, err = store.Create(
		context.Background(),
		&root.ID,
		"wisdom-chain",
		"next",
		json.RawMessage(`{"origin_channel_id":"def"}`),
	)
	if !errors.Is(err, ErrBaggagePathImmutable) {
		t.Fatalf("Create(child override origin) error = %v, want %v", err, ErrBaggagePathImmutable)
	}

	child, err := store.Create(
		context.Background(),
		&root.ID,
		"wisdom-chain",
		"next",
		json.RawMessage(`{"origin_user_id":"u-1"}`),
	)
	if err != nil {
		t.Fatalf("Create(child new origin): %v", err)
	}

	var accumulated map[string]any
	if err := json.Unmarshal(child.AccumulatedJSON, &accumulated); err != nil {
		t.Fatalf("unmarshal child accumulated: %v", err)
	}
	if accumulated["origin_channel_id"] != "abc" {
		t.Fatalf("origin_channel_id = %#v, want %q", accumulated["origin_channel_id"], "abc")
	}
	if accumulated["origin_user_id"] != "u-1" {
		t.Fatalf("origin_user_id = %#v, want %q", accumulated["origin_user_id"], "u-1")
	}
}

func TestContextStoreDeepAccretion(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "state.db")
	db, err := storage.OpenSQLite(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	store := NewContextStore(db)

	root, err := store.Create(
		context.Background(),
		nil,
		"wisdom-chain",
		"root",
		json.RawMessage(`{"whisper":{"text":"hello"},"summary":{"text":"short"},"status":"new"}`),
	)
	if err != nil {
		t.Fatalf("Create(root): %v", err)
	}

	child, err := store.Create(
		context.Background(),
		&root.ID,
		"wisdom-chain",
		"child",
		json.RawMessage(`{"whisper":{"language":"en","text":"hello"},"summary":{"format":"markdown"},"source":{"url":"https://example.test"}}`),
	)
	if err != nil {
		t.Fatalf("Create(child): %v", err)
	}

	var accumulated map[string]any
	if err := json.Unmarshal(child.AccumulatedJSON, &accumulated); err != nil {
		t.Fatalf("unmarshal child accumulated: %v", err)
	}
	whisper := accumulated["whisper"].(map[string]any)
	if whisper["text"] != "hello" {
		t.Fatalf("whisper.text = %#v, want hello", whisper["text"])
	}
	if whisper["language"] != "en" {
		t.Fatalf("whisper.language = %#v, want en", whisper["language"])
	}
	summary := accumulated["summary"].(map[string]any)
	if summary["text"] != "short" {
		t.Fatalf("summary.text = %#v, want short", summary["text"])
	}
	if summary["format"] != "markdown" {
		t.Fatalf("summary.format = %#v, want markdown", summary["format"])
	}
}

func TestContextStoreDeepAccretionRejectsMutation(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "state.db")
	db, err := storage.OpenSQLite(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	store := NewContextStore(db)

	root, err := store.Create(
		context.Background(),
		nil,
		"wisdom-chain",
		"root",
		json.RawMessage(`{"summary":{"text":"short"},"status":"new","source":"inline"}`),
	)
	if err != nil {
		t.Fatalf("Create(root): %v", err)
	}

	tests := []struct {
		name    string
		updates json.RawMessage
		path    string
	}{
		{
			name:    "nested value differs",
			updates: json.RawMessage(`{"summary":{"text":"changed"}}`),
			path:    "summary.text",
		},
		{
			name:    "top level scalar differs",
			updates: json.RawMessage(`{"status":"done"}`),
			path:    "status",
		},
		{
			name:    "parent scalar cannot become object",
			updates: json.RawMessage(`{"source":{"url":"https://example.test"}}`),
			path:    "source",
		},
		{
			name:    "parent object cannot become scalar",
			updates: json.RawMessage(`{"summary":"changed"}`),
			path:    "summary",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := store.Create(context.Background(), &root.ID, "wisdom-chain", "child", tt.updates)
			if !errors.Is(err, ErrBaggagePathImmutable) {
				t.Fatalf("Create(child) error = %v, want %v", err, ErrBaggagePathImmutable)
			}
			var pathErr *BaggagePathImmutableError
			if !errors.As(err, &pathErr) {
				t.Fatalf("Create(child) error = %T, want *BaggagePathImmutableError", err)
			}
			if pathErr.Path != tt.path {
				t.Fatalf("Path = %q, want %q", pathErr.Path, tt.path)
			}
		})
	}
}

func TestContextStoreDeepAccretionAllowsRouteDepthAdvance(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "state.db")
	db, err := storage.OpenSQLite(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	store := NewContextStore(db)

	root, err := store.Create(
		context.Background(),
		nil,
		"chain",
		"",
		json.RawMessage(`{"ductile":{"pipeline_instance_id":"instance-1","route_depth":0,"route_max_depth":3}}`),
	)
	if err != nil {
		t.Fatalf("Create(root): %v", err)
	}

	child, err := store.Create(
		context.Background(),
		&root.ID,
		"chain",
		"step_a",
		json.RawMessage(`{"ductile":{"route_depth":1},"repo":{"name":"demo"}}`),
	)
	if err != nil {
		t.Fatalf("Create(child): %v", err)
	}

	if got := RouteDepthFromAccumulated(child.AccumulatedJSON); got != 1 {
		t.Fatalf("route depth = %d, want 1", got)
	}
	if got := PipelineInstanceIDFromAccumulated(child.AccumulatedJSON); got != "instance-1" {
		t.Fatalf("pipeline instance id = %q, want instance-1", got)
	}
	if got := RouteMaxDepthFromAccumulated(child.AccumulatedJSON); got != 3 {
		t.Fatalf("route max depth = %d, want 3", got)
	}
}

func TestContextStoreMaxAccumulatedSize(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "state.db")
	db, err := storage.OpenSQLite(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	store := NewContextStore(db)
	store.maxContextBytes = 64

	_, err = store.Create(
		context.Background(),
		nil,
		"p",
		"s",
		json.RawMessage(`{"blob":"abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyz"}`),
	)
	if err == nil {
		t.Fatalf("expected max context size error, got nil")
	}
}

func TestContextStoreGetNotFound(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "state.db")
	db, err := storage.OpenSQLite(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	store := NewContextStore(db)
	_, err = store.Get(context.Background(), "missing")
	if !errors.Is(err, ErrEventContextNotFound) {
		t.Fatalf("Get(missing) error = %v, want %v", err, ErrEventContextNotFound)
	}
}
