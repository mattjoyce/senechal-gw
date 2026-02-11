package storage

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var networkFilesystems = map[string]struct{}{
	"afpfs":  {},
	"cifs":   {},
	"nfs":    {},
	"smbfs":  {},
	"smb2":   {},
	"webdav": {},
}

// validateSQLiteFilesystem ensures the DB path is on a local filesystem.
func validateSQLiteFilesystem(path string) error {
	return validateSQLiteFilesystemWithDetector(path, detectFilesystemType)
}

func validateSQLiteFilesystemWithDetector(path string, detector func(string) (string, error)) error {
	if path == "" {
		return fmt.Errorf("sqlite path is empty")
	}

	inspectPath, err := nearestExistingPath(path)
	if err != nil {
		return fmt.Errorf("resolve database path %q: %w", path, err)
	}

	fsType, err := detector(inspectPath)
	if err != nil {
		return fmt.Errorf("detect filesystem for %q: %w", inspectPath, err)
	}

	if isNetworkFilesystem(fsType) {
		return fmt.Errorf(
			"database path %q is on network filesystem %q; SQLite requires a local filesystem for reliable locking. Use a local path via state.path (or --db /path/to/local/file.db) or move the working directory to local disk",
			path,
			fsType,
		)
	}

	return nil
}

func nearestExistingPath(path string) (string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("absolute path: %w", err)
	}

	candidate := absPath
	for {
		_, err := os.Stat(candidate)
		if err == nil {
			return candidate, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("stat %q: %w", candidate, err)
		}

		parent := filepath.Dir(candidate)
		if parent == candidate {
			return "", fmt.Errorf("no existing parent for %q", absPath)
		}
		candidate = parent
	}
}

func isNetworkFilesystem(fsType string) bool {
	normalized := strings.TrimSpace(strings.ToLower(fsType))
	_, found := networkFilesystems[normalized]
	return found
}
