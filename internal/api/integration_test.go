package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/mattjoyce/ductile/internal/api"
	"github.com/mattjoyce/ductile/internal/events"
	"github.com/mattjoyce/ductile/internal/plugin"
	"github.com/mattjoyce/ductile/internal/queue"
	"github.com/mattjoyce/ductile/internal/router"
	"github.com/mattjoyce/ductile/internal/storage"
)

type mockRouter struct{}

func (m *mockRouter) GetPipelineByTrigger(trigger string) *router.PipelineInfo { return nil }
func (m *mockRouter) GetPipelineByName(name string) *router.PipelineInfo       { return nil }
func (m *mockRouter) Next(ctx context.Context, req router.Request) ([]router.Dispatch, error) {
	return nil, nil
}

type mockWaiter struct{}

func (m *mockWaiter) WaitForJobTree(ctx context.Context, rootJobID string, timeout time.Duration) ([]*queue.JobResult, error) {
	return nil, nil
}

// TestAPIIntegration tests the full API flow with real queue
func TestAPIIntegration(t *testing.T) {
	// Setup in-memory database
	ctx := context.Background()
	db, err := storage.OpenSQLite(ctx, ":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	// Create queue
	q := queue.New(db)

	// Create mock plugin registry
	registry := plugin.NewRegistry()
	registry.Add(&plugin.Plugin{
		Name: "echo",
		Commands: plugin.Commands{
			{Name: "poll", Type: plugin.CommandTypeWrite},
			{Name: "handle", Type: plugin.CommandTypeWrite},
		},
	})

	// Create API server
	testPort := "localhost:18080"
	config := api.Config{
		Listen: testPort,
		APIKey: "test-key-123",
	}
	hub := events.NewHub(10)
	server := api.New(config, q, registry, &mockRouter{}, &mockWaiter{}, nil, hub, slog.Default())

	// Start server in background
	serverCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	serverReady := make(chan error, 1)
	go func() {
		if err := server.Start(serverCtx); err != nil && err != context.Canceled {
			serverReady <- err
		}
	}()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	select {
	case err := <-serverReady:
		t.Fatalf("server failed to start: %v", err)
	default:
		// Server started successfully
	}

	// Test 1: Trigger a job
	triggerBody := []byte(`{"payload": {"test": "data"}}`)
	baseURL := "http://" + testPort
	req, _ := http.NewRequest(http.MethodPost, baseURL+"/trigger/echo/poll", bytes.NewReader(triggerBody))
	req.Header.Set("Authorization", "Bearer test-key-123")
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("failed to trigger job: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		t.Errorf("expected status 202, got %d", resp.StatusCode)
	}

	var triggerResp api.TriggerResponse
	if err := json.NewDecoder(resp.Body).Decode(&triggerResp); err != nil {
		t.Fatalf("failed to decode trigger response: %v", err)
	}

	if triggerResp.JobID == "" {
		t.Error("expected non-empty job_id")
	}
	if triggerResp.Status != "queued" {
		t.Errorf("expected status queued, got %s", triggerResp.Status)
	}

	jobID := triggerResp.JobID

	// Test 2: Get job status
	req, _ = http.NewRequest(http.MethodGet, baseURL+"/job/"+jobID, nil)
	req.Header.Set("Authorization", "Bearer test-key-123")

	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("failed to get job: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	var jobResp api.JobStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&jobResp); err != nil {
		t.Fatalf("failed to decode job response: %v", err)
	}

	if jobResp.JobID != jobID {
		t.Errorf("expected job_id %s, got %s", jobID, jobResp.JobID)
	}
	if jobResp.Status != "queued" {
		t.Errorf("expected status queued, got %s", jobResp.Status)
	}
	if jobResp.Plugin != "echo" {
		t.Errorf("expected plugin echo, got %s", jobResp.Plugin)
	}
	if jobResp.Command != "poll" {
		t.Errorf("expected command poll, got %s", jobResp.Command)
	}

	// Test 3: Unauthorized request
	req, _ = http.NewRequest(http.MethodPost, baseURL+"/trigger/echo/poll", nil)
	// No Authorization header

	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("failed to make unauthorized request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", resp.StatusCode)
	}

	// Shutdown server
	cancel()
	time.Sleep(100 * time.Millisecond)
}
