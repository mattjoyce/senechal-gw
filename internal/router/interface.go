package router

import (
	"context"
	"time"

	"github.com/mattjoyce/ductile/internal/protocol"
)

// PipelineInfo describes a loaded pipeline's execution properties.
type PipelineInfo struct {
	Name            string
	Trigger         string
	EntryStepID     string
	TerminalStepIDs []string
	ExecutionMode   string
	Timeout         time.Duration
}

// Request captures the control-plane inputs needed to calculate fan-out in the
// Governance Hybrid model.
//
// The router works only with metadata and event envelopes; artifact cloning and
// workspace lifecycle remain in the workspace manager (data plane).
type Request struct {
	SourcePlugin    string
	SourceJobID     string
	SourceContextID string
	SourcePipeline  string
	SourceStepID    string
	SourceEventID   string
	Event           protocol.Event
}

// Dispatch describes one downstream job to enqueue from a routing decision.
type Dispatch struct {
	Plugin          string
	Command         string
	Event           protocol.Event
	PipelineName    string
	StepID          string
	ParentJobID     string
	ParentContextID string
	SourceEventID   string
}

// Engine maps an emitted event to downstream dispatches.
type Engine interface {
	Next(ctx context.Context, req Request) ([]Dispatch, error)
	// GetPipelineByTrigger returns the first pipeline matched by a trigger event.
	GetPipelineByTrigger(trigger string) *PipelineInfo
}
