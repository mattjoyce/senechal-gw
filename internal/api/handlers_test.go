package api

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mattjoyce/ductile/internal/auth"
	"github.com/mattjoyce/ductile/internal/events"
	"github.com/mattjoyce/ductile/internal/plugin"
	"github.com/mattjoyce/ductile/internal/protocol"
	"github.com/mattjoyce/ductile/internal/queue"
	"github.com/mattjoyce/ductile/internal/router"
	"github.com/mattjoyce/ductile/internal/state"
)

// mockQueue implements JobQueuer for testing
type mockQueue struct {
	enqueueFunc    func(ctx context.Context, req queue.EnqueueRequest) (string, error)
	getJobByIDFunc func(ctx context.Context, jobID string) (*queue.JobResult, error)
	depthFunc      func(ctx context.Context) (int, error)
}

func (m *mockQueue) Enqueue(ctx context.Context, req queue.EnqueueRequest) (string, error) {
	return m.enqueueFunc(ctx, req)
}

func (m *mockQueue) GetJobByID(ctx context.Context, jobID string) (*queue.JobResult, error) {
	return m.getJobByIDFunc(ctx, jobID)
}

func (m *mockQueue) Depth(ctx context.Context) (int, error) {
	if m.depthFunc == nil {
		return 0, nil
	}
	return m.depthFunc(ctx)
}

// mockRegistry implements PluginRegistry for testing
type mockRegistry struct {
	plugins map[string]*plugin.Plugin
}

func (m *mockRegistry) Get(name string) (*plugin.Plugin, bool) {
	p, ok := m.plugins[name]
	return p, ok
}

func (m *mockRegistry) All() map[string]*plugin.Plugin {
	if m.plugins == nil {
		return map[string]*plugin.Plugin{}
	}
	return m.plugins
}

func (m *mockQueue) GetJobTree(ctx context.Context, rootJobID string) ([]*queue.JobResult, error) {
	return nil, nil
}

// mockRouter implements PipelineRouter for testing
type mockRouter struct {
	getPipelineByTriggerFunc func(trigger string) *router.PipelineInfo
}

func (m *mockRouter) GetPipelineByTrigger(trigger string) *router.PipelineInfo {
	if m.getPipelineByTriggerFunc != nil {
		return m.getPipelineByTriggerFunc(trigger)
	}
	return nil
}

// mockWaiter implements TreeWaiter for testing
type mockWaiter struct {
	waitForJobTreeFunc func(ctx context.Context, rootJobID string, timeout time.Duration) ([]*queue.JobResult, error)
}

func (m *mockWaiter) WaitForJobTree(ctx context.Context, rootJobID string, timeout time.Duration) ([]*queue.JobResult, error) {
	if m.waitForJobTreeFunc != nil {
		return m.waitForJobTreeFunc(ctx, rootJobID, timeout)
	}
	return nil, nil
}

type mockContextStore struct {
	createFunc func(ctx context.Context, parentID *string, pipelineName, stepID string, updates json.RawMessage) (*state.EventContext, error)
}

func (m *mockContextStore) Create(ctx context.Context, parentID *string, pipelineName, stepID string, updates json.RawMessage) (*state.EventContext, error) {
	if m.createFunc != nil {
		return m.createFunc(ctx, parentID, pipelineName, stepID, updates)
	}
	return &state.EventContext{ID: "ctx-default", PipelineName: pipelineName, StepID: stepID}, nil
}

func newTestServer(q *mockQueue, reg *mockRegistry) *Server {
	logger := slog.Default()
	config := Config{
		Listen: "localhost:8080",
		APIKey: "test-key-123",
	}
	hub := events.NewHub(10)
	return New(config, q, reg, &mockRouter{}, &mockWaiter{}, nil, hub, logger)
}

