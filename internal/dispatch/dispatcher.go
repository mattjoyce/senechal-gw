package dispatch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"syscall"
	"time"

	"github.com/mattjoyce/senechal-gw/internal/config"
	"github.com/mattjoyce/senechal-gw/internal/log"
	"github.com/mattjoyce/senechal-gw/internal/plugin"
	"github.com/mattjoyce/senechal-gw/internal/protocol"
	"github.com/mattjoyce/senechal-gw/internal/queue"
	"github.com/mattjoyce/senechal-gw/internal/state"
)

const (
	// maxStderrBytes caps the amount of stderr captured from plugin execution.
	maxStderrBytes = 64 * 1024

	// terminationGracePeriod is the time we wait after SIGTERM before sending SIGKILL.
	terminationGracePeriod = 5 * time.Second
)

// Dispatcher dequeues jobs and executes them by spawning plugin subprocesses.
type Dispatcher struct {
	queue    *queue.Queue
	state    *state.Store
	registry *plugin.Registry
	cfg      *config.Config
	logger   *slog.Logger
}

// New creates a new Dispatcher.
func New(q *queue.Queue, st *state.Store, reg *plugin.Registry, cfg *config.Config) *Dispatcher {
	return &Dispatcher{
		queue:    q,
		state:    st,
		registry: reg,
		cfg:      cfg,
		logger:   log.WithComponent("dispatch"),
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
	jobLogger := log.WithJob(job.ID).With("plugin", job.Plugin, "command", job.Command)
	jobLogger.Info("executing job", "attempt", job.Attempt)

	// Get plugin from registry
	plug, ok := d.registry.Get(job.Plugin)
	if !ok {
		errMsg := fmt.Sprintf("plugin %q not found in registry", job.Plugin)
		jobLogger.Error(errMsg)
		d.completeJob(ctx, job.ID, queue.StatusFailed, &errMsg, nil)
		return
	}

	// Check if plugin supports this command
	if !plug.SupportsCommand(job.Command) {
		errMsg := fmt.Sprintf("plugin %q does not support command %q", job.Plugin, job.Command)
		jobLogger.Error(errMsg)
		d.completeJob(ctx, job.ID, queue.StatusFailed, &errMsg, nil)
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
		d.completeJob(ctx, job.ID, queue.StatusFailed, &errMsg, nil)
		return
	}

	// Unmarshal state into map
	var stateMap map[string]interface{}
	if err := json.Unmarshal(pluginState, &stateMap); err != nil {
		errMsg := fmt.Sprintf("failed to unmarshal plugin state: %v", err)
		jobLogger.Error(errMsg)
		d.completeJob(ctx, job.ID, queue.StatusFailed, &errMsg, nil)
		return
	}

	// Determine timeout for this command
	timeout := d.getTimeout(pluginCfg.Timeouts, job.Command)
	deadline := time.Now().Add(timeout)

	// Build protocol request
	req := &protocol.Request{
		Protocol:   1,
		JobID:      job.ID,
		Command:    job.Command,
		Config:     pluginCfg.Config,
		State:      stateMap,
		DeadlineAt: deadline,
	}

	// For handle command, parse and include event payload
	if job.Command == "handle" && len(job.Payload) > 0 {
		var event protocol.Event
		if err := json.Unmarshal(job.Payload, &event); err != nil {
			errMsg := fmt.Sprintf("failed to unmarshal event payload: %v", err)
			jobLogger.Error(errMsg)
			d.completeJob(ctx, job.ID, queue.StatusFailed, &errMsg, nil)
			return
		}
		req.Event = &event
	}

	// Spawn plugin and execute
	resp, stderr, err := d.spawnPlugin(ctx, plug.Entrypoint, req, timeout, jobLogger)

	// Handle timeout (check if error is context.DeadlineExceeded)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			errMsg := fmt.Sprintf("plugin execution timed out after %v", timeout)
			jobLogger.Warn(errMsg)
			d.completeJob(ctx, job.ID, queue.StatusTimedOut, &errMsg, &stderr)
			return
		}

		// Handle other spawn errors
		errMsg := fmt.Sprintf("plugin spawn failed: %v", err)
		jobLogger.Error(errMsg)
		d.completeJob(ctx, job.ID, queue.StatusFailed, &errMsg, &stderr)
		return
	}

	// Handle protocol errors
	if resp == nil {
		errMsg := "plugin returned nil response"
		jobLogger.Error(errMsg)
		d.completeJob(ctx, job.ID, queue.StatusFailed, &errMsg, &stderr)
		return
	}

	// Log plugin logs
	for _, entry := range resp.Logs {
		jobLogger.Info("plugin log", "level", entry.Level, "message", entry.Message)
	}

	// Handle response status
	if resp.Status == "error" {
		jobLogger.Warn("plugin returned error", "error", resp.Error)
		d.completeJob(ctx, job.ID, queue.StatusFailed, &resp.Error, &stderr)
		// TODO: Handle retry logic based on resp.ShouldRetry() - not in MVP
		return
	}

	// Apply state updates
	if len(resp.StateUpdates) > 0 {
		updatesJSON, err := json.Marshal(resp.StateUpdates)
		if err != nil {
			errMsg := fmt.Sprintf("failed to marshal state updates: %v", err)
			jobLogger.Error(errMsg)
			d.completeJob(ctx, job.ID, queue.StatusFailed, &errMsg, &stderr)
			return
		}

		if _, err := d.state.ShallowMerge(ctx, job.Plugin, updatesJSON); err != nil {
			errMsg := fmt.Sprintf("failed to apply state updates: %v", err)
			jobLogger.Error(errMsg)
			d.completeJob(ctx, job.ID, queue.StatusFailed, &errMsg, &stderr)
			return
		}
		jobLogger.Debug("applied state updates", "updates", resp.StateUpdates)
	}

	// Process events (stubbed for MVP - routing not implemented)
	if len(resp.Events) > 0 {
		jobLogger.Info("plugin emitted events (routing not implemented)", "count", len(resp.Events))
		// TODO: Enqueue routing jobs based on config.Routes
	}

	// Mark job as succeeded
	jobLogger.Info("job completed successfully")
	d.completeJob(ctx, job.ID, queue.StatusSucceeded, nil, &stderr)
}

