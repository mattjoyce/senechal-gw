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

	jobID := "ab1234567890"
	ws, err := mgr.Create(context.Background(), jobID)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	wantPath := filepath.Join(baseDir, "ab", jobID)
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

	opened, err := mgr.Open(context.Background(), jobID)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if opened != ws {
		t.Fatalf("Open() workspace = %+v, want %+v", opened, ws)
	}
}

func TestFSWorkspaceManagerShardLayout(t *testing.T) {
	baseDir := filepath.Join(t.TempDir(), "workspaces")
	mgr, err := NewFSManager(baseDir)
	if err != nil {
		t.Fatalf("NewFSManager() error = %v", err)
	}

	// Two jobs with different prefixes go into different shard dirs.
	jobA := "aa000000000000000000000000000000"
	jobB := "bb000000000000000000000000000000"

	wsA, err := mgr.Create(context.Background(), jobA)
	if err != nil {
		t.Fatalf("Create(A) error = %v", err)
	}
	wsB, err := mgr.Create(context.Background(), jobB)
	if err != nil {
		t.Fatalf("Create(B) error = %v", err)
	}

	wantA := filepath.Join(baseDir, "aa", jobA)
	wantB := filepath.Join(baseDir, "bb", jobB)
	if wsA.Dir != wantA {
		t.Errorf("Create(A).Dir = %q, want %q", wsA.Dir, wantA)
	}
	if wsB.Dir != wantB {
		t.Errorf("Create(B).Dir = %q, want %q", wsB.Dir, wantB)
	}

	// Verify shard directories exist.
	for _, shardDir := range []string{
		filepath.Join(baseDir, "aa"),
		filepath.Join(baseDir, "bb"),
	} {
		if info, err := os.Stat(shardDir); err != nil || !info.IsDir() {
			t.Errorf("shard dir %q should exist as a directory", shardDir)
		}
	}
}

func TestFSWorkspaceManagerShortJobIDRejected(t *testing.T) {
	baseDir := filepath.Join(t.TempDir(), "workspaces")
	mgr, err := NewFSManager(baseDir)
	if err != nil {
		t.Fatalf("NewFSManager() error = %v", err)
	}

	_, err = mgr.Create(context.Background(), "x")
	if err == nil {
		t.Fatal("Create() with 1-char ID should return error")
	}
}

func TestFSWorkspaceManagerCloneHardlinkAndIsolation(t *testing.T) {
	baseDir := filepath.Join(t.TempDir(), "workspaces")
	mgr, err := NewFSManager(baseDir)
	if err != nil {
		t.Fatalf("NewFSManager() error = %v", err)
	}

	srcID := "aa-job-src-0000000000000000000000"
	dstID := "bb-job-dst-0000000000000000000000"

	srcWS, err := mgr.Create(context.Background(), srcID)
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

	clonedWS, err := mgr.Clone(context.Background(), srcID, dstID)
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

	oldID := "ab000000000000000000000000000000"
	newID := "cd000000000000000000000000000000"

	oldWS, err := mgr.Create(context.Background(), oldID)
	if err != nil {
		t.Fatalf("Create(old) error = %v", err)
	}
	newWS, err := mgr.Create(context.Background(), newID)
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

func TestFSWorkspaceManagerCleanupPrunesEmptyShard(t *testing.T) {
	baseDir := filepath.Join(t.TempDir(), "workspaces")
	mgr, err := NewFSManager(baseDir)
	if err != nil {
		t.Fatalf("NewFSManager() error = %v", err)
	}

	jobID := "ef000000000000000000000000000000"
	ws, err := mgr.Create(context.Background(), jobID)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	shardDir := filepath.Join(baseDir, "ef")

	// Age the workspace so it gets deleted.
	oldTime := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(ws.Dir, oldTime, oldTime); err != nil {
		t.Fatalf("Chtimes() error = %v", err)
	}

	report, err := mgr.Cleanup(context.Background(), 24*time.Hour)
	if err != nil {
		t.Fatalf("Cleanup() error = %v", err)
	}
	if report.DeletedDirs != 1 {
		t.Fatalf("Cleanup() deleted = %d, want 1", report.DeletedDirs)
	}

	// The workspace dir is gone.
	if _, err := os.Stat(ws.Dir); !os.IsNotExist(err) {
		t.Fatalf("workspace should be deleted, err = %v", err)
	}
	// The shard dir should also be pruned since it is now empty.
	if _, err := os.Stat(shardDir); !os.IsNotExist(err) {
		t.Fatalf("empty shard dir should be pruned, err = %v", err)
	}
}
