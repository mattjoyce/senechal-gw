package dispatch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"math/rand"
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
	"github.com/mattjoyce/ductile/internal/router/conditions"
	"github.com/mattjoyce/ductile/internal/state"
	"github.com/mattjoyce/ductile/internal/workspace"
)

const (
	// maxStderrBytes caps the amount of stderr captured from plugin execution.
	maxStderrBytes = 64 * 1024

	// terminationGracePeriod is the time we wait after SIGTERM before sending SIGKILL.
	terminationGracePeriod = 5 * time.Second

	nonRetryableExitCode = 78
)

// workerResult is sent by a worker goroutine when it finishes executing a job.
type workerResult struct {
	jobID     string
	plugin    string
	dedupeKey string // empty if job had no dedupe key
}

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

// Start runs the main dispatch loop with bounded concurrency. It tracks active
// workers globally and per-plugin, filling available slots each tick or when a
// worker completes. When cfg.Service.MaxWorkers == 1 (the default) this behaves
// identically to the original serial dispatcher.
//
// This is a blocking call that runs until ctx is cancelled.
func (d *Dispatcher) Start(ctx context.Context) error {
	maxWorkers := d.cfg.Service.MaxWorkers
	if maxWorkers <= 0 {
		maxWorkers = 1
	}

	d.logger.Info("dispatch loop started", "max_workers", maxWorkers)
	defer d.logger.Info("dispatch loop stopped")

	// Concurrency tracking — only accessed by the coordinator goroutine (no lock needed).
	activeTotal := 0
	activeByPlugin := make(map[string]int)
	activeKeys := make(map[string]struct{})

	workerDone := make(chan workerResult, maxWorkers)

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	// fillWorkers tries to dequeue eligible jobs until all worker slots are full
	// or no eligible work remains.
	fillWorkers := func() {
		for activeTotal < maxWorkers {
			// Build skip list: plugins at their parallelism cap
			var skipPlugins []string
			for pluginName, count := range activeByPlugin {
				limit := d.pluginParallelism(pluginName)
				if count >= limit {
					skipPlugins = append(skipPlugins, pluginName)
				}
			}

			// Build active concurrency keys list
			var activeKeySlice []string
			for k := range activeKeys {
				activeKeySlice = append(activeKeySlice, k)
			}

			job, err := d.queue.DequeueEligible(ctx, skipPlugins, activeKeySlice)
			if err != nil {
				d.logger.Error("failed to dequeue eligible job", "error", err)
				return
			}
			if job == nil {
				return // No eligible work
			}

			// Track this worker
			activeTotal++
			activeByPlugin[job.Plugin]++
			dedupeKey := ""
			if job.DedupeKey != nil {
				dedupeKey = *job.DedupeKey
				activeKeys[dedupeKey] = struct{}{}
			}

			d.logger.Debug("dispatching job",
				"job_id", job.ID,
				"plugin", job.Plugin,
				"active_total", activeTotal,
				"active_plugin", activeByPlugin[job.Plugin],
			)

			// Launch worker goroutine
			go func(j *queue.Job, dk string) {
				d.executeJob(ctx, j)
				workerDone <- workerResult{
					jobID:     j.ID,
					plugin:    j.Plugin,
					dedupeKey: dk,
				}
			}(job, dedupeKey)
		}
	}

	for {
		select {
		case <-ctx.Done():
			// Drain active workers before returning
			d.logger.Info("dispatch loop shutting down, draining workers", "active", activeTotal)
			for activeTotal > 0 {
				res := <-workerDone
				activeTotal--
				activeByPlugin[res.plugin]--
				if res.dedupeKey != "" {
					delete(activeKeys, res.dedupeKey)
				}
			}
			return ctx.Err()

		case res := <-workerDone:
			activeTotal--
			activeByPlugin[res.plugin]--
			if activeByPlugin[res.plugin] <= 0 {
				delete(activeByPlugin, res.plugin)
			}
			if res.dedupeKey != "" {
				delete(activeKeys, res.dedupeKey)
			}
			// A slot freed up — try to fill
			fillWorkers()

		case <-ticker.C:
			fillWorkers()
		}
	}
}

// pluginParallelism returns the effective parallelism limit for a plugin.
//
// If a plugin manifest declares concurrency_safe: false, it runs serial by
// default (limit=1). Operators can still explicitly override via config by
// setting plugins.<name>.parallelism > 1.
func (d *Dispatcher) pluginParallelism(pluginName string) int {
	limit := 1
	if pc, ok := d.cfg.Plugins[pluginName]; ok && pc.Parallelism > 0 {
		limit = pc.Parallelism
	}

	if plug, ok := d.registry.Get(pluginName); ok && !plug.ConcurrencySafe {
		if limit <= 1 {
			return 1
		}
		// Explicit operator override via config (>1) is honored.
	}

	return limit
}

