package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/mattjoyce/senechal-gw/internal/auth"
	"github.com/mattjoyce/senechal-gw/internal/plugin"
	"github.com/mattjoyce/senechal-gw/internal/protocol"
	"github.com/mattjoyce/senechal-gw/internal/queue"
)

// handleHealthz handles GET /healthz (no auth).
func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	depth, err := s.queue.Depth(r.Context())
	if err != nil {
		s.logger.Error("failed to compute queue depth", "error", err)
		s.writeError(w, http.StatusInternalServerError, "failed to compute queue depth")
		return
	}

	resp := HealthzResponse{
		Status:             "ok",
		UptimeSeconds:      int64(time.Since(s.startedAt).Seconds()),
		QueueDepth:         depth,
		PluginsLoaded:      len(s.registry.All()),
		PluginsCircuitOpen: 0,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

// handleTrigger handles POST /trigger/{plugin}/{command}
func (s *Server) handleTrigger(w http.ResponseWriter, r *http.Request) {
	pluginName := chi.URLParam(r, "plugin")
	commandName := chi.URLParam(r, "command")

	// Validate plugin exists
	plug, ok := s.registry.Get(pluginName)
	if !ok {
		s.writeError(w, http.StatusBadRequest, "plugin not found")
		return
	}

	// Validate command is supported by plugin
	if !plug.SupportsCommand(commandName) {
		s.writeError(w, http.StatusBadRequest, "command not supported by plugin")
		return
	}

	// Enforce token scopes. plugin:ro may only invoke read commands.
	principal, _ := auth.PrincipalFromContext(r.Context())
	cmdType, _ := plug.CommandTypeFor(commandName)
	if cmdType == plugin.CommandTypeRead {
		// already allowed by route middleware
	} else {
		if !auth.HasAnyScope(principal, "plugin:rw", "*") {
			s.writeError(w, http.StatusForbidden, "insufficient scope")
			return
		}
	}

	// Parse request body (optional payload)
	var req TriggerRequest
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
	}

	// For handle commands, wrap payload in Event envelope so the dispatcher
	// can unmarshal it the same way as routed events.
	enqueuePayload := req.Payload
	if commandName == "handle" {
		event := protocol.Event{
			Type:    "api.trigger",
			Payload: make(map[string]any),
		}
		if len(req.Payload) > 0 {
			if err := json.Unmarshal(req.Payload, &event.Payload); err != nil {
				s.writeError(w, http.StatusBadRequest, "payload must be a JSON object")
				return
			}
		}
		enqueuePayload, _ = json.Marshal(event)
	}

	// Enqueue job
	jobID, err := s.queue.Enqueue(r.Context(), queue.EnqueueRequest{
		Plugin:      pluginName,
		Command:     commandName,
		Payload:     enqueuePayload,
		SubmittedBy: "api",
	})
	if err != nil {
		s.logger.Error("failed to enqueue job", "plugin", pluginName, "command", commandName, "error", err)
		s.writeError(w, http.StatusInternalServerError, "failed to enqueue job")
		return
	}

	s.events.Publish("job_enqueued", map[string]any{
		"at":           time.Now().UTC().Format(time.RFC3339Nano),
		"job_id":       jobID,
		"plugin":       pluginName,
		"command":      commandName,
		"submitted_by": "api",
	})

	// Return success response
	resp := TriggerResponse{
		JobID:   jobID,
		Status:  "queued",
		Plugin:  pluginName,
		Command: commandName,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(resp)

	s.logger.Info("job enqueued via API", "job_id", jobID, "plugin", pluginName, "command", commandName)
}

// handleGetJob handles GET /job/{jobID}
func (s *Server) handleGetJob(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "jobID")

	// Retrieve job from queue
	job, err := s.queue.GetJobByID(r.Context(), jobID)
	if err != nil {
		if err == queue.ErrJobNotFound {
			s.writeError(w, http.StatusNotFound, "job not found")
			return
		}
		s.logger.Error("failed to retrieve job", "job_id", jobID, "error", err)
		s.writeError(w, http.StatusInternalServerError, "failed to retrieve job")
		return
	}

	// Build response (using Codex's JobResult type with result field)
	resp := JobStatusResponse{
		JobID:       job.JobID,
		Status:      string(job.Status),
		Plugin:      job.Plugin,
		Command:     job.Command,
		Result:      job.Result,
		StartedAt:   job.StartedAt,
		CompletedAt: job.CompletedAt,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

// writeError writes a JSON error response
func (s *Server) writeError(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(ErrorResponse{Error: message})
}
