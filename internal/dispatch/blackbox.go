package dispatch

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// BlackBoxMetadata records the terminal state of a job execution.
type BlackBoxMetadata struct {
	JobID       string         `json:"job_id"`
	Plugin      string         `json:"plugin"`
	Command     string         `json:"command"`
	Status      string         `json:"status"`
	Attempt     int            `json:"attempt"`
	CreatedAt   time.Time      `json:"created_at"`
	StartedAt   *time.Time     `json:"started_at,omitempty"`
	CompletedAt time.Time      `json:"completed_at"`
	LastError   *string        `json:"last_error,omitempty"`
	Context     map[string]any `json:"context"`
}

// writeBlackBox writes the black box bundle (.ductile/ directory) into workspaceDir.
// It creates stdout, stderr, and metadata.json files. Errors are non-fatal; the
// caller should log and continue.
func writeBlackBox(workspaceDir string, stdout []byte, stderr string, meta BlackBoxMetadata) error {
	if workspaceDir == "" {
		return nil
	}

	bundleDir := filepath.Join(workspaceDir, ".ductile")
	if err := os.MkdirAll(bundleDir, 0o700); err != nil {
		return fmt.Errorf("create black box bundle dir: %w", err)
	}

	if len(stdout) > 0 {
		if err := os.WriteFile(filepath.Join(bundleDir, "stdout"), stdout, 0o600); err != nil {
			return fmt.Errorf("write stdout: %w", err)
		}
	}

	if stderr != "" {
		if err := os.WriteFile(filepath.Join(bundleDir, "stderr"), []byte(stderr), 0o600); err != nil {
			return fmt.Errorf("write stderr: %w", err)
		}
	}

	metaBytes, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}
	if err := os.WriteFile(filepath.Join(bundleDir, "metadata.json"), metaBytes, 0o600); err != nil {
		return fmt.Errorf("write metadata.json: %w", err)
	}

	return nil
}