// spawnPlugin spawns the plugin subprocess, writes the request to stdin, and reads the response from stdout.
// Returns the response, stderr output, and any error.
func (d *Dispatcher) spawnPlugin(
	ctx context.Context,
	entrypoint string,
	req *protocol.Request,
	timeout time.Duration,
	logger *slog.Logger,
) (*protocol.Response, string, error) {
	// Create timer for timeout enforcement
	timeoutTimer := time.NewTimer(timeout)
	defer timeoutTimer.Stop()

	// Prepare command (don't use CommandContext - we'll manage termination ourselves)
	cmd := exec.Command(entrypoint)

	// Prepare stdin pipe
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, "", fmt.Errorf("create stdin pipe: %w", err)
	}

	// Capture stdout and stderr
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	logger.Debug("spawning plugin", "entrypoint", entrypoint, "timeout", timeout)

	// Start the process
	if err := cmd.Start(); err != nil {
		return nil, "", fmt.Errorf("start process: %w", err)
	}

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
		return nil, stderrStr, context.DeadlineExceeded

	case err := <-waitErr:
		// Process completed
		if werr := <-writeErr; werr != nil {
			stderrStr := truncateStderr(stderr.String())
			return nil, stderrStr, werr
		}

		stderrStr := truncateStderr(stderr.String())

		// Check exit code
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				logger.Warn("plugin exited with non-zero status", "exit_code", exitErr.ExitCode())
			} else {
				return nil, stderrStr, fmt.Errorf("wait for process: %w", err)
			}
		}

		// Decode response from stdout
		resp, rawBytes, err := protocol.DecodeResponseLenient(bytes.NewReader(stdout.Bytes()))
		if err != nil {
			logger.Error("failed to decode plugin response", "error", err, "stdout", string(rawBytes))
			return nil, stderrStr, fmt.Errorf("decode response: %w", err)
		}

		return resp, stderrStr, nil
	}
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

// completeJob marks a job as complete with the given status.
func (d *Dispatcher) completeJob(ctx context.Context, jobID string, status queue.Status, lastError, stderr *string) {
	if err := d.queue.Complete(ctx, jobID, status, lastError, stderr); err != nil {
		d.logger.Error("failed to complete job", "job_id", jobID, "error", err)
	}
}

// truncateStderr truncates stderr to maxStderrBytes.
func truncateStderr(s string) string {
	if len(s) > maxStderrBytes {
		return s[:maxStderrBytes]
	}
	return s
}
