package relay

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/mattjoyce/ductile/internal/jobtree"
	"github.com/mattjoyce/ductile/internal/protocol"
	"github.com/mattjoyce/ductile/internal/queue"
	"github.com/mattjoyce/ductile/internal/router"
	"github.com/mattjoyce/ductile/internal/router/dsl"
	"github.com/mattjoyce/ductile/internal/state"
)

// Relay transport headers.
const (
	HeaderPeer      = "X-Ductile-Peer"
	HeaderKeyID     = "X-Ductile-Key-Id"
	HeaderTimestamp = "X-Ductile-Timestamp"
	HeaderSignature = "X-Ductile-Signature"
)

// Envelope is the on-wire remote relay request body.
type Envelope struct {
	Event   EnvelopeEvent  `json:"event"`
	Origin  EnvelopeOrigin `json:"origin,omitempty"`
	Baggage map[string]any `json:"baggage,omitempty"`
	// Reply, when present with Mode "sync", asks the receiver to block on the
	// triggered pipeline tree and return its result on this same connection.
	// It lives inside the signed body deliberately: reply intent is part of
	// the message, not an unauthenticated side channel.
	Reply *EnvelopeReply `json:"reply,omitempty"`
}

// EnvelopeReply expresses how the sender wants the receiver to answer.
type EnvelopeReply struct {
	// Mode is "" (async, fire-and-forget — the default) or "sync".
	Mode string `json:"mode,omitempty"`
	// Timeout is the sender's requested wait budget (e.g. "30s"). The
	// receiver clamps this to its own configured maximum; the receiver is
	// authoritative.
	Timeout string `json:"timeout,omitempty"`
}

// SyncReplyMode is the only non-async reply mode.
const SyncReplyMode = "sync"

// EnvelopeEvent is the immutable relayed event value.
type EnvelopeEvent struct {
	Type      string         `json:"type"`
	Payload   map[string]any `json:"payload"`
	DedupeKey string         `json:"dedupe_key,omitempty"`
}

// EnvelopeOrigin carries sender-side observability metadata.
type EnvelopeOrigin struct {
	Instance string `json:"instance"`
	Plugin   string `json:"plugin,omitempty"`
	JobID    string `json:"job_id,omitempty"`
	EventID  string `json:"event_id,omitempty"`
}

// AcceptanceResponse describes a validated relay request.
//
// One value shape carries both outcomes. For a fire-and-forget relay it
// reports Status "accepted" and the enqueued JobID. For a synchronous reply
// it additionally carries the settled tree's Status (succeeded/failed/...),
// the final Result, the Tree, and DurationMs — or, on a wait timeout,
// Status "running" with TimedOut set. The mode is discriminated by Status,
// so neither sender callers nor the wire needed a second response type.
type AcceptanceResponse struct {
	Status          string `json:"status"`
	Peer            string `json:"peer"`
	EventType       string `json:"event_type"`
	ReceiverEventID string `json:"receiver_event_id"`
	JobID           string `json:"job_id,omitempty"`

	// Synchronous-reply fields (present only when reply.mode == "sync").
	Result     json.RawMessage `json:"result,omitempty"`
	Tree       []jobtree.Node  `json:"tree,omitempty"`
	DurationMs int64           `json:"duration_ms,omitempty"`
	TimedOut   bool            `json:"timed_out,omitempty"`
}

// ErrorResponse reports a relay request error.
type ErrorResponse struct {
	Error string `json:"error"`
}

type remoteInstance struct {
	Name        string
	BaseURL     string
	IngressPath string
	Secret      string
	KeyID       string
	Timeout     time.Duration
	SyncTimeout time.Duration
	Allow       map[string]struct{}
}

type trustedPeer struct {
	Name        string
	Secret      string
	KeyID       string
	Accept      map[string]struct{}
	AllowedBags map[string]struct{}
	AllowSync   bool
}

// syncPolicy is the receiver-authoritative synchronous-reply budget.
// MaxConcurrent is enforced by a dedicated semaphore on the Receiver,
// kept separate from the local API's sync budget so a remote peer cannot
// starve local synchronous callers.
type syncPolicy struct {
	Enabled    bool
	MaxTimeout time.Duration
}

type senderConfig struct {
	ServiceName string
	Instances   map[string]remoteInstance
}

type receiverConfig struct {
	ListenPath       string
	MaxBodySize      int64
	AllowedClockSkew time.Duration
	RequireKeyID     bool
	Peers            map[string]trustedPeer
	Sync             syncPolicy
}

// JobQueuer enqueues relay-triggered jobs.
type JobQueuer interface {
	Enqueue(ctx context.Context, req queue.EnqueueRequest) (string, error)
}

// EventRouter resolves root relay events into local jobs.
type EventRouter interface {
	Next(ctx context.Context, req router.Request) ([]router.Dispatch, error)
	GetNode(pipelineName string, stepID string) (dsl.Node, bool)
	// GetCompiledRoutes and GetPipelineByName let the synchronous-reply path
	// identify the triggered pipeline's terminal steps so it reports the same
	// "final result" the local sync API would.
	GetCompiledRoutes(pipelineName string) []dsl.CompiledRoute
	GetPipelineByName(name string) *router.PipelineInfo
}

// TreeWaiter blocks until a root job and all descendants settle, or timeout.
// The dispatcher implements this; relay holds only the narrow interface so
// it never imports the dispatcher (which already imports relay).
type TreeWaiter interface {
	WaitForJobTree(ctx context.Context, rootJobID string, timeout time.Duration) ([]*queue.JobResult, error)
}

// EventContextStore persists root-entry baggage and route metadata.
type EventContextStore interface {
	Create(ctx context.Context, parentID *string, pipelineName string, stepID string, updates json.RawMessage) (*state.EventContext, error)
	Get(ctx context.Context, id string) (*state.EventContext, error)
}

type rootAcceptance struct {
	LocalEvent protocol.Event
	Peer       trustedPeer
	Envelope   Envelope
}

func defaultLogger(logger *slog.Logger) *slog.Logger {
	if logger != nil {
		return logger
	}
	return slog.Default()
}