// ExecuteJob runs a single job by spawning the plugin subprocess.
func (d *Dispatcher) ExecuteJob(ctx context.Context, job *queue.Job) {
	d.executeJob(ctx, job)
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

	preflight, err := d.preflightJob(ctx, job)
	if err != nil {
		errMsg := fmt.Sprintf("job preflight failed: %v", err)
		jobLogger.Error(errMsg)
		d.events.Publish("job.preflight", map[string]any{
			"job_id":   job.ID,
			"plugin":   job.Plugin,
			"command":  job.Command,
			"decision": "fail",
			"reason":   errMsg,
		})
		d.completeJob(ctx, jobLogger, job.ID, job.Plugin, job.StartedAt, queue.StatusFailed, nil, &errMsg, nil)
		return
	}
	d.events.Publish("job.preflight", map[string]any{
		"job_id":        job.ID,
		"plugin":        job.Plugin,
		"command":       job.Command,
		"decision":      string(preflight.decision),
		"reason":        preflight.reason,
		"workspace_dir": preflight.workspaceDir,
	})
	if preflight.decision == preflightDecisionSkip {
		d.skipJob(ctx, jobLogger, job, preflight.requestContext, preflight.reason)
		return
	}

	requestContext := preflight.requestContext
	workspaceDir := preflight.workspaceDir

	// Get plugin from registry
	plug, ok := d.registry.Get(job.Plugin)
	if !ok {
		errMsg := fmt.Sprintf("plugin %q not found in registry", job.Plugin)
		jobLogger.Error(errMsg)
		d.completeJob(ctx, jobLogger, job.ID, job.Plugin, job.StartedAt, queue.StatusFailed, nil, &errMsg, nil)
		return
	}

	// Check if plugin supports this command
	if !plug.SupportsCommand(job.Command) {
		errMsg := fmt.Sprintf("plugin %q does not support command %q", job.Plugin, job.Command)
		jobLogger.Error(errMsg)
		d.completeJob(ctx, jobLogger, job.ID, job.Plugin, job.StartedAt, queue.StatusFailed, nil, &errMsg, nil)
		return
	}

	// Get plugin configuration
	pluginCfg := d.cfg.Plugins[job.Plugin]
	if pluginCfg.Config == nil {
		pluginCfg.Config = make(map[string]any)
	}

	// Get plugin state
	pluginState, err := d.state.Get(ctx, job.Plugin)
	if err != nil {
		errMsg := fmt.Sprintf("failed to get plugin state: %v", err)
		jobLogger.Error(errMsg)
		d.completeJob(ctx, jobLogger, job.ID, job.Plugin, job.StartedAt, queue.StatusFailed, nil, &errMsg, nil)
		return
	}

	// Unmarshal state into map
	var stateMap map[string]any
	if err := json.Unmarshal(pluginState, &stateMap); err != nil {
		errMsg := fmt.Sprintf("failed to unmarshal plugin state: %v", err)
		jobLogger.Error(errMsg)
		d.completeJob(ctx, jobLogger, job.ID, job.Plugin, job.StartedAt, queue.StatusFailed, nil, &errMsg, nil)
		return
	}

	// Determine timeout for this command
	timeout := d.getTimeout(pluginCfg.Timeouts, job.Command)
	deadline := time.Now().Add(timeout)

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

	// Include payload if present
	if len(job.Payload) > 0 {
		if err := json.Unmarshal(job.Payload, &req.Payload); err != nil {
			// If it's a 'handle' command, it might be a protocol.Event wrapper
			// We try to unmarshal into Payload first, if it's just a JSON object.
			jobLogger.Debug("payload is not a simple JSON object, attempting event unmarshal for handle")
		}
	}

	// For handle command, parse and include event payload
	if job.Command == "handle" && len(job.Payload) > 0 {
		var event protocol.Event
		if err := json.Unmarshal(job.Payload, &event); err != nil {
			errMsg := fmt.Sprintf("failed to unmarshal event payload: %v", err)
			jobLogger.Error(errMsg)
			d.completeJob(ctx, jobLogger, job.ID, job.Plugin, job.StartedAt, queue.StatusFailed, nil, &errMsg, nil)
			return
		}

		// GOVERNANCE HYBRID MERGE:
		// Automatically merge accumulated context into the event payload
		// so plugins only have to look in one place. Immediate event data wins.
		if event.Payload == nil {
			if event.Type == "" {
				// Raw payload (e.g. webhook body) — not a protocol.Event wrapper.
				// Promote the raw JSON into event.Payload so switch/pipeline plugins
				// can address fields via payload.* without special-casing.
				var raw map[string]any
				if err := json.Unmarshal(job.Payload, &raw); err == nil {
					event.Payload = raw
				}
			}
			if event.Payload == nil {
				event.Payload = make(map[string]any)
			}
		}
		for k, v := range requestContext {
			if _, exists := event.Payload[k]; !exists {
				event.Payload[k] = v
			}
		}

		req.Event = &event
	}

	// Spawn plugin and execute
	resp, rawResp, stdoutBytes, stderr, exitCode, err := d.spawnPlugin(ctx, job.Plugin, plug.Entrypoint, req, timeout, jobLogger)

	// Handle timeout (check if error is context.DeadlineExceeded)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			errMsg := fmt.Sprintf("plugin execution timed out after %v", timeout)
			jobLogger.Warn(errMsg)
			d.failOrRetry(ctx, jobLogger, job, pluginCfg, queue.StatusTimedOut, nil, &errMsg, &stderr, "timeout", true)
			return
		}

		// Handle other spawn errors
		errMsg := fmt.Sprintf("plugin spawn failed: %v", err)
		jobLogger.Error(errMsg)
		retryable := exitCode != nonRetryableExitCode
		reason := "spawn_error"
		if exitCode == nonRetryableExitCode {
			reason = "exit_code_78"
		}
		d.failOrRetry(ctx, jobLogger, job, pluginCfg, queue.StatusFailed, rawResp, &errMsg, &stderr, reason, retryable)
		return
	}

	// Handle protocol errors
	if resp == nil {
		errMsg := "plugin returned nil response"
		jobLogger.Error(errMsg)
		d.failOrRetry(ctx, jobLogger, job, pluginCfg, queue.StatusFailed, rawResp, &errMsg, &stderr, "nil_response", true)
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
		retryable := resp.ShouldRetry() && exitCode != nonRetryableExitCode
		reason := "plugin_error"
		if !resp.ShouldRetry() {
			reason = "plugin_retry_false"
		}
		if exitCode == nonRetryableExitCode {
			reason = "exit_code_78"
		}
		d.failOrRetry(ctx, jobLogger, job, pluginCfg, queue.StatusFailed, rawResp, &errMsg, &stderr, reason, retryable)
		return
	}

	// recordBlackBox writes terminal job data into the workspace .ductile/ bundle.
	// Errors are non-fatal — log and continue.
	recordBlackBox := func(status queue.Status, lastError *string) {
		meta := BlackBoxMetadata{
			JobID:       job.ID,
			Plugin:      job.Plugin,
			Command:     job.Command,
			Status:      string(status),
			Attempt:     job.Attempt,
			CreatedAt:   job.CreatedAt,
			StartedAt:   job.StartedAt,
			CompletedAt: time.Now().UTC(),
			LastError:   lastError,
			Context:     requestContext,
		}
		if err := writeBlackBox(workspaceDir, stdoutBytes, stderr, meta); err != nil {
			jobLogger.Warn("black box write failed (non-fatal)", "error", err)
		}
	}

	// Apply state updates
	if len(resp.StateUpdates) > 0 {
		updatesJSON, err := json.Marshal(resp.StateUpdates)
		if err != nil {
			errMsg := fmt.Sprintf("failed to marshal state updates: %v", err)
			jobLogger.Error(errMsg)
			recordBlackBox(queue.StatusFailed, &errMsg)
			d.completeJob(ctx, jobLogger, job.ID, job.Plugin, job.StartedAt, queue.StatusFailed, rawResp, &errMsg, &stderr)
			return
		}

		if _, err := d.state.ShallowMerge(ctx, job.Plugin, updatesJSON); err != nil {
			errMsg := fmt.Sprintf("failed to apply state updates: %v", err)
			jobLogger.Error(errMsg)
			recordBlackBox(queue.StatusFailed, &errMsg)
			d.completeJob(ctx, jobLogger, job.ID, job.Plugin, job.StartedAt, queue.StatusFailed, rawResp, &errMsg, &stderr)
			return
		}
		jobLogger.Debug("applied state updates", "updates", resp.StateUpdates)
	}

	// Process events (routing + orchestration)
	if len(resp.Events) > 0 {
		if strings.TrimSpace(resp.Result) != "" {
			for i := range resp.Events {
				if resp.Events[i].Payload == nil {
					resp.Events[i].Payload = make(map[string]any)
				}
				if _, exists := resp.Events[i].Payload["result"]; !exists {
					resp.Events[i].Payload["result"] = resp.Result
				}
			}
		}
		if err := d.routeEvents(ctx, job, resp.Events, jobLogger); err != nil {
			errMsg := fmt.Sprintf("failed to route events: %v", err)
			jobLogger.Error(errMsg)
			recordBlackBox(queue.StatusFailed, &errMsg)
			d.completeJob(ctx, jobLogger, job.ID, job.Plugin, job.StartedAt, queue.StatusFailed, rawResp, &errMsg, &stderr)
			return
		}
	} else if job.EventContextID != nil {
		// Plugin emitted no events but is running within a pipeline step — emit a
		// synthetic step-succeeded event so sequential successors can still advance.
		// Symmetric with routeSkippedStepSuccessors for skipped steps.
		synthetic := []protocol.Event{{
			Type:    "ductile.step.succeeded",
			Payload: map[string]any{"result": resp.Result},
		}}
		if err := d.routeEvents(ctx, job, synthetic, jobLogger); err != nil {
			errMsg := fmt.Sprintf("failed to route step-succeeded event: %v", err)
			jobLogger.Error(errMsg)
			recordBlackBox(queue.StatusFailed, &errMsg)
			d.completeJob(ctx, jobLogger, job.ID, job.Plugin, job.StartedAt, queue.StatusFailed, rawResp, &errMsg, &stderr)
			return
		}
	}

	// Mark job as succeeded
	recordBlackBox(queue.StatusSucceeded, nil)
	d.maybeFireHooks(ctx, job, hookSignalForStatus(queue.StatusSucceeded), map[string]any{
		"job_id":  job.ID,
		"plugin":  job.Plugin,
		"command": job.Command,
		"status":  "succeeded",
		"result":  resp.Result,
	})
	d.completeJob(ctx, jobLogger, job.ID, job.Plugin, job.StartedAt, queue.StatusSucceeded, rawResp, nil, &stderr)
}

