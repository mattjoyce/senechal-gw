package api

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mattjoyce/senechal-gw/internal/plugin"
	"github.com/mattjoyce/senechal-gw/internal/queue"
)

// mockQueue implements JobQueuer for testing
type mockQueue struct {
	enqueueFunc   func(ctx context.Context, req queue.EnqueueRequest) (string, error)
	getJobByIDFunc func(ctx context.Context, jobID string) (*queue.JobResult, error)
}

func (m *mockQueue) Enqueue(ctx context.Context, req queue.EnqueueRequest) (string, error) {
	return m.enqueueFunc(ctx, req)
}

func (m *mockQueue) GetJobByID(ctx context.Context, jobID string) (*queue.JobResult, error) {
	return m.getJobByIDFunc(ctx, jobID)
}

// mockRegistry implements PluginRegistry for testing
type mockRegistry struct {
	plugins map[string]*plugin.Plugin
}

func (m *mockRegistry) Get(name string) (*plugin.Plugin, bool) {
	p, ok := m.plugins[name]
	return p, ok
}

func newTestServer(q *mockQueue, reg *mockRegistry) *Server {
	logger := slog.Default()
	config := Config{
		Listen: "localhost:8080",
		APIKey: "test-key-123",
	}
	return New(config, q, reg, logger)
}

func TestHandleTrigger_Success(t *testing.T) {
	q := &mockQueue{
		enqueueFunc: func(ctx context.Context, req queue.EnqueueRequest) (string, error) {
			if req.Plugin != "echo" || req.Command != "poll" || req.SubmittedBy != "api" {
				t.Errorf("unexpected enqueue request: %+v", req)
			}
			return "job-123", nil
		},
	}

	reg := &mockRegistry{
		plugins: map[string]*plugin.Plugin{
			"echo": {
				Name:     "echo",
				Commands: []string{"poll", "handle"},
			},
		},
	}

	server := newTestServer(q, reg)

	body := bytes.NewBufferString(`{"payload": {"test": "data"}}`)
	req := httptest.NewRequest(http.MethodPost, "/trigger/echo/poll", body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-key-123")

	rr := httptest.NewRecorder()
	router := server.setupRoutes()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Errorf("expected status 202, got %d", rr.Code)
	}

	var resp TriggerResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.JobID != "job-123" {
		t.Errorf("expected job_id job-123, got %s", resp.JobID)
	}
	if resp.Status != "queued" {
		t.Errorf("expected status queued, got %s", resp.Status)
	}
	if resp.Plugin != "echo" {
		t.Errorf("expected plugin echo, got %s", resp.Plugin)
	}
	if resp.Command != "poll" {
		t.Errorf("expected command poll, got %s", resp.Command)
	}
}

func TestHandleTrigger_PluginNotFound(t *testing.T) {
	q := &mockQueue{}
	reg := &mockRegistry{
		plugins: map[string]*plugin.Plugin{},
	}

	server := newTestServer(q, reg)

	req := httptest.NewRequest(http.MethodPost, "/trigger/unknown/poll", nil)
	req.Header.Set("Authorization", "Bearer test-key-123")

	rr := httptest.NewRecorder()
	router := server.setupRoutes()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rr.Code)
	}

	var resp ErrorResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Error != "plugin not found" {
		t.Errorf("expected error 'plugin not found', got %s", resp.Error)
	}
}

func TestHandleTrigger_CommandNotSupported(t *testing.T) {
	q := &mockQueue{}
	reg := &mockRegistry{
		plugins: map[string]*plugin.Plugin{
			"echo": {
				Name:     "echo",
				Commands: []string{"poll"},
			},
		},
	}

	server := newTestServer(q, reg)

	req := httptest.NewRequest(http.MethodPost, "/trigger/echo/unknown", nil)
	req.Header.Set("Authorization", "Bearer test-key-123")

	rr := httptest.NewRecorder()
	router := server.setupRoutes()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rr.Code)
	}

	var resp ErrorResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Error != "command not supported by plugin" {
		t.Errorf("expected error 'command not supported by plugin', got %s", resp.Error)
	}
}

func TestHandleTrigger_Unauthorized(t *testing.T) {
	q := &mockQueue{}
	reg := &mockRegistry{}

	server := newTestServer(q, reg)

	req := httptest.NewRequest(http.MethodPost, "/trigger/echo/poll", nil)
	// No Authorization header

	rr := httptest.NewRecorder()
	router := server.setupRoutes()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", rr.Code)
	}

	var resp ErrorResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Error != "missing Authorization header" {
		t.Errorf("expected error 'missing Authorization header', got %s", resp.Error)
	}
}

func TestHandleTrigger_InvalidAPIKey(t *testing.T) {
	q := &mockQueue{}
	reg := &mockRegistry{}

	server := newTestServer(q, reg)

	req := httptest.NewRequest(http.MethodPost, "/trigger/echo/poll", nil)
	req.Header.Set("Authorization", "Bearer wrong-key")

	rr := httptest.NewRecorder()
	router := server.setupRoutes()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", rr.Code)
	}

	var resp ErrorResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Error != "invalid API key" {
		t.Errorf("expected error 'invalid API key', got %s", resp.Error)
	}
}

func TestHandleGetJob_Success(t *testing.T) {
	q := &mockQueue{
		getJobByIDFunc: func(ctx context.Context, jobID string) (*queue.JobResult, error) {
			if jobID != "job-123" {
				t.Errorf("unexpected job_id: %s", jobID)
			}
			return &queue.JobResult{
				JobID:   "job-123",
				Status:  queue.StatusSucceeded,
				Plugin:  "echo",
				Command: "poll",
			}, nil
		},
	}

	reg := &mockRegistry{}

	server := newTestServer(q, reg)

	req := httptest.NewRequest(http.MethodGet, "/job/job-123", nil)
	req.Header.Set("Authorization", "Bearer test-key-123")

	rr := httptest.NewRecorder()
	router := server.setupRoutes()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}

	var resp JobStatusResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.JobID != "job-123" {
		t.Errorf("expected job_id job-123, got %s", resp.JobID)
	}
	if resp.Status != "succeeded" {
		t.Errorf("expected status succeeded, got %s", resp.Status)
	}
	if resp.Plugin != "echo" {
		t.Errorf("expected plugin echo, got %s", resp.Plugin)
	}
}

func TestHandleGetJob_NotFound(t *testing.T) {
	q := &mockQueue{
		getJobByIDFunc: func(ctx context.Context, jobID string) (*queue.JobResult, error) {
			return nil, queue.ErrJobNotFound
		},
	}

	reg := &mockRegistry{}

	server := newTestServer(q, reg)

	req := httptest.NewRequest(http.MethodGet, "/job/unknown", nil)
	req.Header.Set("Authorization", "Bearer test-key-123")

	rr := httptest.NewRecorder()
	router := server.setupRoutes()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", rr.Code)
	}

	var resp ErrorResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Error != "job not found" {
		t.Errorf("expected error 'job not found', got %s", resp.Error)
	}
}