func TestHandleHealthz_NoAuth(t *testing.T) {
	q := &mockQueue{
		depthFunc: func(ctx context.Context) (int, error) { return 7, nil },
	}
	reg := &mockRegistry{
		plugins: map[string]*plugin.Plugin{
			"echo": {Name: "echo"},
		},
	}

	server := newTestServer(q, reg)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()
	router := server.setupRoutes()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	var resp HealthzResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Status != "ok" {
		t.Fatalf("expected status ok, got %q", resp.Status)
	}
	if resp.QueueDepth != 7 {
		t.Fatalf("expected queue_depth 7, got %d", resp.QueueDepth)
	}
	if resp.PluginsLoaded != 1 {
		t.Fatalf("expected plugins_loaded 1, got %d", resp.PluginsLoaded)
	}
	if resp.UptimeSeconds < 0 {
		t.Fatalf("expected non-negative uptime_seconds")
	}
}

func TestHandleTrigger_PluginROToken_CannotInvokeWriteCommand(t *testing.T) {
	q := &mockQueue{
		enqueueFunc: func(ctx context.Context, req queue.EnqueueRequest) (string, error) {
			t.Fatalf("enqueue should not be called for forbidden request")
			return "", nil
		},
	}

	reg := &mockRegistry{
		plugins: map[string]*plugin.Plugin{
			"echo": {
				Name: "echo",
				Commands: plugin.Commands{
					{Name: "poll", Type: plugin.CommandTypeWrite},
					{Name: "health", Type: plugin.CommandTypeRead},
				},
			},
		},
	}

	server := newTestServer(q, reg)
	server.config.Tokens = []auth.TokenConfig{
		{Token: "ro-token", Scopes: []string{"plugin:ro"}},
	}

	req := httptest.NewRequest(http.MethodPost, "/trigger/echo/poll", nil)
	req.Header.Set("Authorization", "Bearer ro-token")

	rr := httptest.NewRecorder()
	router := server.setupRoutes()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d", rr.Code)
	}
}

func TestHandleTrigger_PluginROToken_CanInvokeReadCommand(t *testing.T) {
	q := &mockQueue{
		enqueueFunc: func(ctx context.Context, req queue.EnqueueRequest) (string, error) {
			if req.Plugin != "echo" || req.Command != "health" || req.SubmittedBy != "api" {
				t.Errorf("unexpected enqueue request: %+v", req)
			}
			return "job-999", nil
		},
	}

	reg := &mockRegistry{
		plugins: map[string]*plugin.Plugin{
			"echo": {
				Name: "echo",
				Commands: plugin.Commands{
					{Name: "poll", Type: plugin.CommandTypeWrite},
					{Name: "health", Type: plugin.CommandTypeRead},
				},
			},
		},
	}

	server := newTestServer(q, reg)
	server.config.Tokens = []auth.TokenConfig{
		{Token: "ro-token", Scopes: []string{"plugin:ro"}},
	}

	req := httptest.NewRequest(http.MethodPost, "/trigger/echo/health", nil)
	req.Header.Set("Authorization", "Bearer ro-token")

	rr := httptest.NewRecorder()
	router := server.setupRoutes()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected status 202, got %d", rr.Code)
	}
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
				Name: "echo",
				Commands: plugin.Commands{
					{Name: "poll", Type: plugin.CommandTypeWrite},
					{Name: "handle", Type: plugin.CommandTypeWrite},
				},
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

