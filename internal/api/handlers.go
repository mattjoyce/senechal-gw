package api

import (
	"encoding/json"
	"net/http"
	"slices"

	"github.com/go-chi/chi/v5"
	"github.com/mattjoyce/senechal-gw/internal/queue"
)

// handleTrigger handles POST /trigger/{plugin}/{command}
func (s *Server) handleTrigger(w http.ResponseWriter, r *http.Request) {
	pluginName := chi.URLParam(r, "plugin")
	commandName := chi.URLParam(r, "command")

	// Validate plugin exists
	plugin, ok := s.registry.Get(pluginName)
	if !ok {
		s.writeError(w, http.StatusBadRequest, "plugin not found")
		return
	}

	// Validate command is supported by plugin
	if !slices.Contains(plugin.Commands, commandName) {
		s.writeError(w, http.StatusBadRequest, "command not supported by plugin")
		return
	}

	// Parse request body (optional payload)
	var req TriggerRequest
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
	}

	// Enqueue job
	jobID, err := s.queue.Enqueue(r.Context(), queue.EnqueueRequest{
		Plugin:      pluginName,
		Command:     commandName,
		Payload:     req.Payload,
		SubmittedBy: "api",
	})
	if err != nil {
		s.logger.Error("failed to enqueue job", "plugin", pluginName, "command", commandName, "error", err)
		s.writeError(w, http.StatusInternalServerError, "failed to enqueue job")
		return
	}

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
