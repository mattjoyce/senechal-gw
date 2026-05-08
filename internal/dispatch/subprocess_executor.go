package dispatch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"time"

	"github.com/mattjoyce/ductile/internal/events"
	"github.com/mattjoyce/ductile/internal/protocol"
)

type subprocessExecutor struct {
	events *events.Hub
}

func newSubprocessExecutor(events *events.Hub) *subprocessExecutor {
	return &subprocessExecutor{events: events}
}

// spawnPlugin spawns the plugin subprocess, writes the request to stdin, and reads the response from stdout.
// Returns the parsed response, raw response bytes, raw stdout bytes, stderr output, exit code, and any error.
func (e *subprocessExecutor) execute(
	ctx context.Context,
	pluginName string,
	entrypoint string,
	req *protocol.Request,
	timeout time.Duration,
	logger *slog.Logger,
) (*protocol.Response, protocol.ResponseCompat, json.RawMessage, []byte, string, int, error) {
	// Create timer for timeout enforcement
	timeoutTimer := time.NewTimer(timeout)
	defer timeoutTimer.Stop()

	// Prepare command (don't use CommandContext - we'll manage termination ourselves)
	cmd := exec.Command(entrypoint)
	configurePluginProcess(cmd)

	// Prepare stdin pipe
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, protocol.ResponseCompat{}, nil, nil, "", 0, fmt.Errorf("create stdin pipe: %w", err)
	}

	// Capture stdout and stderr with hard memory caps. Stdout is the protocol
	// channel, so overflow is a protocol failure; stderr is diagnostic evidence
	// and is truncated without failing an otherwise valid response.
	stdout := newBoundedBuffer(maxStdoutBytes)
	stderr := newBoundedBuffer(maxStderrBytes)
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	logger.Info("plugin executing", "entrypoint", entrypoint, "timeout", timeout)

	// Start the process
	if err := cmd.Start(); err != nil {
		return nil, protocol.ResponseCompat{}, nil, nil, "", 0, fmt.Errorf("start process: %w", err)
	}

	if e.events != nil {
		e.events.Publish("plugin.spawned", map[string]any{
			"job_id":  req.JobID,
			"plugin":  pluginName,
			"command": req.Command,
			"pid":     cmd.Process.Pid,
		})
	}

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
		if err := terminatePluginProcess(cmd); err != nil {
			logger.Error("failed to send SIGTERM", "error", err)
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
			if err := killPluginProcess(cmd); err != nil {
				logger.Error("failed to send SIGKILL", "error", err)
			}
			<-waitErr // Wait for process to die
		}

		if stdout.Truncated() {
			logger.Warn("plugin stdout exceeded capture limit", "limit_bytes", maxStdoutBytes)
		}
		if stderr.Truncated() {
			logger.Warn("plugin stderr truncated", "limit_bytes", maxStderrBytes)
		}
		stderrStr := stderr.String()
		return nil, protocol.ResponseCompat{}, nil, stdout.Bytes(), stderrStr, 0, context.DeadlineExceeded

	case err := <-waitErr:
		// Process completed
		werr := <-writeErr

		stdoutBytes := stdout.Bytes()
		stderrStr := stderr.String()
		if stderr.Truncated() {
			logger.Warn("plugin stderr truncated", "limit_bytes", maxStderrBytes)
		}
		exitCode := 0

		// Check exit code
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
				logger.Warn("plugin exited with non-zero status", "exit_code", exitCode)
			} else {
				return nil, protocol.ResponseCompat{}, nil, stdoutBytes, stderrStr, 0, fmt.Errorf("wait for process: %w", err)
			}
		}

		if stdout.Truncated() {
			logger.Error("plugin stdout exceeded capture limit", "limit_bytes", maxStdoutBytes)
			return nil, protocol.ResponseCompat{}, json.RawMessage(stdoutBytes), stdoutBytes, stderrStr, exitCode, outputLimitError{stream: "stdout", limit: maxStdoutBytes}
		}

		// Decode response from stdout
		resp, compat, rawBytes, err := protocol.DecodeResponseLenient(bytes.NewReader(stdoutBytes))
		if err != nil {
			// If we also had a stdin write error, include it for diagnostics
			if werr != nil {
				logger.Warn("stdin write failed (process may not read stdin)", "error", werr)
			}
			logger.Error("failed to decode plugin response", "error", err, "stdout", string(rawBytes))
			return nil, protocol.ResponseCompat{}, json.RawMessage(rawBytes), stdoutBytes, stderrStr, exitCode, fmt.Errorf("decode response: %w", err)
		}

		// Log stdin write errors as warnings — some plugins don't read stdin
		// and may exit before the write completes, which is not a failure if
		// the process produced a valid response.
		if werr != nil {
			logger.Debug("stdin write error (ignored, valid response received)", "error", werr)
		}

		return resp, compat, json.RawMessage(rawBytes), stdoutBytes, stderrStr, exitCode, nil
	}
}
