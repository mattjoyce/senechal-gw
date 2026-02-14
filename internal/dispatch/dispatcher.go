package dispatch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/mattjoyce/ductile/internal/config"
	"github.com/mattjoyce/ductile/internal/events"
	"github.com/mattjoyce/ductile/internal/log"
	"github.com/mattjoyce/ductile/internal/plugin"
	"github.com/mattjoyce/ductile/internal/protocol"
	"github.com/mattjoyce/ductile/internal/queue"
	"github.com/mattjoyce/ductile/internal/router"
	"github.com/mattjoyce/ductile/internal/state"
	"github.com/mattjoyce/ductile/internal/workspace"
)

const (
	// maxStderrBytes caps the amount of stderr captured from plugin execution.
	maxStderrBytes = 64 * 1024

	// terminationGracePeriod is the time we wait after SIGTERM before sending SIGKILL.
	terminationGracePeriod = 5 * time.Second
)

// Dispatcher dequeues jobs and executes them by spawning plugin subprocesses.
type Dispatcher struct {
	queue     *queue.Queue
	state     *state.Store
	contexts  *state.ContextStore
	workspace workspace.Manager
	router    router.Engine
	registry  *plugin.Registry
	cfg       *config.Config
	events    *events.Hub
	logger    *slog.Logger

	// completions tracks jobs being waited on for synchronous execution.
	// Map key is root job ID. Value is a channel that is closed when the tree is complete.
	completions map[string]chan struct{}
	mu          sync.RWMutex
}

// New creates a new Dispatcher.
func New(
	q *queue.Queue,
	st *state.Store,
	contexts *state.ContextStore,
	ws workspace.Manager,
	rt router.Engine,
	reg *plugin.Registry,
	hub *events.Hub,
	cfg *config.Config,
) *Dispatcher {
	return &Dispatcher{
		queue:       q,
		state:       st,
		contexts:    contexts,
		workspace:   ws,
		router:      rt,
		registry:    reg,
		events:      hub,
		cfg:         cfg,
		logger:      log.WithComponent("dispatch"),
		completions: make(map[string]chan struct{}),
	}
}

// Start runs the main dispatch loop. It dequeues jobs serially and executes them one at a time.
// This is a blocking call that runs until ctx is cancelled.
func (d *Dispatcher) Start(ctx context.Context) error {
	d.logger.Info("dispatch loop started")
	defer d.logger.Info("dispatch loop stopped")

	ticker := time.NewTicker(1 * time.Second) // Poll queue every second
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			// Attempt to dequeue and execute one job
			if err := d.processNextJob(ctx); err != nil {
				d.logger.Error("failed to process job", "error", err)
				// Continue processing - don't crash the loop on individual job errors
			}
		}
	}
}

// ExecuteJob runs a single job by spawning the plugin subprocess.
func (d *Dispatcher) ExecuteJob(ctx context.Context, job *queue.Job) {
	d.executeJob(ctx, job)
}

// processNextJob dequeues the next job and executes it.
func (d *Dispatcher) processNextJob(ctx context.Context) error {
	job, err := d.queue.Dequeue(ctx)
	if err != nil {
		return fmt.Errorf("dequeue: %w", err)
	}
	if job == nil {
		// Queue is empty, nothing to do
		return nil
	}

	// Execute the job
	d.executeJob(ctx, job)
	return nil
}

