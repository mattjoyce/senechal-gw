package workspace

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// fsWorkspaceManager manages per-job workspace directories on local disk.
type fsWorkspaceManager struct {
	baseDir string
	now     func() time.Time
}

var _ Manager = (*fsWorkspaceManager)(nil)

// NewFSManager creates a filesystem-backed workspace manager rooted at baseDir.
func NewFSManager(baseDir string) (*fsWorkspaceManager, error) {
	trimmed := strings.TrimSpace(baseDir)
	if trimmed == "" {
		return nil, fmt.Errorf("workspace base directory is empty")
	}

	return &fsWorkspaceManager{
		baseDir: filepath.Clean(trimmed),
		now:     time.Now,
	}, nil
}

// Create initializes a workspace directory for jobID.
func (m *fsWorkspaceManager) Create(ctx context.Context, jobID string) (Workspace, error) {
	if err := ctx.Err(); err != nil {
		return Workspace{}, err
	}

	path, err := m.workspacePath(jobID)
	if err != nil {
		return Workspace{}, err
	}

	if err := os.MkdirAll(m.baseDir, 0o755); err != nil {
		return Workspace{}, fmt.Errorf("create workspace base directory: %w", err)
	}

	if err := os.Mkdir(path, 0o755); err != nil {
		return Workspace{}, fmt.Errorf("create workspace for job %q: %w", jobID, err)
	}

	return Workspace{JobID: jobID, Dir: path}, nil
}

// Clone creates a new workspace for dstJobID by hard-linking regular files from
// srcJobID's workspace.
func (m *fsWorkspaceManager) Clone(ctx context.Context, srcJobID, dstJobID string) (Workspace, error) {
	if err := ctx.Err(); err != nil {
		return Workspace{}, err
	}
	if srcJobID == dstJobID {
		return Workspace{}, fmt.Errorf("source and destination job IDs must differ")
	}

	src, err := m.Open(ctx, srcJobID)
	if err != nil {
		return Workspace{}, fmt.Errorf("open source workspace: %w", err)
	}

	dstPath, err := m.workspacePath(dstJobID)
	if err != nil {
		return Workspace{}, err
	}

	if _, err := os.Stat(dstPath); err == nil {
		return Workspace{}, fmt.Errorf("destination workspace for job %q already exists", dstJobID)
	} else if !os.IsNotExist(err) {
		return Workspace{}, fmt.Errorf("stat destination workspace for job %q: %w", dstJobID, err)
	}

	if err := m.cloneTreeWithHardLinks(ctx, src.Dir, dstPath); err != nil {
		_ = os.RemoveAll(dstPath)
		return Workspace{}, fmt.Errorf("clone workspace %q to %q: %w", srcJobID, dstJobID, err)
	}

	return Workspace{JobID: dstJobID, Dir: dstPath}, nil
}

// Open returns metadata for an existing workspace directory.
func (m *fsWorkspaceManager) Open(ctx context.Context, jobID string) (Workspace, error) {
	if err := ctx.Err(); err != nil {
		return Workspace{}, err
	}

	path, err := m.workspacePath(jobID)
	if err != nil {
		return Workspace{}, err
	}

	info, err := os.Stat(path)
	if err != nil {
		return Workspace{}, fmt.Errorf("open workspace for job %q: %w", jobID, err)
	}
	if !info.IsDir() {
		return Workspace{}, fmt.Errorf("workspace path for job %q is not a directory", jobID)
	}

	return Workspace{JobID: jobID, Dir: path}, nil
}

// Cleanup removes workspace directories older than olderThan based on directory
// modification time.
func (m *fsWorkspaceManager) Cleanup(ctx context.Context, olderThan time.Duration) (CleanupReport, error) {
	if err := ctx.Err(); err != nil {
		return CleanupReport{}, err
	}
	if olderThan <= 0 {
		return CleanupReport{}, fmt.Errorf("olderThan must be positive")
	}

	entries, err := os.ReadDir(m.baseDir)
	if os.IsNotExist(err) {
		return CleanupReport{}, nil
	}
	if err != nil {
		return CleanupReport{}, fmt.Errorf("read workspace base directory: %w", err)
	}

	cutoff := m.now().Add(-olderThan)
	report := CleanupReport{}

	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return report, err
		}
		if !entry.IsDir() {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			return report, fmt.Errorf("read workspace entry info %q: %w", entry.Name(), err)
		}
		if info.ModTime().After(cutoff) {
			continue
		}

		path := filepath.Join(m.baseDir, entry.Name())
		if err := os.RemoveAll(path); err != nil {
			return report, fmt.Errorf("remove workspace %q: %w", entry.Name(), err)
		}
		report.DeletedDirs++
	}

	return report, nil
}

func (m *fsWorkspaceManager) workspacePath(jobID string) (string, error) {
	if err := validateJobID(jobID); err != nil {
		return "", err
	}
	return filepath.Join(m.baseDir, jobID), nil
}

func (m *fsWorkspaceManager) cloneTreeWithHardLinks(ctx context.Context, srcDir, dstDir string) error {
	srcInfo, err := os.Stat(srcDir)
	if err != nil {
		return fmt.Errorf("stat source directory: %w", err)
	}
	if !srcInfo.IsDir() {
		return fmt.Errorf("source path %q is not a directory", srcDir)
	}

	if err := os.MkdirAll(filepath.Dir(dstDir), 0o755); err != nil {
		return fmt.Errorf("create destination parent: %w", err)
	}
	if err := os.Mkdir(dstDir, srcInfo.Mode().Perm()); err != nil {
		return fmt.Errorf("create destination directory: %w", err)
	}

	return filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == srcDir {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}

		relPath, err := filepath.Rel(srcDir, path)
		if err != nil {
			return fmt.Errorf("resolve relative path: %w", err)
		}
		dstPath := filepath.Join(dstDir, relPath)

		info, err := d.Info()
		if err != nil {
			return fmt.Errorf("read entry info for %q: %w", path, err)
		}

		switch {
		case d.IsDir():
			if err := os.Mkdir(dstPath, info.Mode().Perm()); err != nil {
				return fmt.Errorf("create directory %q: %w", dstPath, err)
			}
		case info.Mode().IsRegular():
			if err := os.Link(path, dstPath); err != nil {
				return fmt.Errorf("hard-link %q to %q: %w", path, dstPath, err)
			}
		case info.Mode()&os.ModeSymlink != 0:
			target, err := os.Readlink(path)
			if err != nil {
				return fmt.Errorf("read symlink %q: %w", path, err)
			}
			if err := os.Symlink(target, dstPath); err != nil {
				return fmt.Errorf("create symlink %q: %w", dstPath, err)
			}
		default:
			return fmt.Errorf("unsupported file type for %q (%s)", path, info.Mode().Type())
		}

		return nil
	})
}

func validateJobID(jobID string) error {
	trimmed := strings.TrimSpace(jobID)
	if trimmed == "" {
		return fmt.Errorf("jobID is empty")
	}
	if trimmed == "." || trimmed == ".." {
		return fmt.Errorf("jobID %q is invalid", jobID)
	}
	if strings.Contains(trimmed, "/") || strings.Contains(trimmed, `\`) {
		return fmt.Errorf("jobID %q must not contain path separators", jobID)
	}
	if filepath.Clean(trimmed) != trimmed {
		return fmt.Errorf("jobID %q is invalid", jobID)
	}
	return nil
}
