package storage

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateSQLiteFilesystemWithDetector_AllowsLocalFS(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "state.db")
	err := validateSQLiteFilesystemWithDetector(dbPath, func(path string) (string, error) {
		return "apfs", nil
	})
	if err != nil {
		t.Fatalf("expected local filesystem to pass, got: %v", err)
	}
}

func TestValidateSQLiteFilesystemWithDetector_RejectsNetworkFS(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "state.db")
	err := validateSQLiteFilesystemWithDetector(dbPath, func(path string) (string, error) {
		return "smbfs", nil
	})
	if err == nil {
		t.Fatal("expected network filesystem validation error")
	}

	msg := err.Error()
	for _, want := range []string{"smbfs", "SQLite requires a local filesystem", "--db /path/to/local/file.db"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("expected error to contain %q, got %q", want, msg)
		}
	}
}

func TestValidateSQLiteFilesystemWithDetector_UsesNearestExistingPath(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dbPath := filepath.Join(root, "nested", "dir", "state.db")

	var inspectedPath string
	err := validateSQLiteFilesystemWithDetector(dbPath, func(path string) (string, error) {
		inspectedPath = path
		return "apfs", nil
	})
	if err != nil {
		t.Fatalf("expected local filesystem to pass, got: %v", err)
	}

	if inspectedPath != root {
		t.Fatalf("expected detector to inspect nearest existing path %q, got %q", root, inspectedPath)
	}
}

func TestIsNetworkFilesystem(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		fs   string
		want bool
	}{
		{name: "nfs", fs: "nfs", want: true},
		{name: "smbfs uppercase", fs: "SMBFS", want: true},
		{name: "local apfs", fs: "apfs", want: false},
		{name: "hex linux magic", fs: "0x6969", want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := isNetworkFilesystem(tc.fs)
			if got != tc.want {
				t.Fatalf("isNetworkFilesystem(%q)=%v, want %v", tc.fs, got, tc.want)
			}
		})
	}
}