func TestHandleTrigger_PipelineTriggerCreatesEventContext(t *testing.T) {
	var captured queue.EnqueueRequest

	q := &mockQueue{
		enqueueFunc: func(ctx context.Context, req queue.EnqueueRequest) (string, error) {
			captured = req
			return "job-ctx-123", nil
		},
	}

	reg := &mockRegistry{
		plugins: map[string]*plugin.Plugin{
			"echo": {
				Name: "echo",
				Commands: plugin.Commands{
					{Name: "handle", Type: plugin.CommandTypeWrite},
				},
			},
		},
	}

	rt := &mockRouter{
		getPipelineByTriggerFunc: func(trigger string) *router.PipelineInfo {
			if trigger != "echo.handle" {
				t.Fatalf("unexpected trigger lookup: %s", trigger)
			}
			return &router.PipelineInfo{
				Name:        "echo-pipeline",
				Trigger:     "echo.handle",
				EntryStepID: "step-1",
			}
		},
	}

	contextStore := &mockContextStore{
		createFunc: func(ctx context.Context, parentID *string, pipelineName, stepID string, updates json.RawMessage) (*state.EventContext, error) {
			if parentID != nil {
				t.Fatalf("expected nil parentID for root context, got %v", *parentID)
			}
			if pipelineName != "echo-pipeline" {
				t.Fatalf("pipelineName = %q, want %q", pipelineName, "echo-pipeline")
			}
			if stepID != "step-1" {
				t.Fatalf("stepID = %q, want %q", stepID, "step-1")
			}
			return &state.EventContext{
				ID:           "ctx-123",
				PipelineName: pipelineName,
				StepID:       stepID,
			}, nil
		},
	}

	server := New(
		Config{Listen: "localhost:8080", APIKey: "test-key-123"},
		q,
		reg,
		rt,
		&mockWaiter{},
		contextStore,
		events.NewHub(10),
		slog.Default(),
	)

	req := httptest.NewRequest(http.MethodPost, "/trigger/echo/handle", bytes.NewBufferString(`{"payload":{"k":"v"}}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-key-123")
	rr := httptest.NewRecorder()
	server.setupRoutes().ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected status 202, got %d", rr.Code)
	}
	if captured.EventContextID == nil {
		t.Fatal("expected enqueue EventContextID to be set")
	}
	if *captured.EventContextID != "ctx-123" {
		t.Fatalf("EventContextID = %q, want %q", *captured.EventContextID, "ctx-123")
	}
}

func TestHandleTrigger_PipelineContextCreateFailure(t *testing.T) {
	q := &mockQueue{
		enqueueFunc: func(ctx context.Context, req queue.EnqueueRequest) (string, error) {
			t.Fatal("enqueue should not be called when context creation fails")
			return "", nil
		},
	}

	reg := &mockRegistry{
		plugins: map[string]*plugin.Plugin{
			"echo": {
				Name: "echo",
				Commands: plugin.Commands{
					{Name: "handle", Type: plugin.CommandTypeWrite},
				},
			},
		},
	}

	rt := &mockRouter{
		getPipelineByTriggerFunc: func(trigger string) *router.PipelineInfo {
			return &router.PipelineInfo{Name: "echo-pipeline", Trigger: trigger, EntryStepID: "step-1"}
		},
	}

	contextStore := &mockContextStore{
		createFunc: func(ctx context.Context, parentID *string, pipelineName, stepID string, updates json.RawMessage) (*state.EventContext, error) {
			return nil, context.DeadlineExceeded
		},
	}

	server := New(
		Config{Listen: "localhost:8080", APIKey: "test-key-123"},
		q,
		reg,
		rt,
		&mockWaiter{},
		contextStore,
		events.NewHub(10),
		slog.Default(),
	)

	req := httptest.NewRequest(http.MethodPost, "/trigger/echo/handle", bytes.NewBufferString(`{"payload":{"k":"v"}}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-key-123")
	rr := httptest.NewRecorder()
	server.setupRoutes().ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", rr.Code)
	}
}

func TestHandleTrigger_HandleWrapsPayload(t *testing.T) {
	var capturedPayload json.RawMessage
	q := &mockQueue{
		enqueueFunc: func(ctx context.Context, req queue.EnqueueRequest) (string, error) {
			capturedPayload = req.Payload
			return "job-123", nil
		},
	}

	reg := &mockRegistry{
		plugins: map[string]*plugin.Plugin{
			"echo": {
				Name: "echo",
				Commands: plugin.Commands{
					{Name: "handle", Type: plugin.CommandTypeWrite},
				},
			},
		},
	}

	server := newTestServer(q, reg)

	t.Run("non-empty body", func(t *testing.T) {
		body := bytes.NewBufferString(`{"payload": {"key": "val"}}`)
		req := httptest.NewRequest(http.MethodPost, "/trigger/echo/handle", body)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer test-key-123")

		rr := httptest.NewRecorder()
		server.setupRoutes().ServeHTTP(rr, req)

		if rr.Code != http.StatusAccepted {
			t.Fatalf("expected status 202, got %d", rr.Code)
		}

		var event protocol.Event
		if err := json.Unmarshal(capturedPayload, &event); err != nil {
			t.Fatalf("failed to unmarshal wrapped event: %v", err)
		}
		if event.Type != "api.trigger" {
			t.Errorf("expected type api.trigger, got %q", event.Type)
		}
		if event.Payload["key"] != "val" {
			t.Errorf("expected payload key=val, got %v", event.Payload)
		}
	})

	t.Run("empty body", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/trigger/echo/handle", nil)
		req.Header.Set("Authorization", "Bearer test-key-123")

		rr := httptest.NewRecorder()
		server.setupRoutes().ServeHTTP(rr, req)

		if rr.Code != http.StatusAccepted {
			t.Fatalf("expected status 202, got %d", rr.Code)
		}

		var event protocol.Event
		if err := json.Unmarshal(capturedPayload, &event); err != nil {
			t.Fatalf("failed to unmarshal wrapped event: %v", err)
		}
		if event.Type != "api.trigger" {
			t.Errorf("expected type api.trigger, got %q", event.Type)
		}
		if len(event.Payload) != 0 {
			t.Errorf("expected empty payload, got %v", event.Payload)
		}
	})
}

