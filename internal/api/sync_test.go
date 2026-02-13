package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mattjoyce/senechal-gw/internal/plugin"
	"github.com/mattjoyce/senechal-gw/internal/queue"
	"github.com/mattjoyce/senechal-gw/internal/router"
)

func TestHandleTrigger_SyncSuccess(t *testing.T) {
	q := &mockQueue{
		enqueueFunc: func(ctx context.Context, req queue.EnqueueRequest) (string, error) {
			return "job-sync-123", nil
		},
	}

	reg := &mockRegistry{
		plugins: map[string]*plugin.Plugin{
			"echo": {
				Name: "echo",
				Commands: plugin.Commands{
					{Name: "poll", Type: plugin.CommandTypeWrite},
				},
			},
		},
	}

	rt := &mockRouter{
		getPipelineByTriggerFunc: func(trigger string) *router.PipelineInfo {
			if trigger == "echo.poll" {
				return &router.PipelineInfo{
					Name:          "sync-pipeline",
					Trigger:       "echo.poll",
					ExecutionMode: "synchronous",
					Timeout:       5 * time.Second,
				}
			}
			return nil
		},
	}

	wt := &mockWaiter{
		waitForJobTreeFunc: func(ctx context.Context, rootJobID string, timeout time.Duration) ([]*queue.JobResult, error) {
			return []*queue.JobResult{
				{
					JobID:   rootJobID,
					Plugin:  "echo",
					Command: "poll",
					Status:  queue.StatusSucceeded,
					Result:  json.RawMessage(`{"status":"ok"}`),
				},
			}, nil
		},
	}

	server := New(Config{APIKey: "test"}, q, reg, rt, wt, slog.Default())

	req := httptest.NewRequest(http.MethodPost, "/trigger/echo/poll", nil)
	req.Header.Set("Authorization", "Bearer test")
	rr := httptest.NewRecorder()

	server.setupRoutes().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	var resp SyncResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.JobID != "job-sync-123" {
		t.Errorf("expected job_id job-sync-123, got %s", resp.JobID)
	}
	if resp.Status != "succeeded" {
		t.Errorf("expected status succeeded, got %s", resp.Status)
	}
	if len(resp.Tree) != 1 {
		t.Errorf("expected 1 tree entry, got %d", len(resp.Tree))
	}
}

func TestHandleTrigger_SyncTimeout(t *testing.T) {
	q := &mockQueue{
		enqueueFunc: func(ctx context.Context, req queue.EnqueueRequest) (string, error) {
			return "job-timeout-123", nil
		},
	}

	reg := &mockRegistry{
		plugins: map[string]*plugin.Plugin{
			"echo": {
				Name: "echo",
				Commands: plugin.Commands{
					{Name: "poll", Type: plugin.CommandTypeWrite},
				},
			},
		},
	}

	rt := &mockRouter{
		getPipelineByTriggerFunc: func(trigger string) *router.PipelineInfo {
			return &router.PipelineInfo{
				Name:          "sync-pipeline",
				Trigger:       "echo.poll",
				ExecutionMode: "synchronous",
				Timeout:       100 * time.Millisecond,
			}
		},
	}

	wt := &mockWaiter{
		waitForJobTreeFunc: func(ctx context.Context, rootJobID string, timeout time.Duration) ([]*queue.JobResult, error) {
			return nil, context.DeadlineExceeded
		},
	}

	server := New(Config{APIKey: "test"}, q, reg, rt, wt, slog.Default())

	req := httptest.NewRequest(http.MethodPost, "/trigger/echo/poll", nil)
	req.Header.Set("Authorization", "Bearer test")
	rr := httptest.NewRecorder()

	server.setupRoutes().ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected status 202, got %d", rr.Code)
	}

	var resp TimeoutResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.JobID != "job-timeout-123" {
		t.Errorf("expected job_id job-timeout-123, got %s", resp.JobID)
	}
	if !resp.TimeoutExceeded {
		t.Error("expected timeout_exceeded to be true")
	}
}

func TestHandleTrigger_SyncLimitReached(t *testing.T) {
	q := &mockQueue{
		enqueueFunc: func(ctx context.Context, req queue.EnqueueRequest) (string, error) {
			return "job-limit-123", nil
		},
	}

	reg := &mockRegistry{
		plugins: map[string]*plugin.Plugin{
			"echo": {
				Name: "echo",
				Commands: plugin.Commands{
					{Name: "poll", Type: plugin.CommandTypeWrite},
				},
			},
		},
	}

	rt := &mockRouter{
		getPipelineByTriggerFunc: func(trigger string) *router.PipelineInfo {
			return &router.PipelineInfo{
				Name:          "sync-pipeline",
				Trigger:       "echo.poll",
				ExecutionMode: "synchronous",
				Timeout:       5 * time.Second,
			}
		},
	}

	wt := &mockWaiter{}

	// Create server with limit of 1
	server := New(Config{APIKey: "test", MaxConcurrentSync: 1}, q, reg, rt, wt, slog.Default())

	// Occupy the semaphore
	server.syncSemaphore <- struct{}{}

	req := httptest.NewRequest(http.MethodPost, "/trigger/echo/poll", nil)
	req.Header.Set("Authorization", "Bearer test")
	rr := httptest.NewRecorder()

	server.setupRoutes().ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status 503, got %d", rr.Code)
	}

	var resp ErrorResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if !strings.Contains(resp.Error, "too many concurrent synchronous requests") {
		t.Errorf("expected limit reached error, got %q", resp.Error)
	}
}
