package workspace

import (
	"context"
	"time"
)

// Workspace describes a job-scoped data-plane directory in the Governance
// Hybrid model.
//
// The control plane stores job/context identifiers in SQLite; absolute paths stay
// in the workspace manager so the data directory can move without DB rewrites.
type Workspace struct {
	JobID string
	Dir   string
}

// CleanupReport summarizes a cleanup run.
type CleanupReport struct {
	DeletedDirs int
}

// Manager governs artifact workspace lifecycle for the Governance Hybrid data
// plane.
type Manager interface {
	// Create initializes a new workspace for jobID.
	Create(ctx context.Context, jobID string) (Workspace, error)

	// Clone creates dstJobID as an isolated copy of srcJobID (hard-link strategy).
	Clone(ctx context.Context, srcJobID, dstJobID string) (Workspace, error)

	// Open resolves an existing workspace for jobID.
	Open(ctx context.Context, jobID string) (Workspace, error)

	// Cleanup removes stale workspaces older than olderThan.
	Cleanup(ctx context.Context, olderThan time.Duration) (CleanupReport, error)
}
