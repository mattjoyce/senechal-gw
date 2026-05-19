package relay

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/mattjoyce/ductile/internal/jobtree"
	"github.com/mattjoyce/ductile/internal/protocol"
	"github.com/mattjoyce/ductile/internal/router/dsl"
)

type requestError struct {
	status int
	msg    string
}

func (e *requestError) Error() string {
	return e.msg
}

// Receiver validates and accepts authenticated remote relay requests.
type Receiver struct {
	cfg      receiverConfig
	queue    JobQueuer
	router   EventRouter
	contexts EventContextStore
	logger   *slog.Logger
	now      func() time.Time

	// waiter blocks on a settled job tree for synchronous replies. Nil when
	// the dispatcher is not wired (sync replies are then refused, not silently
	// downgraded). syncSem is a relay-dedicated concurrency budget, kept
	// separate from the local API's sync semaphore on purpose.
	waiter  TreeWaiter
	syncSem chan struct{}
}

// RoutePattern returns the chi route pattern for peer-specific relay ingress.
func (r *Receiver) RoutePattern() string {
	return normalizeListenPath(r.cfg.ListenPath) + "/{peer}"
}

// HandleHTTP validates one remote relay request and converts it into local root work.
func (r *Receiver) HandleHTTP(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	accepted, err := r.accept(req)
	if err != nil {
		var reqErr *requestError
		if errors.As(err, &reqErr) {
			r.respondJSON(w, reqErr.status, ErrorResponse{Error: reqErr.msg})
			return
		}
		r.logger.Error("relay request failed", "path", req.URL.Path, "error", err)
		r.respondJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "failed to accept relay request"})
		return
	}

	enq, err := r.enqueueRootDispatches(ctx, *accepted)
	if err != nil {
		r.logger.Error("relay enqueue failed",
			"peer", accepted.Peer.Name,
			"event_type", accepted.LocalEvent.Type,
			"receiver_event_id", accepted.LocalEvent.EventID,
			"error", err,
		)
		r.respondJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "failed to enqueue relay event"})
		return
	}

	r.logger.Info("relay request accepted",
		"peer", accepted.Peer.Name,
		"event_type", accepted.LocalEvent.Type,
		"receiver_event_id", accepted.LocalEvent.EventID,
		"job_id", enq.FirstJobID,
		"origin_instance", accepted.Envelope.Origin.Instance,
		"origin_job_id", accepted.Envelope.Origin.JobID,
		"origin_event_id", accepted.Envelope.Origin.EventID,
	)

	base := AcceptanceResponse{
		Status:          "accepted",
		Peer:            accepted.Peer.Name,
		EventType:       accepted.LocalEvent.Type,
		ReceiverEventID: accepted.LocalEvent.EventID,
		JobID:           enq.FirstJobID,
	}

	if syncRequested(accepted.Envelope) {
		r.serveSyncReply(w, req, *accepted, enq, base)
		return
	}

	r.respondJSON(w, http.StatusAccepted, base)
}

// serveSyncReply blocks on the triggered pipeline tree and answers on the
// same connection. The enqueue has already happened: if the sender asked for
// sync but policy forbids it, or the event fanned out, we fail explicitly
// rather than silently degrade to fire-and-forget — the sender is waiting and
// deserves a definite answer. The enqueued jobs still run (at-least-once is
// preserved); the receiver owns them as always.
func (r *Receiver) serveSyncReply(w http.ResponseWriter, req *http.Request, accepted rootAcceptance, enq rootEnqueue, base AcceptanceResponse) {
	if !r.cfg.Sync.Enabled || !accepted.Peer.AllowSync {
		r.respondJSON(w, http.StatusUnprocessableEntity, ErrorResponse{Error: "synchronous reply not permitted for peer"})
		return
	}
	if r.waiter == nil {
		r.respondJSON(w, http.StatusServiceUnavailable, ErrorResponse{Error: "synchronous reply unavailable"})
		return
	}
	if enq.FirstJobID == "" {
		r.respondJSON(w, http.StatusUnprocessableEntity, ErrorResponse{Error: "relayed event matched no local pipeline"})
		return
	}
	if enq.DispatchCount != 1 {
		r.respondJSON(w, http.StatusUnprocessableEntity, ErrorResponse{Error: "synchronous reply requires exactly one entry dispatch"})
		return
	}

	select {
	case r.syncSem <- struct{}{}:
		defer func() { <-r.syncSem }()
	default:
		r.respondJSON(w, http.StatusServiceUnavailable, ErrorResponse{Error: "too many concurrent synchronous relay requests"})
		return
	}

	budget := clampSyncTimeout(accepted.Envelope.Reply, r.cfg.Sync.MaxTimeout)
	start := time.Now()

	results, err := r.waiter.WaitForJobTree(req.Context(), enq.FirstJobID, budget)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || strings.Contains(strings.ToLower(err.Error()), "timeout") {
			base.Status = "running"
			base.TimedOut = true
			base.DurationMs = time.Since(start).Milliseconds()
			r.respondJSON(w, http.StatusAccepted, base)
			return
		}
		r.logger.Error("relay sync wait failed",
			"peer", accepted.Peer.Name,
			"job_id", enq.FirstJobID,
			"error", err,
		)
		r.respondJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "failed to wait for relay pipeline"})
		return
	}

	terminalSteps := r.terminalSteps(enq.PipelineName)
	outcome := jobtree.Aggregate(results, enq.FirstJobID, terminalSteps)

	base.Status = outcome.Status
	base.Result = outcome.FinalResult
	base.Tree = outcome.Tree
	base.DurationMs = time.Since(start).Milliseconds()
	r.respondJSON(w, http.StatusOK, base)
}