// spawnPlugin spawns the plugin subprocess, writes the request to stdin, and reads the response from stdout.
// Returns the parsed response, raw response bytes, raw stdout bytes, stderr output, exit code, and any error.
func (d *Dispatcher) spawnPlugin(
	ctx context.Context,
	pluginName string,
	entrypoint string,
	req *protocol.Request,
	timeout time.Duration,
	logger *slog.Logger,
) (*protocol.Response, json.RawMessage, []byte, string, int, error) {
	// Create timer for timeout enforcement
	timeoutTimer := time.NewTimer(timeout)
	defer timeoutTimer.Stop()

	// Prepare command (don't use CommandContext - we'll manage termination ourselves)
	cmd := exec.Command(entrypoint)

	// Prepare stdin pipe
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, nil, nil, "", 0, fmt.Errorf("create stdin pipe: %w", err)
	}

	// Capture stdout and stderr
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	logger.Info("plugin executing", "entrypoint", entrypoint, "timeout", timeout)

	// Start the process
	if err := cmd.Start(); err != nil {
		return nil, nil, nil, "", 0, fmt.Errorf("start process: %w", err)
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
		if err := protocol.EncodeRequest(stdin, req); err != nil {
			if closeErr := stdin.Close(); closeErr != nil {
				writeErr <- fmt.Errorf("encode request: %w (close stdin: %v)", err, closeErr)
				return
			}
			writeErr <- fmt.Errorf("encode request: %w", err)
			return
		}
		if err := stdin.Close(); err != nil {
			writeErr <- fmt.Errorf("close stdin: %w", err)
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
		return nil, nil, stdout.Bytes(), stderrStr, 0, context.DeadlineExceeded

	case err := <-waitErr:
		// Process completed
		werr := <-writeErr

		stdoutBytes := stdout.Bytes()
		stderrStr := truncateStderr(stderr.String())
		exitCode := 0

		// Check exit code
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
				logger.Warn("plugin exited with non-zero status", "exit_code", exitCode)
			} else {
				return nil, nil, stdoutBytes, stderrStr, 0, fmt.Errorf("wait for process: %w", err)
			}
		}

		// Decode response from stdout
		resp, rawBytes, err := protocol.DecodeResponseLenient(bytes.NewReader(stdoutBytes))
		if err != nil {
			// If we also had a stdin write error, include it for diagnostics
			if werr != nil {
				logger.Warn("stdin write failed (process may not read stdin)", "error", werr)
			}
			logger.Error("failed to decode plugin response", "error", err, "stdout", string(rawBytes))
			return nil, json.RawMessage(rawBytes), stdoutBytes, stderrStr, exitCode, fmt.Errorf("decode response: %w", err)
		}

		// Log stdin write errors as warnings — some plugins don't read stdin
		// and may exit before the write completes, which is not a failure if
		// the process produced a valid response.
		if werr != nil {
			logger.Debug("stdin write error (ignored, valid response received)", "error", werr)
		}

		return resp, json.RawMessage(rawBytes), stdoutBytes, stderrStr, exitCode, nil
	}
}

