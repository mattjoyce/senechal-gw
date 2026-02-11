package workspace

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFSWorkspaceManagerCreateAndOpen(t *testing.T) {
	baseDir := filepath.Join(t.TempDir(), "workspaces")
	mgr, err := NewFSManager(baseDir)
	if err != nil {
		t.Fatalf("NewFSManager() error = %v", err)
	}

	ws, err := mgr.Create(context.Background(), "job-a")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	wantPath := filepath.Join(baseDir, "job-a")
	if ws.Dir != wantPath {
		t.Fatalf("Create() dir = %q, want %q", ws.Dir, wantPath)
	}

	info, err := os.Stat(ws.Dir)
	if err != nil {
		t.Fatalf("Stat(workspace) error = %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("workspace path is not a directory")
	}

	opened, err := mgr.Open(context.Background(), "job-a")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if opened != ws {
		t.Fatalf("Open() workspace = %+v, want %+v", opened, ws)
	}
}

func TestFSWorkspaceManagerCloneHardlinkAndIsolation(t *testing.T) {
	baseDir := filepath.Join(t.TempDir(), "workspaces")
	mgr, err := NewFSManager(baseDir)
	if err != nil {
		t.Fatalf("NewFSManager() error = %v", err)
	}

	srcWS, err := mgr.Create(context.Background(), "job-src")
	if err != nil {
		t.Fatalf("Create(src) error = %v", err)
	}

	srcSubDir := filepath.Join(srcWS.Dir, "artifacts")
	if err := os.MkdirAll(srcSubDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(src artifacts) error = %v", err)
	}

	srcFile := filepath.Join(srcSubDir, "data.txt")
	if err := os.WriteFile(srcFile, []byte("hello"), 0o644); err != nil {
		t.Fatalf("WriteFile(src) error = %v", err)
	}

	clonedWS, err := mgr.Clone(context.Background(), "job-src", "job-dst")
	if err != nil {
		t.Fatalf("Clone() error = %v", err)
	}

	clonedFile := filepath.Join(clonedWS.Dir, "artifacts", "data.txt")
	got, err := os.ReadFile(clonedFile)
	if err != nil {
		t.Fatalf("ReadFile(cloned) error = %v", err)
	}
	if string(got) != "hello" {
		t.Fatalf("ReadFile(cloned) = %q, want %q", string(got), "hello")
	}

	srcInfo, err := os.Stat(srcFile)
	if err != nil {
		t.Fatalf("Stat(src file) error = %v", err)
	}
	clonedInfo, err := os.Stat(clonedFile)
	if err != nil {
		t.Fatalf("Stat(cloned file) error = %v", err)
	}
	if !os.SameFile(srcInfo, clonedInfo) {
		t.Fatalf("expected source and clone files to be hard-linked")
	}

	// Branch isolation requirement: deleting a file in one branch should not
	// remove the sibling branch's path.
	if err := os.Remove(clonedFile); err != nil {
		t.Fatalf("Remove(cloned file) error = %v", err)
	}
	if _, err := os.Stat(srcFile); err != nil {
		t.Fatalf("source file should still exist after clone deletion, error = %v", err)
	}

	newCloneFile := filepath.Join(clonedWS.Dir, "only-in-clone.txt")
	if err := os.WriteFile(newCloneFile, []byte("clone-local"), 0o644); err != nil {
		t.Fatalf("WriteFile(new clone file) error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(srcWS.Dir, "only-in-clone.txt")); !os.IsNotExist(err) {
		t.Fatalf("source workspace unexpectedly contains clone-only file, err = %v", err)
	}
}

func TestFSWorkspaceManagerCleanup(t *testing.T) {
	baseDir := filepath.Join(t.TempDir(), "workspaces")
	mgr, err := NewFSManager(baseDir)
	if err != nil {
		t.Fatalf("NewFSManager() error = %v", err)
	}

	oldWS, err := mgr.Create(context.Background(), "job-old")
	if err != nil {
		t.Fatalf("Create(old) error = %v", err)
	}
	newWS, err := mgr.Create(context.Background(), "job-new")
	if err != nil {
		t.Fatalf("Create(new) error = %v", err)
	}

	oldTime := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(oldWS.Dir, oldTime, oldTime); err != nil {
		t.Fatalf("Chtimes(old workspace) error = %v", err)
	}

	report, err := mgr.Cleanup(context.Background(), 24*time.Hour)
	if err != nil {
		t.Fatalf("Cleanup() error = %v", err)
	}
	if report.DeletedDirs != 1 {
		t.Fatalf("Cleanup() deleted = %d, want 1", report.DeletedDirs)
	}

	if _, err := os.Stat(oldWS.Dir); !os.IsNotExist(err) {
		t.Fatalf("old workspace should be deleted, err = %v", err)
	}
	if _, err := os.Stat(newWS.Dir); err != nil {
		t.Fatalf("new workspace should still exist, err = %v", err)
	}
}
