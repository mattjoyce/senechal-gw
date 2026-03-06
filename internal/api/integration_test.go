package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/mattjoyce/ductile/internal/api"
	"github.com/mattjoyce/ductile/internal/auth"
	"github.com/mattjoyce/ductile/internal/events"
	"github.com/mattjoyce/ductile/internal/plugin"
	"github.com/mattjoyce/ductile/internal/queue"
	"github.com/mattjoyce/ductile/internal/router"
	"github.com/mattjoyce/ductile/internal/router/dsl"
	"github.com/mattjoyce/ductile/internal/state"
	"github.com/mattjoyce/ductile/internal/storage"
)

type queueBackedWaiter struct {
	q *queue.Queue
}

func (w *queueBackedWaiter) WaitForJobTree(ctx context.Context, rootJobID string, timeout time.Duration) ([]*queue.JobResult, error) {
	return w.q.GetJobTree(ctx, rootJobID)
}

func allocateLocalAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("allocate local addr: %v", err)
	}
	addr := ln.Addr().String()
	if err := ln.Close(); err != nil {
		t.Fatalf("close temp listener: %v", err)
	}
	return addr
}

// TestAPIIntegration tests the API flow with real queue, router, waiter, and context store.
func TestAPIIntegration(t *testing.T) {
	ctx := context.Background()
	db, err := storage.OpenSQLite(ctx, ":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer func() { _ = db.Close() }()

	q := queue.New(db)
	ctxStore := state.NewContextStore(db)

	registry := plugin.NewRegistry()
	if err := registry.Add(&plugin.Plugin{
		Name: "echo",
		Commands: plugin.Commands{
			{Name: "poll", Type: plugin.CommandTypeWrite},
			{Name: "handle", Type: plugin.CommandTypeWrite},
		},
	}); err != nil {
		t.Fatalf("registry.Add: %v", err)
	}

	r := router.New(&dsl.Set{
		Pipelines: map[string]*dsl.Pipeline{
			"test-pipeline": {
				Name:          "test-pipeline",
				Trigger:       "test.trigger",
				ExecutionMode: "asynchronous",
				Nodes: map[string]dsl.Node{
					"entry": {ID: "entry", Kind: dsl.NodeKindUses, Uses: "echo"},
				},
				EntryNodeIDs:    []string{"entry"},
				TerminalNodeIDs: []string{"entry"},
			},
		},
	}, slog.Default())

	listenAddr := allocateLocalAddr(t)
	server := api.New(api.Config{
		Listen: listenAddr,
		Tokens: []auth.TokenConfig{{Token: "test-key-123", Scopes: []string{"*"}}},
	}, q, registry, r, &queueBackedWaiter{q: q}, ctxStore, events.NewHub(10), slog.Default())

	serverCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	serverReady := make(chan error, 1)
	go func() {
		if err := server.Start(serverCtx); err != nil && err != context.Canceled {
			serverReady <- err
		}
	}()

	time.Sleep(100 * time.Millisecond)
	select {
	case err := <-serverReady:
		t.Fatalf("server failed to start: %v", err)
	default:
	}

	client := &http.Client{Timeout: 5 * time.Second}
	baseURL := fmt.Sprintf("http://%s", listenAddr)

	t.Run("direct plugin trigger enqueues real job", func(t *testing.T) {
		triggerBody := []byte(`{"payload": {"test": "data"}}`)
		req, _ := http.NewRequest(http.MethodPost, baseURL+"/plugin/echo/poll", bytes.NewReader(triggerBody))
		req.Header.Set("Authorization", "Bearer test-key-123")
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("failed to trigger job: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusAccepted {
			t.Fatalf("expected status 202, got %d", resp.StatusCode)
		}

		var triggerResp api.TriggerResponse
		if err := json.NewDecoder(resp.Body).Decode(&triggerResp); err != nil {
			t.Fatalf("failed to decode trigger response: %v", err)
		}
		if triggerResp.JobID == "" {
			t.Fatal("expected non-empty job_id")
		}

		req, _ = http.NewRequest(http.MethodGet, baseURL+"/job/"+triggerResp.JobID, nil)
		req.Header.Set("Authorization", "Bearer test-key-123")
		resp, err = client.Do(req)
		if err != nil {
			t.Fatalf("failed to get job: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		var jobResp api.JobStatusResponse
		if err := json.NewDecoder(resp.Body).Decode(&jobResp); err != nil {
			t.Fatalf("failed to decode job response: %v", err)
		}
		if jobResp.JobID != triggerResp.JobID || jobResp.Plugin != "echo" || jobResp.Command != "poll" {
			t.Fatalf("unexpected job response: %+v", jobResp)
		}
	})

	t.Run("pipeline trigger uses real router and context store", func(t *testing.T) {
		triggerBody := []byte(`{"payload": {"test": "pipeline"}}`)
		req, _ := http.NewRequest(http.MethodPost, baseURL+"/pipeline/test-pipeline", bytes.NewReader(triggerBody))
		req.Header.Set("Authorization", "Bearer test-key-123")
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("failed to trigger pipeline: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusAccepted {
			t.Fatalf("expected status 202, got %d", resp.StatusCode)
		}

		var triggerResp api.TriggerResponse
		if err := json.NewDecoder(resp.Body).Decode(&triggerResp); err != nil {
			t.Fatalf("failed to decode trigger response: %v", err)
		}

		job, err := q.GetJobByID(ctx, triggerResp.JobID)
		if err != nil {
			t.Fatalf("GetJobByID: %v", err)
		}
		if job.Plugin != "echo" || job.Command != "handle" {
			t.Fatalf("unexpected pipeline job: %+v", job)
		}

		rows, err := db.Query("SELECT event_context_id FROM job_queue WHERE id = ?", triggerResp.JobID)
		if err != nil {
			t.Fatalf("query event_context_id: %v", err)
		}
		var contextID string
		if !rows.Next() {
			_ = rows.Close()
			t.Fatal("expected queued pipeline job row")
		}
		if err := rows.Scan(&contextID); err != nil {
			_ = rows.Close()
			t.Fatalf("scan event_context_id: %v", err)
		}
		if err := rows.Close(); err != nil {
			t.Fatalf("close rows: %v", err)
		}
		if contextID == "" {
			t.Fatal("expected non-empty event_context_id")
		}

		eventCtx, err := ctxStore.Get(ctx, contextID)
		if err != nil {
			t.Fatalf("context store get: %v", err)
		}
		if eventCtx.PipelineName != "test-pipeline" || eventCtx.StepID != "entry" {
			t.Fatalf("unexpected event context: %+v", eventCtx)
		}
	})

	t.Run("unauthorized request is rejected", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodPost, baseURL+"/plugin/echo/poll", nil)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("failed to make unauthorized request: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("expected status 401, got %d", resp.StatusCode)
		}
	})

	cancel()
	time.Sleep(100 * time.Millisecond)
}
