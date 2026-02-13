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
	if !errors.Is(err, ErrOriginAnchorImmutable) {
		t.Fatalf("Create(child override origin) error = %v, want %v", err, ErrOriginAnchorImmutable)
	}

	_, err = store.Create(
		context.Background(),
		&root.ID,
		"wisdom-chain",
		"next",
		json.RawMessage(`{"origin_user_id":"u-1"}`),
	)
	if !errors.Is(err, ErrOriginAnchorImmutable) {
		t.Fatalf("Create(child new origin) error = %v, want %v", err, ErrOriginAnchorImmutable)
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