// executeJob runs a single job by spawning the plugin subprocess.
func (d *Dispatcher) executeJob(ctx context.Context, job *queue.Job) {
	jobLogger := d.logger.With("job_id", job.ID, "plugin", job.Plugin, "command", job.Command)
	jobLogger.Info("job started", "attempt", job.Attempt)
	d.events.Publish("job.started", map[string]any{
		"job_id":        job.ID,
		"plugin":        job.Plugin,
		"command":       job.Command,
		"attempt":       job.Attempt,
		"parent_job_id": deref(job.ParentJobID),
	})

	// Get plugin from registry
	plug, ok := d.registry.Get(job.Plugin)
	if !ok {
		errMsg := fmt.Sprintf("plugin %q not found in registry", job.Plugin)
		jobLogger.Error(errMsg)
		d.completeJob(ctx, jobLogger, job.ID, job.StartedAt, queue.StatusFailed, nil, &errMsg, nil)
		return
	}

	// Check if plugin supports this command
	if !plug.SupportsCommand(job.Command) {
		errMsg := fmt.Sprintf("plugin %q does not support command %q", job.Plugin, job.Command)
		jobLogger.Error(errMsg)
		d.completeJob(ctx, jobLogger, job.ID, job.StartedAt, queue.StatusFailed, nil, &errMsg, nil)
		return
	}

	// Get plugin configuration
	pluginCfg := d.cfg.Plugins[job.Plugin]
	if pluginCfg.Config == nil {
		pluginCfg.Config = make(map[string]interface{})
	}

	// Get plugin state
	pluginState, err := d.state.Get(ctx, job.Plugin)
	if err != nil {
		errMsg := fmt.Sprintf("failed to get plugin state: %v", err)
		jobLogger.Error(errMsg)
		d.completeJob(ctx, jobLogger, job.ID, job.StartedAt, queue.StatusFailed, nil, &errMsg, nil)
		return
	}

	// Unmarshal state into map
	var stateMap map[string]interface{}
	if err := json.Unmarshal(pluginState, &stateMap); err != nil {
		errMsg := fmt.Sprintf("failed to unmarshal plugin state: %v", err)
		jobLogger.Error(errMsg)
		d.completeJob(ctx, jobLogger, job.ID, job.StartedAt, queue.StatusFailed, nil, &errMsg, nil)
		return
	}

	// Determine timeout for this command
	timeout := d.getTimeout(pluginCfg.Timeouts, job.Command)
	deadline := time.Now().Add(timeout)

	workspaceDir, err := d.ensureWorkspaceForJob(ctx, job)
	if err != nil {
		errMsg := fmt.Sprintf("failed to prepare workspace: %v", err)
		jobLogger.Error(errMsg)
		d.completeJob(ctx, jobLogger, job.ID, job.StartedAt, queue.StatusFailed, nil, &errMsg, nil)
		return
	}

	requestContext, err := d.loadRequestContext(ctx, job)
	if err != nil {
		errMsg := fmt.Sprintf("failed to load event context: %v", err)
		jobLogger.Error(errMsg)
		d.completeJob(ctx, jobLogger, job.ID, job.StartedAt, queue.StatusFailed, nil, &errMsg, nil)
		return
	}

	// Build protocol request
	req := &protocol.Request{
		Protocol:     2,
		JobID:        job.ID,
		Command:      job.Command,
		Config:       pluginCfg.Config,
		State:        stateMap,
		Context:      requestContext,
		WorkspaceDir: workspaceDir,
		DeadlineAt:   deadline,
	}

	// For handle command, parse and include event payload
	if job.Command == "handle" && len(job.Payload) > 0 {
		var event protocol.Event
		if err := json.Unmarshal(job.Payload, &event); err != nil {
			errMsg := fmt.Sprintf("failed to unmarshal event payload: %v", err)
			jobLogger.Error(errMsg)
			d.completeJob(ctx, jobLogger, job.ID, job.StartedAt, queue.StatusFailed, nil, &errMsg, nil)
			return
		}

		// GOVERNANCE HYBRID MERGE:
		// Automatically merge accumulated context into the event payload
		// so plugins only have to look in one place. Immediate event data wins.
		if event.Payload == nil {
			event.Payload = make(map[string]any)
		}
		for k, v := range requestContext {
			if _, exists := event.Payload[k]; !exists {
				event.Payload[k] = v
			}
		}

		req.Event = &event
	}

	// Spawn plugin and execute
	resp, rawResp, stderr, err := d.spawnPlugin(ctx, job.Plugin, plug.Entrypoint, req, timeout, jobLogger)

	// Handle timeout (check if error is context.DeadlineExceeded)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			errMsg := fmt.Sprintf("plugin execution timed out after %v", timeout)
			jobLogger.Warn(errMsg)
			d.completeJob(ctx, jobLogger, job.ID, job.StartedAt, queue.StatusTimedOut, nil, &errMsg, &stderr)
			return
		}

		// Handle other spawn errors
		errMsg := fmt.Sprintf("plugin spawn failed: %v", err)
		jobLogger.Error(errMsg)
		d.completeJob(ctx, jobLogger, job.ID, job.StartedAt, queue.StatusFailed, rawResp, &errMsg, &stderr)
		return
	}

	// Handle protocol errors
	if resp == nil {
		errMsg := "plugin returned nil response"
		jobLogger.Error(errMsg)
		d.completeJob(ctx, jobLogger, job.ID, job.StartedAt, queue.StatusFailed, rawResp, &errMsg, &stderr)
		return
	}

	// Log plugin logs
	for _, entry := range resp.Logs {
		jobLogger.Info("plugin log", "level", entry.Level, "message", entry.Message)
	}

	// Handle response status
	if resp.Status == "error" {
		jobLogger.Warn("plugin returned error", "error", resp.Error)
		errMsg := resp.Error
		d.completeJob(ctx, jobLogger, job.ID, job.StartedAt, queue.StatusFailed, rawResp, &errMsg, &stderr)
		// TODO: Handle retry logic based on resp.ShouldRetry() - not in MVP
		return
	}

	// Apply state updates
	if len(resp.StateUpdates) > 0 {
		updatesJSON, err := json.Marshal(resp.StateUpdates)
		if err != nil {
			errMsg := fmt.Sprintf("failed to marshal state updates: %v", err)
			jobLogger.Error(errMsg)
			d.completeJob(ctx, jobLogger, job.ID, job.StartedAt, queue.StatusFailed, rawResp, &errMsg, &stderr)
			return
		}

		if _, err := d.state.ShallowMerge(ctx, job.Plugin, updatesJSON); err != nil {
			errMsg := fmt.Sprintf("failed to apply state updates: %v", err)
			jobLogger.Error(errMsg)
			d.completeJob(ctx, jobLogger, job.ID, job.StartedAt, queue.StatusFailed, rawResp, &errMsg, &stderr)
			return
		}
		jobLogger.Debug("applied state updates", "updates", resp.StateUpdates)
	}

	// Process events (routing + orchestration)
	if len(resp.Events) > 0 {
		if err := d.routeEvents(ctx, job, resp.Events, jobLogger); err != nil {
			errMsg := fmt.Sprintf("failed to route events: %v", err)
			jobLogger.Error(errMsg)
			d.completeJob(ctx, jobLogger, job.ID, job.StartedAt, queue.StatusFailed, rawResp, &errMsg, &stderr)
			return
		}
	}

	// Mark job as succeeded
	d.completeJob(ctx, jobLogger, job.ID, job.StartedAt, queue.StatusSucceeded, rawResp, nil, &stderr)
}