func (d *Dispatcher) failOrRetry(
	ctx context.Context,
	logger *slog.Logger,
	job *queue.Job,
	pluginCfg config.PluginConf,
	status queue.Status,
	result json.RawMessage,
	lastError,
	stderr *string,
	reason string,
	retryable bool,
) {
	if retryable && job.Attempt < job.MaxAttempts {
		backoff := d.computeRetryDelay(pluginCfg.Retry, job.Attempt)
		nextRetryAt := time.Now().UTC().Add(backoff)
		nextAttempt := job.Attempt + 1

		if err := d.queue.UpdateJobForRecovery(ctx, job.ID, queue.StatusQueued, nextAttempt, &nextRetryAt, deref(lastError)); err != nil {
			logger.Error("failed to schedule retry; marking job terminal", "error", err)
			d.completeJob(ctx, logger, job.ID, job.Plugin, job.StartedAt, status, result, lastError, stderr)
			return
		}

		logger.Warn(
			"retry scheduled",
			"job_id", job.ID,
			"attempt", job.Attempt,
			"max_attempts", job.MaxAttempts,
			"backoff_ms", backoff.Milliseconds(),
			"next_retry_at", nextRetryAt.Format(time.RFC3339Nano),
			"reason", reason,
		)

		d.events.Publish("job.retry_scheduled", map[string]any{
			"job_id":       job.ID,
			"attempt":      job.Attempt,
			"max_attempts": job.MaxAttempts,
			"retry_count":  job.Attempt,
			"max_retries":  job.MaxAttempts,
			"backoff_ms":   backoff.Milliseconds(),
			"backoff_schedule": []int64{
				backoff.Milliseconds(),
			},
			"next_retry_at": nextRetryAt.Format(time.RFC3339Nano),
			"reason":        reason,
		})
		return
	}

	if retryable {
		d.events.Publish("job.retry_exhausted", map[string]any{
			"job_id":      job.ID,
			"attempts":    job.Attempt,
			"final_error": deref(lastError),
		})
	} else {
		d.events.Publish("job.retry_exhausted", map[string]any{
			"job_id":      job.ID,
			"attempts":    job.Attempt,
			"final_error": deref(lastError),
			"reason":      reason,
		})
	}

	hookPayload := map[string]any{
		"job_id":  job.ID,
		"plugin":  job.Plugin,
		"command": job.Command,
		"status":  string(status),
		"reason":  reason,
	}
	if lastError != nil {
		hookPayload["error"] = *lastError
	}
	d.maybeFireHooks(ctx, job, hookSignalForStatus(status), hookPayload)
	d.completeJob(ctx, logger, job.ID, job.Plugin, job.StartedAt, status, result, lastError, stderr)
}

