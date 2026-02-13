package lock

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAcquirePIDLockWritesPID(t *testing.T) {
	t.Parallel()

	lockPath := filepath.Join(t.TempDir(), "ductile.lock")
	l, err := AcquirePIDLock(lockPath)
	if err != nil {
		t.Fatalf("AcquirePIDLock: %v", err)
	}
	t.Cleanup(func() { _ = l.Release() })

	b, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if strings.TrimSpace(string(b)) == "" {
		t.Fatalf("expected PID in lock file, got empty")
	}
}