// spawnPlugin spawns the plugin subprocess, writes the request to stdin, and reads the response from stdout.
// Returns the response, stderr output, and any error.
func (d *Dispatcher) spawnPlugin(
	ctx context.Context,
	pluginName string,
	entrypoint string,
	req *protocol.Request,
	timeout time.Duration,
	logger *slog.Logger,
) (*protocol.Response, json.RawMessage, string, error) {
	// Create timer for timeout enforcement
	timeoutTimer := time.NewTimer(timeout)
	defer timeoutTimer.Stop()

	// Prepare command (don't use CommandContext - we'll manage termination ourselves)
	cmd := exec.Command(entrypoint)

	// Prepare stdin pipe
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, nil, "", fmt.Errorf("create stdin pipe: %w", err)
	}

	// Capture stdout and stderr
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	logger.Info("plugin executing", "entrypoint", entrypoint, "timeout", timeout)

	// Start the process
	if err := cmd.Start(); err != nil {
		return nil, nil, "", fmt.Errorf("start process: %w", err)
	}

	d.events.Publish("plugin.spawned", map[string]any{
		"job_id":  req.JobID,
		"plugin":  pluginName,
		"command": req.Command,
		"pid":     cmd.Process.Pid,
	})

	// Write request to stdin in a goroutine
	writeErr := make(chan error, 1)
	go func() {
		defer stdin.Close()
		if err := protocol.EncodeRequest(stdin, req); err != nil {
			writeErr <- fmt.Errorf("encode request: %w", err)
			return
		}
		writeErr <- nil
	}()

	// Wait for process to complete or timeout
	waitErr := make(chan error, 1)
	go func() {
		waitErr <- cmd.Wait()
	}()

	// Wait for completion or timeout
	select {
	case <-timeoutTimer.C:
		// Timeout occurred - enforce termination
		logger.Warn("plugin execution timed out, sending SIGTERM")
		if cmd.Process != nil {
			if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
				logger.Error("failed to send SIGTERM", "error", err)
			}
		}

		// Wait for grace period
		grace := time.NewTimer(terminationGracePeriod)
		defer grace.Stop()

		select {
		case <-waitErr:
			// Process exited gracefully
			logger.Info("plugin exited after SIGTERM")
		case <-grace.C:
			// Grace period expired, send SIGKILL
			logger.Warn("plugin did not exit after SIGTERM, sending SIGKILL")
			if cmd.Process != nil {
				if err := cmd.Process.Kill(); err != nil {
					logger.Error("failed to send SIGKILL", "error", err)
				}
			}
			<-waitErr // Wait for process to die
		}

		stderrStr := truncateStderr(stderr.String())
		return nil, nil, stderrStr, context.DeadlineExceeded

	case err := <-waitErr:
		// Process completed
		if werr := <-writeErr; werr != nil {
			stderrStr := truncateStderr(stderr.String())
			return nil, nil, stderrStr, werr
		}

		stderrStr := truncateStderr(stderr.String())

		// Check exit code
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				logger.Warn("plugin exited with non-zero status", "exit_code", exitErr.ExitCode())
			} else {
				return nil, nil, stderrStr, fmt.Errorf("wait for process: %w", err)
			}
		}

		// Decode response from stdout
		resp, rawBytes, err := protocol.DecodeResponseLenient(bytes.NewReader(stdout.Bytes()))
		if err != nil {
			logger.Error("failed to decode plugin response", "error", err, "stdout", string(rawBytes))
			return nil, json.RawMessage(rawBytes), stderrStr, fmt.Errorf("decode response: %w", err)
		}

		return resp, json.RawMessage(rawBytes), stderrStr, nil
	}
}