func hookSignalForStatus(status queue.Status) string {
	switch status {
	case queue.StatusSucceeded:
		return "job.completed"
	case queue.StatusTimedOut:
		return "job.timed_out"
	case queue.StatusFailed, queue.StatusDead:
		return "job.failed"
	default:
		return ""
	}
}

func (d *Dispatcher) computeRetryDelay(retryCfg *config.RetryConfig, attempt int) time.Duration {
	base := config.DefaultPluginConf().Retry.BackoffBase
	if retryCfg != nil && retryCfg.BackoffBase > 0 {
		base = retryCfg.BackoffBase
	}

	jitter := time.Duration(0)
	maxJitter := base / 4
	if maxJitter > 0 {
		// #nosec G404 -- jitter is non-cryptographic scheduling noise.
		jitter = time.Duration(rand.Int63n(int64(maxJitter) + 1))
	}

	return calculateBackoffDelay(base, attempt, jitter)
}

func calculateBackoffDelay(base time.Duration, attempt int, jitter time.Duration) time.Duration {
	if base <= 0 {
		base = 30 * time.Second
	}
	if attempt < 1 {
		attempt = 1
	}

	factor := max(int64(math.Pow(2, float64(attempt-1))), 1)

	delay := time.Duration(factor) * base
	if delay < 0 {
		delay = base
	}
	delay += jitter
	if delay < 0 {
		delay = base
	}
	return delay
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
	out := make(map[string]any)
	out["ductile_plugin"] = job.Plugin

	if d.contexts == nil || job.EventContextID == nil {
		return out, nil
	}

	eventCtx, err := d.contexts.Get(ctx, *job.EventContextID)
	if err != nil {
		return nil, err
	}
	if len(eventCtx.AccumulatedJSON) > 0 {
		if err := json.Unmarshal(eventCtx.AccumulatedJSON, &out); err != nil {
			return nil, err
		}
	}
	if eventCtx.PipelineName != "" {
		out["ductile_pipeline"] = eventCtx.PipelineName
	}
	if eventCtx.StepID != "" {
		out["ductile_step_id"] = eventCtx.StepID
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
			if next.Event.Payload == nil {
				next.Event.Payload = make(map[string]any)
			}
			if _, exists := next.Event.Payload["ductile_upstream_plugin"]; !exists {
				next.Event.Payload["ductile_upstream_plugin"] = job.Plugin
			}
			if sourcePipeline != "" {
				if _, exists := next.Event.Payload["ductile_upstream_pipeline"]; !exists {
					next.Event.Payload["ductile_upstream_pipeline"] = sourcePipeline
				}
			}
			if sourceStepID != "" {
				if _, exists := next.Event.Payload["ductile_upstream_step_id"]; !exists {
					next.Event.Payload["ductile_upstream_step_id"] = sourceStepID
				}
			}

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
			if dedupeKey := strings.TrimSpace(next.Event.DedupeKey); dedupeKey != "" {
				enqueueReq.DedupeKey = &dedupeKey
			}
			childJobID, err := d.queue.Enqueue(ctx, enqueueReq)
			if err != nil {
				if errors.Is(err, queue.ErrDedupeDrop) {
					logger.Info("skipping routed job (deduplicated)",
						"plugin", next.Plugin,
						"dedup_key", strings.TrimSpace(next.Event.DedupeKey),
					)
					continue
				}
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

type preflightDecision string

const (
	preflightDecisionRun  preflightDecision = "run"
	preflightDecisionSkip preflightDecision = "skip"
)

type jobPreflight struct {
	decision       preflightDecision
	reason         string
	requestContext map[string]any
	workspaceDir   string
}

type conditionEvaluation struct {
	shouldSkip bool
	reason     string
}

func (d *Dispatcher) preflightJob(ctx context.Context, job *queue.Job) (jobPreflight, error) {
	requestContext, err := d.loadRequestContext(ctx, job)
	if err != nil {
		return jobPreflight{}, fmt.Errorf("load event context: %w", err)
	}

	// Prepare the workspace before evaluating first-class control-flow.
	//
	// A skipped step can still have downstream successors, and those children may
	// inherit/clone the parent's workspace even though no plugin process ran for
	// the skipped node. Treating workspace creation as pre-execution assurance
	// keeps the data-plane semantics consistent for both run and skip outcomes.
	workspaceDir, err := d.ensureWorkspaceForJob(ctx, job)
	if err != nil {
		return jobPreflight{}, fmt.Errorf("prepare workspace: %w", err)
	}

	if d.router != nil {
		evaluation, err := d.evaluateStepCondition(ctx, job, requestContext)
		if err != nil {
			return jobPreflight{}, fmt.Errorf("evaluate step condition: %w", err)
		}
		if evaluation.shouldSkip {
			return jobPreflight{
				decision:       preflightDecisionSkip,
				reason:         evaluation.reason,
				requestContext: requestContext,
				workspaceDir:   workspaceDir,
			}, nil
		}
	}

	return jobPreflight{
		decision:       preflightDecisionRun,
		requestContext: requestContext,
		workspaceDir:   workspaceDir,
	}, nil
}

func (d *Dispatcher) evaluateStepCondition(_ context.Context, job *queue.Job, requestContext map[string]any) (conditionEvaluation, error) {
	if d.router == nil {
		return conditionEvaluation{}, nil
	}

	pipelineName, _ := requestContext["ductile_pipeline"].(string)
	stepID, _ := requestContext["ductile_step_id"].(string)
	if pipelineName == "" || stepID == "" {
		return conditionEvaluation{}, nil
	}

	node, ok := d.router.GetNode(pipelineName, stepID)
	if !ok || node.Condition == nil {
		return conditionEvaluation{}, nil
	}

	scope := conditions.Scope{
		Payload: eventPayloadForCondition(job.Payload),
		Context: requestContext,
		Config:  pluginConfigMap(d.cfg, job.Plugin),
	}
	matched, err := conditions.Eval(node.Condition, scope)
	if err != nil {
		return conditionEvaluation{}, err
	}
	if matched {
		return conditionEvaluation{}, nil
	}
	return conditionEvaluation{shouldSkip: true, reason: "if condition evaluated false"}, nil
}

func pluginConfigMap(cfg *config.Config, pluginName string) map[string]any {
	if cfg == nil {
		return map[string]any{}
	}
	pluginCfg, ok := cfg.Plugins[pluginName]
	if !ok || pluginCfg.Config == nil {
		return map[string]any{}
	}
	return pluginCfg.Config
}

func eventPayloadForCondition(raw json.RawMessage) map[string]any {
	if len(raw) == 0 {
		return map[string]any{}
	}

	var event protocol.Event
	if err := json.Unmarshal(raw, &event); err == nil && (event.Type != "" || event.Payload != nil) {
		if event.Payload != nil {
			return event.Payload
		}
		return map[string]any{}
	}

	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err == nil {
		return payload
	}
	return map[string]any{}
}

func (d *Dispatcher) skipJob(ctx context.Context, logger *slog.Logger, job *queue.Job, requestContext map[string]any, reason string) {
	logger.Info("job skipped", "reason", reason)
	d.events.Publish("job.skipped", map[string]any{
		"job_id":  job.ID,
		"plugin":  job.Plugin,
		"command": job.Command,
		"reason":  reason,
	})

	result, err := json.Marshal(map[string]any{
		"status": "skipped",
		"reason": reason,
	})
	if err != nil {
		errMsg := fmt.Sprintf("failed to marshal skipped job result: %v", err)
		d.completeJob(ctx, logger, job.ID, job.Plugin, job.StartedAt, queue.StatusFailed, nil, &errMsg, nil)
		return
	}

	// Route successors before marking the skipped job terminal.
	//
	// A skipped first-class `if` step can still expand the execution tree. For
	// synchronous callers, notifying completion before successor enqueue can make
	// the tree appear finished too early. Preflight now ensures the skipped job
	// already has a workspace, so downstream clone/inheritance semantics remain
	// valid even though no plugin process ran.
	if err := d.routeSkippedStepSuccessors(ctx, logger, job, reason); err != nil {
		errMsg := fmt.Sprintf("failed to route successors after skip: %v", err)
		d.completeJob(ctx, logger, job.ID, job.Plugin, job.StartedAt, queue.StatusFailed, nil, &errMsg, nil)
		return
	}

	d.completeJob(ctx, logger, job.ID, job.Plugin, job.StartedAt, queue.StatusSkipped, result, &reason, nil)
}

func (d *Dispatcher) routeSkippedStepSuccessors(ctx context.Context, logger *slog.Logger, job *queue.Job, reason string) error {
	if err := d.routeEvents(ctx, job, []protocol.Event{{Type: "ductile.step.skipped", Payload: map[string]any{"reason": reason}}}, logger); err != nil {
		return err
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
func (d *Dispatcher) completeJob(ctx context.Context, logger *slog.Logger, jobID string, plugin string, startTime *time.Time, status queue.Status, result json.RawMessage, lastError, stderr *string) {
	duration := time.Duration(0)
	if startTime != nil {
		duration = time.Since(*startTime)
	}
	logger.Info("job completed", "status", string(status), "duration", duration.String())

	eventData := map[string]any{
		"job_id":      jobID,
		"plugin":      plugin,
		"status":      string(status),
		"duration_ms": duration.Milliseconds(),
	}
	if lastError != nil {
		eventData["error"] = *lastError
		msg := fmt.Sprintf("Job failed [%s]: %s", plugin, *lastError)
		eventData["message"] = msg
		eventData["text"] = msg
	}

	switch status {
	case queue.StatusSucceeded:
		d.events.Publish("job.completed", eventData)
	case queue.StatusTimedOut:
		msg := fmt.Sprintf("Job timed out [%s]", plugin)
		eventData["message"] = msg
		eventData["text"] = msg
		d.events.Publish("job.timed_out", eventData)
	case queue.StatusFailed, queue.StatusDead:
		if _, ok := eventData["message"]; !ok {
			msg := fmt.Sprintf("Job failed [%s]", plugin)
			eventData["message"] = msg
			eventData["text"] = msg
		}
		d.events.Publish("job.failed", eventData)
	}

	if err := d.queue.CompleteWithResult(ctx, jobID, status, result, lastError, stderr); err != nil {
		d.logger.Error("failed to complete job", "job_id", jobID, "error", err)
	}

	// Notify any synchronous waiters
	d.notifyCompletion(jobID)
}

// maybeFireHooks fires on-hook lifecycle pipelines for a completed root job if the plugin
// has notify_on_complete: true in its config. Pipeline steps and retried jobs are skipped.
func (d *Dispatcher) maybeFireHooks(ctx context.Context, job *queue.Job, signal string, payload map[string]any) {
	if strings.TrimSpace(signal) == "" {
		return
	}
	if job.EventContextID != nil {
		return // pipeline step — not a root job
	}
	pluginCfg, ok := d.cfg.Plugins[job.Plugin]
	if !ok || pluginCfg.NotifyOnComplete == nil || !*pluginCfg.NotifyOnComplete {
		return
	}
	if d.router == nil {
		return
	}
	dispatches, err := d.router.NextHook(ctx, job.Plugin, signal, payload)
	if err != nil {
		d.logger.Error("hook routing error", "plugin", job.Plugin, "signal", signal, "error", err)
		return
	}
	for _, disp := range dispatches {
		payloadJSON, err := json.Marshal(disp.Event)
		if err != nil {
			d.logger.Error("failed to marshal hook event", "plugin", disp.Plugin, "signal", signal, "error", err)
			continue
		}
		enqReq := queue.EnqueueRequest{
			Plugin:      disp.Plugin,
			Command:     disp.Command,
			Payload:     payloadJSON,
			SubmittedBy: "hook",
		}
		childID, err := d.queue.Enqueue(ctx, enqReq)
		if err != nil {
			d.logger.Error("failed to enqueue hook job", "plugin", disp.Plugin, "error", err)
			continue
		}
		d.logger.Info("enqueued hook job", "signal", signal, "source_plugin", job.Plugin, "hook_plugin", disp.Plugin, "child_job_id", childID)
	}
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

	// Initial check in case it's already done.
	//
	// We require a settled tree, not just a single "no running jobs" snapshot.
	// Structural control-flow such as a skipped `if` step can make the current job
	// terminal before all downstream child rows are visible to a concurrent waiter.
	// A second confirmation read avoids returning a truncated synchronous tree.
	results, complete, err := d.settledJobTree(ctx, rootJobID)
	if err != nil {
		return nil, fmt.Errorf("initial tree check: %w", err)
	}
	if complete {
		return results, nil
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
			// Something in the tree finished. Recheck until the visible tree is settled.
			results, complete, err := d.settledJobTree(ctx, rootJobID)
			if err != nil {
				return nil, fmt.Errorf("tree completion check: %w", err)
			}
			if complete {
				return results, nil
			}
		}
	}
}

func (d *Dispatcher) settledJobTree(ctx context.Context, rootJobID string) ([]*queue.JobResult, bool, error) {
	results, err := d.queue.GetJobTree(ctx, rootJobID)
	if err != nil {
		return nil, false, err
	}
	if len(results) == 0 {
		return nil, false, fmt.Errorf("job %s not found", rootJobID)
	}
	if !jobTreeComplete(results) {
		return results, false, nil
	}

	// Re-read once more before declaring success.
	// This tiny confirmation step protects synchronous callers from seeing a
	// truncated tree during structural transitions (for example, a skipped root
	// step whose successors have already been enqueued but were not visible on the
	// previous snapshot).
	confirmed, err := d.queue.GetJobTree(ctx, rootJobID)
	if err != nil {
		return nil, false, err
	}
	if len(confirmed) == 0 {
		return nil, false, fmt.Errorf("job %s not found", rootJobID)
	}
	if !jobTreeComplete(confirmed) {
		return confirmed, false, nil
	}
	if len(confirmed) != len(results) {
		return confirmed, false, nil
	}
	return confirmed, true, nil
}

func jobTreeComplete(results []*queue.JobResult) bool {
	for _, res := range results {
		switch res.Status {
		case queue.StatusQueued, queue.StatusRunning:
			return false
		}
	}
	return true
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

// truncateStderr truncates stderr to maxStderrBytes.
func truncateStderr(s string) string {
	if len(s) > maxStderrBytes {
		return s[:maxStderrBytes]
	}
	return s
}