func (r *Receiver) terminalSteps(pipelineName string) map[string]struct{} {
	out := make(map[string]struct{})
	if r.router == nil || strings.TrimSpace(pipelineName) == "" {
		return out
	}
	for _, route := range r.router.GetCompiledRoutes(pipelineName) {
		if route.Destination.Kind != dsl.CompiledRouteDestinationTerminal {
			continue
		}
		if stepID := strings.TrimSpace(route.Source.StepID); stepID != "" {
			out[stepID] = struct{}{}
		}
	}
	if len(out) > 0 {
		return out
	}
	if info := r.router.GetPipelineByName(pipelineName); info != nil {
		for _, stepID := range info.TerminalStepIDs {
			if stepID = strings.TrimSpace(stepID); stepID != "" {
				out[stepID] = struct{}{}
			}
		}
	}
	return out
}

func (r *Receiver) accept(req *http.Request) (*rootAcceptance, error) {
	peerName := strings.TrimSpace(chi.URLParam(req, "peer"))
	if peerName == "" {
		return nil, &requestError{status: http.StatusForbidden, msg: "unknown relay peer"}
	}

	peer, ok := r.cfg.Peers[peerName]
	if !ok {
		return nil, &requestError{status: http.StatusForbidden, msg: "unknown relay peer"}
	}

	headerPeer := strings.TrimSpace(req.Header.Get(HeaderPeer))
	if headerPeer == "" || headerPeer != peerName {
		return nil, &requestError{status: http.StatusForbidden, msg: "peer identity mismatch"}
	}

	timestampHeader := strings.TrimSpace(req.Header.Get(HeaderTimestamp))
	if timestampHeader == "" {
		return nil, &requestError{status: http.StatusForbidden, msg: "missing relay timestamp"}
	}
	timestamp, err := parseRelayTimestamp(timestampHeader)
	if err != nil {
		return nil, &requestError{status: http.StatusForbidden, msg: "invalid relay timestamp"}
	}
	now := nowUTC()
	if r.now != nil {
		now = r.now().UTC()
	}
	skew := now.Sub(timestamp)
	if skew < 0 {
		skew = -skew
	}
	if skew > r.cfg.AllowedClockSkew {
		return nil, &requestError{status: http.StatusForbidden, msg: "relay timestamp outside allowed clock skew"}
	}

	keyID := strings.TrimSpace(req.Header.Get(HeaderKeyID))
	if r.cfg.RequireKeyID && keyID == "" {
		return nil, &requestError{status: http.StatusForbidden, msg: "missing relay key id"}
	}
	if peer.KeyID != "" && keyID != peer.KeyID {
		return nil, &requestError{status: http.StatusForbidden, msg: "relay key id mismatch"}
	}

	signature := strings.TrimSpace(req.Header.Get(HeaderSignature))
	if signature == "" {
		return nil, &requestError{status: http.StatusForbidden, msg: "missing relay signature"}
	}

	limitedReader := io.LimitReader(req.Body, r.cfg.MaxBodySize+1)
	body, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, &requestError{status: http.StatusBadRequest, msg: "failed to read request body"}
	}
	if int64(len(body)) > r.cfg.MaxBodySize {
		return nil, &requestError{status: http.StatusRequestEntityTooLarge, msg: "relay payload too large"}
	}

	if err := verifySignature(req.Method, req.URL.Path, timestampHeader, body, signature, peer.Secret); err != nil {
		return nil, &requestError{status: http.StatusForbidden, msg: "forbidden"}
	}

	envelope, err := validateEnvelope(body)
	if err != nil {
		return nil, &requestError{status: http.StatusBadRequest, msg: err.Error()}
	}
	if origin := strings.TrimSpace(envelope.Origin.Instance); origin != "" && origin != peerName {
		return nil, &requestError{status: http.StatusForbidden, msg: "origin instance mismatch"}
	}
	if len(peer.Accept) > 0 {
		if _, ok := peer.Accept[envelope.Event.Type]; !ok {
			return nil, &requestError{status: http.StatusUnprocessableEntity, msg: "event type not allowed for peer"}
		}
	}

	// Use the caller-supplied dedupe key when present. If absent, derive one
	// from envelope identity so a replay of the same signed envelope within the
	// clock-skew window cannot enqueue a second job. Derivation requires
	// origin.event_id; if that field is empty the caller has not opted into
	// replay protection and we accept the risk rather than silently dropping.
	dedupeKey := strings.TrimSpace(envelope.Event.DedupeKey)
	if dedupeKey == "" {
		if originEventID := strings.TrimSpace(envelope.Origin.EventID); originEventID != "" {
			dedupeKey = "relay:" + peerName + ":" + strings.TrimSpace(envelope.Origin.Instance) + ":" + originEventID
		}
	}

	localEvent := protocol.Event{
		Type:      envelope.Event.Type,
		Payload:   cloneMap(envelope.Event.Payload),
		DedupeKey: dedupeKey,
		Source:    "relay:" + peer.Name,
		Timestamp: now,
		EventID:   uuid.NewString(),
	}

	return &rootAcceptance{
		LocalEvent: localEvent,
		Peer:       peer,
		Envelope:   *envelope,
	}, nil
}

func (r *Receiver) respondJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		r.logger.Error("failed to write relay response", "error", err)
	}
}

func cloneMap(in map[string]any) map[string]any {
	if in == nil {
		return map[string]any{}
	}
	raw, err := json.Marshal(in)
	if err != nil {
		out := make(map[string]any, len(in))
		for key, value := range in {
			out[key] = value
		}
		return out
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		out = make(map[string]any, len(in))
		for key, value := range in {
			out[key] = value
		}
	}
	return out
}