func (d *Dispatcher) ensureWorkspaceForJob(ctx context.Context, job *queue.Job) (string, error) {
	if d.workspace == nil {
		return "", nil
	}

	if ws, err := d.workspace.Open(ctx, job.ID); err == nil {
		return ws.Dir, nil
	}

	if job.ParentJobID != nil {
		if ws, err := d.workspace.Clone(ctx, *job.ParentJobID, job.ID); err == nil {
			return ws.Dir, nil
		}
	}

	ws, err := d.workspace.Create(ctx, job.ID)
	if err != nil {
		return "", err
	}
	return ws.Dir, nil
}

func (d *Dispatcher) loadRequestContext(ctx context.Context, job *queue.Job) (map[string]any, error) {
	if d.contexts == nil || job.EventContextID == nil {
		return nil, nil
	}

	eventCtx, err := d.contexts.Get(ctx, *job.EventContextID)
	if err != nil {
		return nil, err
	}
	out := make(map[string]any)
	if len(eventCtx.AccumulatedJSON) == 0 {
		return out, nil
	}
	if err := json.Unmarshal(eventCtx.AccumulatedJSON, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (d *Dispatcher) routeEvents(ctx context.Context, job *queue.Job, events []protocol.Event, logger *slog.Logger) error {
	if d.router == nil {
		logger.Info("plugin emitted events (router not configured)", "count", len(events))
		return nil
	}

	var sourcePipeline, sourceStepID string
	if d.contexts != nil && job.EventContextID != nil {
		currentCtx, err := d.contexts.Get(ctx, *job.EventContextID)
		if err != nil {
			return fmt.Errorf("load current context %q: %w", *job.EventContextID, err)
		}
		sourcePipeline = currentCtx.PipelineName
		sourceStepID = currentCtx.StepID
	}

	for i := range events {
		if events[i].EventID == "" {
			events[i].EventID = uuid.NewString()
		}
		if events[i].Timestamp.IsZero() {
			events[i].Timestamp = time.Now().UTC()
		}
		if events[i].Source == "" {
			events[i].Source = job.Plugin
		}
		ev := events[i]

		req := router.Request{
			SourcePlugin:    job.Plugin,
			SourceJobID:     job.ID,
			SourceContextID: deref(job.EventContextID),
			SourcePipeline:  sourcePipeline,
			SourceStepID:    sourceStepID,
			SourceEventID:   ev.EventID,
			Event:           ev,
		}

		nextDispatches, err := d.router.Next(ctx, req)
		if err != nil {
			return fmt.Errorf("resolve routes for event %q: %w", ev.Type, err)
		}
		if len(nextDispatches) == 0 {
			logger.Debug("no route matched for event", "event_type", ev.Type, "source_plugin", job.Plugin)
			continue
		}

		for _, next := range nextDispatches {
			payload, err := json.Marshal(next.Event)
			if err != nil {
				return fmt.Errorf("marshal routed event payload: %w", err)
			}

			var contextID *string
			if d.contexts != nil {
				updates, err := payloadObjectFromEvent(next.Event)
				if err != nil {
					return err
				}
				parentCtxID := strPtrOrNil(next.ParentContextID)
				createdCtx, err := d.contexts.Create(ctx, parentCtxID, next.PipelineName, next.StepID, updates)
				if err != nil {
					return fmt.Errorf("create routed context (%s:%s): %w", next.PipelineName, next.StepID, err)
				}
				contextID = &createdCtx.ID
			}

			sourceEventID := next.SourceEventID
			enqueueReq := queue.EnqueueRequest{
				Plugin:         next.Plugin,
				Command:        next.Command,
				Payload:        payload,
				SubmittedBy:    "route",
				ParentJobID:    strPtrOrNil(next.ParentJobID),
				EventContextID: contextID,
				SourceEventID:  strPtrOrNil(sourceEventID),
			}
			childJobID, err := d.queue.Enqueue(ctx, enqueueReq)
			if err != nil {
				return fmt.Errorf("enqueue routed job for plugin %q: %w", next.Plugin, err)
			}

			d.events.Publish("router.enqueued", map[string]any{
				"job_id":           childJobID,
				"parent_job_id":    job.ID,
				"plugin":           next.Plugin,
				"command":          next.Command,
				"pipeline":         next.PipelineName,
				"step_id":          next.StepID,
				"event_context_id": deref(contextID),
			})

			if d.workspace != nil {
				if _, err := d.workspace.Clone(ctx, job.ID, childJobID); err != nil {
					return fmt.Errorf("clone workspace %q -> %q: %w", job.ID, childJobID, err)
				}
			}

			logger.Info(
				"enqueued routed job",
				"event_type", ev.Type,
				"to_plugin", next.Plugin,
				"to_command", next.Command,
				"child_job_id", childJobID,
				"pipeline", next.PipelineName,
				"step_id", next.StepID,
				"event_context_id", deref(contextID),
			)
		}
	}

	return nil
}

func payloadObjectFromEvent(event protocol.Event) (json.RawMessage, error) {
	if event.Payload == nil {
		return json.RawMessage(`{}`), nil
	}
	raw, err := json.Marshal(event.Payload)
	if err != nil {
		return nil, fmt.Errorf("marshal event payload: %w", err)
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, fmt.Errorf("event payload must be a JSON object: %w", err)
	}
	return raw, nil
}

func strPtrOrNil(v string) *string {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	out := v
	return &out
}

func deref(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

// getTimeout returns the timeout for a given command, falling back to defaults.
func (d *Dispatcher) getTimeout(timeouts *config.TimeoutsConfig, command string) time.Duration {
	if timeouts == nil {
		timeouts = config.DefaultPluginConf().Timeouts
	}

	switch command {
	case "poll":
		if timeouts.Poll > 0 {
			return timeouts.Poll
		}
		return 60 * time.Second
	case "handle":
		if timeouts.Handle > 0 {
			return timeouts.Handle
		}
		return 120 * time.Second
	case "health":
		if timeouts.Health > 0 {
			return timeouts.Health
		}
		return 10 * time.Second
	case "init":
		if timeouts.Init > 0 {
			return timeouts.Init
		}
		return 30 * time.Second
	default:
		return 60 * time.Second
	}
}

// completeJob marks a job as complete with the given status and logs the outcome.
func (d *Dispatcher) completeJob(ctx context.Context, logger *slog.Logger, jobID string, startTime *time.Time, status queue.Status, result json.RawMessage, lastError, stderr *string) {
	duration := time.Duration(0)
	if startTime != nil {
		duration = time.Since(*startTime)
	}
	logger.Info("job completed", "status", string(status), "duration", duration.String())

	eventData := map[string]any{
		"job_id":      jobID,
		"status":      string(status),
		"duration_ms": duration.Milliseconds(),
	}
	if lastError != nil {
		eventData["error"] = *lastError
	}

	switch status {
	case queue.StatusSucceeded:
		d.events.Publish("job.completed", eventData)
	case queue.StatusTimedOut:
		d.events.Publish("job.timed_out", eventData)
	case queue.StatusFailed, queue.StatusDead:
		d.events.Publish("job.failed", eventData)
	}

	if err := d.queue.CompleteWithResult(ctx, jobID, status, result, lastError, stderr); err != nil {
		d.logger.Error("failed to complete job", "job_id", jobID, "error", err)
	}

	// Notify any synchronous waiters
	d.notifyCompletion(jobID)
}

// WaitForJobTree blocks until the root job and all its descendants are complete or timeout.
func (d *Dispatcher) WaitForJobTree(ctx context.Context, rootJobID string, timeout time.Duration) ([]*queue.JobResult, error) {
	// Create completion channel
	ch := make(chan struct{}, 1)

	d.mu.Lock()
	d.completions[rootJobID] = ch
	d.mu.Unlock()

	defer func() {
		d.mu.Lock()
		delete(d.completions, rootJobID)
		d.mu.Unlock()
	}()

	// Initial check in case it's already done
	complete, err := d.checkJobTreeComplete(ctx, rootJobID)
	if err != nil {
		return nil, fmt.Errorf("initial tree check: %w", err)
	}
	if complete {
		return d.queue.GetJobTree(ctx, rootJobID)
	}

	// Set up timeout
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timer.C:
			return nil, fmt.Errorf("timeout waiting for job tree completion")
		case <-ch:
			// Something in the tree finished, check if the whole tree is done
			complete, err := d.checkJobTreeComplete(ctx, rootJobID)
			if err != nil {
				return nil, fmt.Errorf("tree completion check: %w", err)
			}
			if complete {
				return d.queue.GetJobTree(ctx, rootJobID)
			}
		}
	}
}

// notifyCompletion signals any waiters that a job has finished.
func (d *Dispatcher) notifyCompletion(jobID string) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	// Currently we notify all waiters. Optimization: only notify waiters interested in this tree.
	// To do that efficiently, we'd need to know the root of every jobID.
	// For MVP, notifying all sync waiters (usually few) is acceptable.
	for _, ch := range d.completions {
		select {
		case ch <- struct{}{}:
		default:
			// Channel already has a pending notification
		}
	}
}

// checkJobTreeComplete returns true if the root job and all its descendants are in terminal states.
func (d *Dispatcher) checkJobTreeComplete(ctx context.Context, rootJobID string) (bool, error) {
	// Query database for tree status
	// We include timed_out, dead, cancelled (if added later) as terminal.
	results, err := d.queue.GetJobTree(ctx, rootJobID)
	if err != nil {
		return false, err
	}

	if len(results) == 0 {
		return false, fmt.Errorf("job %s not found", rootJobID)
	}

	for _, res := range results {
		switch res.Status {
		case queue.StatusQueued, queue.StatusRunning:
			return false, nil
		}
	}

	return true, nil
}

// truncateStderr truncates stderr to maxStderrBytes.
func truncateStderr(s string) string {
	if len(s) > maxStderrBytes {
		return s[:maxStderrBytes]
	}
	return s
}
