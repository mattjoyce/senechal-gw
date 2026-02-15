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

// handlePipelineTrigger handles POST /pipeline/{pipeline}
// Explicitly triggers a named pipeline.
func (s *Server) handlePipelineTrigger(w http.ResponseWriter, r *http.Request) {
	pipelineName := chi.URLParam(r, "pipeline")

	pipeline := s.router.GetPipelineByName(pipelineName)
	if pipeline == nil {
		s.writeError(w, http.StatusNotFound, "pipeline not found")
		return
	}

	principal, _ := auth.PrincipalFromContext(r.Context())
	if !auth.HasAnyScope(principal, "plugin:rw", "*") {
		s.writeError(w, http.StatusForbidden, "insufficient scope to trigger pipeline")
		return
	}

	var req TriggerRequest
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
	}

	// Prepare event for pipeline entry using the actual pipeline trigger type
	event := protocol.Event{
		Type:    pipeline.Trigger,
		Payload: make(map[string]any),
	}
	if len(req.Payload) > 0 {
		if err := json.Unmarshal(req.Payload, &event.Payload); err != nil {
			s.writeError(w, http.StatusBadRequest, "payload must be a JSON object")
			return
		}
	}

	// Resolve entry dispatches
	dispatches, err := s.router.GetEntryDispatches(pipelineName, event)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "failed to resolve pipeline entry: "+err.Error())
		return
	}
	if len(dispatches) == 0 {
		s.writeError(w, http.StatusInternalServerError, "pipeline has no entry points")
		return
	}

	startTime := time.Now()
	var firstJobID string
	var enqueuedIDs []string

	// Enqueue each entry point
	for _, d := range dispatches {
		// Create a unique event context for each entry step to preserve lineage/step identity
		var eventContextID *string
		if s.contextStore != nil {
			// Each entry step might be different in a split-root pipeline
			ctxRec, err := s.contextStore.Create(r.Context(), nil, pipeline.Name, d.StepID, json.RawMessage(`{}`))
			if err != nil {
				s.logger.Error("failed to create event context for pipeline step", "pipeline", pipeline.Name, "step_id", d.StepID, "error", err)
				s.writeError(w, http.StatusInternalServerError, "failed to create event context")
				// Atomicity: would be better to "undo" previous enqueues, but for now we hard-fail
				return
			}
			eventContextID = &ctxRec.ID
		}

		// Wrap payload for handle command
		enqueuePayload := req.Payload
		if d.Command == "handle" {
			enqueuePayload, _ = json.Marshal(event)
		}

		jobID, err := s.queue.Enqueue(r.Context(), queue.EnqueueRequest{
			Plugin:         d.Plugin,
			Command:        d.Command,
			Payload:        enqueuePayload,
			SubmittedBy:    "api",
			EventContextID: eventContextID,
		})
		if err != nil {
			s.logger.Error("failed to enqueue pipeline job", "pipeline", pipelineName, "plugin", d.Plugin, "error", err)
			s.writeError(w, http.StatusInternalServerError, "failed to enqueue job (partial start prevented)")
			return
		}
		if firstJobID == "" {
			firstJobID = jobID
		}
		enqueuedIDs = append(enqueuedIDs, jobID)
	}

	s.events.Publish("pipeline.enqueued", map[string]any{
		"at":           time.Now().UTC().Format(time.RFC3339Nano),
		"job_id":       firstJobID,
		"job_ids":      enqueuedIDs,
		"pipeline":     pipelineName,
		"submitted_by": "api",
	})

	s.logger.Info("pipeline enqueued via API", "job_id", firstJobID, "pipeline", pipelineName)

	forceAsync := r.URL.Query().Get("async") == "true"
	if pipeline.ExecutionMode == "synchronous" && !forceAsync {
		select {
		case s.syncSemaphore <- struct{}{}:
			defer func() { <-s.syncSemaphore }()
		default:
			s.writeError(w, http.StatusServiceUnavailable, "too many concurrent synchronous requests")
			return
		}

		waitTimeout := pipeline.Timeout
		if s.config.MaxSyncTimeout > 0 && waitTimeout > s.config.MaxSyncTimeout {
			waitTimeout = s.config.MaxSyncTimeout
		}

		results, err := s.waiter.WaitForJobTree(r.Context(), firstJobID, waitTimeout)
		if err != nil {
			if err == context.DeadlineExceeded || strings.Contains(strings.ToLower(err.Error()), "timeout") {
				respondJSON(w, http.StatusAccepted, TimeoutResponse{
					JobID:           firstJobID,
					Status:          "running",
					TimeoutExceeded: true,
					Message:         "Pipeline still running after timeout.",
				})
				return
			}
			s.writeError(w, http.StatusInternalServerError, "wait failed: "+err.Error())
			return
		}

		// Process results (Fixed consistency with handleTrigger)
		var tree []JobResultData
		var rootResult json.RawMessage
		finalStatus := string(queue.StatusSucceeded)
		var terminalResult json.RawMessage

		for _, res := range results {
			if res.JobID == firstJobID {
				rootResult = res.Result
			}
			// If any job failed, the overall tree status is failed (for the purpose of the response)
			if res.Status == queue.StatusFailed || res.Status == queue.StatusTimedOut || res.Status == queue.StatusDead {
				finalStatus = string(res.Status)
			}
			
			for _, termStepID := range pipeline.TerminalStepIDs {
				if res.StepID == termStepID {
					terminalResult = res.Result
				}
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

		finalResult := terminalResult
		if finalResult == nil {
			finalResult = rootResult
		}

		respondJSON(w, http.StatusOK, SyncResponse{
			JobID:      firstJobID,
			Status:     finalStatus,
			DurationMs: time.Since(startTime).Milliseconds(),
			Result:     finalResult,
			Tree:       tree,
		})
		return
	}

	respondJSON(w, http.StatusAccepted, TriggerResponse{
		JobID:   firstJobID,
		Status:  "queued",
		Plugin:  "pipeline",
		Command: pipelineName,
	})
}

// handlePluginTrigger handles POST /plugin/{plugin}/{command}
// Bypasses router and executes the plugin directly.
func (s *Server) handlePluginTrigger(w http.ResponseWriter, r *http.Request) {
	pluginName := chi.URLParam(r, "plugin")
	commandName := chi.URLParam(r, "command")

	plug, ok := s.registry.Get(pluginName)
	if !ok {
		s.writeError(w, http.StatusBadRequest, "plugin not found")
		return
	}

	if !plug.SupportsCommand(commandName) {
		s.writeError(w, http.StatusBadRequest, "command not supported by plugin")
		return
	}

	principal, _ := auth.PrincipalFromContext(r.Context())
	cmdType, _ := plug.CommandTypeFor(commandName)
	if cmdType != plugin.CommandTypeRead {
		if !auth.HasAnyScope(principal, "plugin:rw", "*") {
			s.writeError(w, http.StatusForbidden, "insufficient scope")
			return
		}
	}

	var req TriggerRequest
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
	}

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
		"direct":       true,
	})

	s.logger.Info("plugin job enqueued directly via API", "job_id", jobID, "plugin", pluginName, "command", commandName)

	respondJSON(w, http.StatusAccepted, TriggerResponse{
		JobID:   jobID,
		Status:  "queued",
		Plugin:  pluginName,
		Command: commandName,
	})
}