type streamWriter struct {
	mu     sync.Mutex
	header http.Header
	status int
	buf    bytes.Buffer
}

func newStreamWriter() *streamWriter {
	return &streamWriter{header: make(http.Header)}
}

func (w *streamWriter) Header() http.Header { return w.header }

func (w *streamWriter) WriteHeader(statusCode int) {
	w.mu.Lock()
	w.status = statusCode
	w.mu.Unlock()
}

func (w *streamWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Write(p)
}

func (w *streamWriter) Flush() {}

func (w *streamWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

func (w *streamWriter) Status() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.status == 0 {
		return http.StatusOK
	}
	return w.status
}

func TestHandleEvents_Unauthorized(t *testing.T) {
	q := &mockQueue{}
	reg := &mockRegistry{plugins: map[string]*plugin.Plugin{}}
	server := newTestServer(q, reg)

	req := httptest.NewRequest(http.MethodGet, "/events", nil)
	rr := httptest.NewRecorder()
	router := server.setupRoutes()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", rr.Code)
	}
}

func TestHandleEvents_ReplaysBufferedEvents(t *testing.T) {
	q := &mockQueue{}
	reg := &mockRegistry{plugins: map[string]*plugin.Plugin{}}
	server := newTestServer(q, reg)
	server.events.Publish("test_event", map[string]any{"k": "v"})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, "/events", nil).WithContext(ctx)
	req.Header.Set("Authorization", "Bearer test-key-123")

	w := newStreamWriter()
	router := server.setupRoutes()

	done := make(chan struct{})
	go func() {
		router.ServeHTTP(w, req)
		close(done)
	}()

	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(w.String(), "event: test_event\n") {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !strings.Contains(w.String(), "event: test_event\n") {
		t.Fatalf("expected SSE event in stream, got: %q", w.String())
	}

	cancel()
	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatalf("stream did not exit after context cancel")
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
				Name: "echo",
				Commands: plugin.Commands{
					{Name: "poll", Type: plugin.CommandTypeWrite},
				},
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
