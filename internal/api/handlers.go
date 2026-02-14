package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/mattjoyce/ductile/internal/auth"
	"github.com/mattjoyce/ductile/internal/plugin"
	"github.com/mattjoyce/ductile/internal/protocol"
	"github.com/mattjoyce/ductile/internal/queue"
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
	startTime := time.Now()
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

	s.events.Publish("job.enqueued", map[string]any{
		"at":           time.Now().UTC().Format(time.RFC3339Nano),
		"job_id":       jobID,
		"plugin":       pluginName,
		"command":      commandName,
		"submitted_by": "api",
	})

	s.logger.Info("job enqueued via API", "job_id", jobID, "plugin", pluginName, "command", commandName)

	// Check if this trigger matches a synchronous pipeline
	triggerEvent := fmt.Sprintf("%s.%s", pluginName, commandName)
	pipeline := s.router.GetPipelineByTrigger(triggerEvent)

	// Allow query param override for async mode
	forceAsync := r.URL.Query().Get("async") == "true"

	if pipeline != nil && pipeline.ExecutionMode == "synchronous" && !forceAsync {
		// Acquire semaphore to limit concurrent synchronous requests
		select {
		case s.syncSemaphore <- struct{}{}:
			defer func() { <-s.syncSemaphore }()
		default:
			s.logger.Warn("too many concurrent synchronous requests", "job_id", jobID)
			s.writeError(w, http.StatusServiceUnavailable, "too many concurrent synchronous requests, please try again later or use async mode")
			return
		}

		// Enforce server-side maximum timeout
		waitTimeout := pipeline.Timeout
		if s.config.MaxSyncTimeout > 0 && waitTimeout > s.config.MaxSyncTimeout {
			waitTimeout = s.config.MaxSyncTimeout
		}

		s.logger.Info("waiting for synchronous pipeline", "job_id", jobID, "pipeline", pipeline.Name, "timeout", waitTimeout)

		results, err := s.waiter.WaitForJobTree(r.Context(), jobID, waitTimeout)
		if err != nil {
			// Check if it was a timeout
			if err == context.DeadlineExceeded || strings.Contains(strings.ToLower(err.Error()), "timeout") {
				respondJSON(w, http.StatusAccepted, TimeoutResponse{
					JobID:           jobID,
					Status:          "running",
					TimeoutExceeded: true,
					Message:         "Pipeline still running after timeout. Check /job/" + jobID,
				})
				return
			}
			s.logger.Error("failed to wait for job tree", "job_id", jobID, "error", err)
			s.writeError(w, http.StatusInternalServerError, "failed to wait for job completion: "+err.Error())
			return
		}

		// Success: Return aggregated results
		duration := time.Since(startTime)
		
		var tree []JobResultData
		var rootResult json.RawMessage
		finalStatus := string(queue.StatusSucceeded)

		for _, res := range results {
			if res.JobID == jobID {
				rootResult = res.Result
			}
			// If any job failed, the overall tree status is failed (for the purpose of the response)
			if res.Status == queue.StatusFailed || res.Status == queue.StatusTimedOut || res.Status == queue.StatusDead {
				finalStatus = string(res.Status)
			}

			tree = append(tree, JobResultData{
				JobID:       res.JobID,
				Plugin:      res.Plugin,
				Command:     res.Command,
				Status:      string(res.Status),
				Result:      res.Result,
				LastError:   res.LastError,
				StartedAt:   res.StartedAt,
				CompletedAt: res.CompletedAt,
			})
		}

		respondJSON(w, http.StatusOK, SyncResponse{
			JobID:      jobID,
			Status:     finalStatus,
			DurationMs: duration.Milliseconds(),
			Result:     rootResult,
			Tree:       tree,
		})
		return
	}

	// Default: Return success response immediately (async)
	resp := TriggerResponse{
		JobID:   jobID,
		Status:  "queued",
		Plugin:  pluginName,
		Command: commandName,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(resp)
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

// respondJSON is a helper to write JSON responses
func respondJSON(w http.ResponseWriter, statusCode int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(data)
}

// writeError writes a JSON error response
func (s *Server) writeError(w http.ResponseWriter, statusCode int, message string) {
	respondJSON(w, statusCode, ErrorResponse{Error: message})
}
