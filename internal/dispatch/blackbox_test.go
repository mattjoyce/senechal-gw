package dispatch

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWriteBlackBox(t *testing.T) {
	workspaceDir := t.TempDir()
	now := time.Now().UTC()

	meta := BlackBoxMetadata{
		JobID:       "ab000000000000000000000000000000",
		Plugin:      "echo",
		Command:     "poll",
		Status:      "succeeded",
		Attempt:     1,
		CreatedAt:   now.Add(-5 * time.Second),
		StartedAt:   &now,
		CompletedAt: now,
		Context:     map[string]any{"ductile_plugin": "echo"},
	}

	stdout := []byte(`{"status":"ok","result":"hello"}`)
	stderr := "some warning line"

	if err := writeBlackBox(workspaceDir, stdout, stderr, meta); err != nil {
		t.Fatalf("writeBlackBox() error = %v", err)
	}

	bundleDir := filepath.Join(workspaceDir, ".ductile")

	// stdout file
	gotStdout, err := os.ReadFile(filepath.Join(bundleDir, "stdout"))
	if err != nil {
		t.Fatalf("read stdout: %v", err)
	}
	if string(gotStdout) != string(stdout) {
		t.Errorf("stdout = %q, want %q", gotStdout, stdout)
	}

	// stderr file
	gotStderr, err := os.ReadFile(filepath.Join(bundleDir, "stderr"))
	if err != nil {
		t.Fatalf("read stderr: %v", err)
	}
	if string(gotStderr) != stderr {
		t.Errorf("stderr = %q, want %q", gotStderr, stderr)
	}

	// metadata.json schema
	metaBytes, err := os.ReadFile(filepath.Join(bundleDir, "metadata.json"))
	if err != nil {
		t.Fatalf("read metadata.json: %v", err)
	}
	var decoded BlackBoxMetadata
	if err := json.Unmarshal(metaBytes, &decoded); err != nil {
		t.Fatalf("unmarshal metadata.json: %v", err)
	}
	if decoded.JobID != meta.JobID {
		t.Errorf("metadata JobID = %q, want %q", decoded.JobID, meta.JobID)
	}
	if decoded.Status != meta.Status {
		t.Errorf("metadata Status = %q, want %q", decoded.Status, meta.Status)
	}
	if decoded.Plugin != meta.Plugin {
		t.Errorf("metadata Plugin = %q, want %q", decoded.Plugin, meta.Plugin)
	}
}

func TestWriteBlackBoxNilStdout(t *testing.T) {
	workspaceDir := t.TempDir()
	now := time.Now().UTC()

	meta := BlackBoxMetadata{
		JobID:       "cd000000000000000000000000000000",
		Plugin:      "echo",
		Command:     "poll",
		Status:      "failed",
		Attempt:     1,
		CreatedAt:   now,
		CompletedAt: now,
		Context:     map[string]any{},
	}

	// nil stdout should not create a stdout file but should not error either.
	if err := writeBlackBox(workspaceDir, nil, "", meta); err != nil {
		t.Fatalf("writeBlackBox(nil stdout) error = %v", err)
	}

	bundleDir := filepath.Join(workspaceDir, ".ductile")

	if _, err := os.Stat(filepath.Join(bundleDir, "stdout")); !os.IsNotExist(err) {
		t.Errorf("stdout file should not be created for nil stdout, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(bundleDir, "stderr")); !os.IsNotExist(err) {
		t.Errorf("stderr file should not be created for empty stderr, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(bundleDir, "metadata.json")); err != nil {
		t.Errorf("metadata.json should always be created, err = %v", err)
	}
}

func TestWriteBlackBoxEmptyWorkspaceDir(t *testing.T) {
	// Empty workspaceDir should be a no-op.
	if err := writeBlackBox("", []byte("data"), "err", BlackBoxMetadata{}); err != nil {
		t.Fatalf("writeBlackBox('') should be a no-op, got error = %v", err)
	}
}