// handleTrigger handles POST /trigger/{plugin}/{command}
// DEPRECATED: Use /plugin/{plugin}/{command} for direct execution or /pipeline/{name} for pipelines.
func (s *Server) handleTrigger(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Ductile-Deprecation", "The /trigger endpoint is ambiguous and will be removed in a future version. Use /plugin or /pipeline instead.")
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

	triggerEvent := fmt.Sprintf("%s.%s", pluginName, commandName)
	pipeline := s.router.GetPipelineByTrigger(triggerEvent)

	var eventContextID *string
	if pipeline != nil && s.contextStore != nil {
		root, err := s.contextStore.Create(r.Context(), nil, pipeline.Name, pipeline.EntryStepID, json.RawMessage(`{}`))
		if err != nil {
			s.logger.Error(
				"failed to create event context for trigger",
				"pipeline", pipeline.Name,
				"step_id", pipeline.EntryStepID,
				"error", err,
			)
			s.writeError(w, http.StatusInternalServerError, "failed to create event context")
			return
		}
		eventContextID = &root.ID
	}

	// Enqueue job
	startTime := time.Now()
	jobID, err := s.queue.Enqueue(r.Context(), queue.EnqueueRequest{
		Plugin:         pluginName,
		Command:        commandName,
		Payload:        enqueuePayload,
		SubmittedBy:    "api",
		EventContextID: eventContextID,
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

		// Find terminal step result (if pipeline has terminal steps)
		var terminalResult json.RawMessage
		if pipeline != nil && len(pipeline.TerminalStepIDs) > 0 {
			// Look for a job matching one of the terminal step IDs
			for _, res := range results {
				for _, termStepID := range pipeline.TerminalStepIDs {
					if res.StepID == termStepID {
						terminalResult = res.Result
						break
					}
				}
				if terminalResult != nil {
					break
				}
			}
		}

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

		// Use terminal step result if found, otherwise fallback to root job result
		finalResult := terminalResult
		if finalResult == nil {
			finalResult = rootResult
		}

		respondJSON(w, http.StatusOK, SyncResponse{
			JobID:      jobID,
			Status:     finalStatus,
			DurationMs: duration.Milliseconds(),
			Result:     finalResult,
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
