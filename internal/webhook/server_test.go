package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/mattjoyce/senechal-gw/internal/queue"
)

// mockQueue is a mock implementation of JobQueuer for testing.
type mockQueue struct {
	enqueueFn func(ctx context.Context, req queue.EnqueueRequest) (string, error)
}

func (m *mockQueue) Enqueue(ctx context.Context, req queue.EnqueueRequest) (string, error) {
	if m.enqueueFn != nil {
		return m.enqueueFn(ctx, req)
	}
	return "test-job-id", nil
}

func TestHandleWebhook_ValidSignature(t *testing.T) {
	secret := "test-secret"
	body := []byte(`{"event":"push"}`)
	signature := formatGitHubSignature(computeExpectedSignature(body, secret))

	config := Config{
		Listen: "127.0.0.1:0",
		Endpoints: []EndpointConfig{
			{
				Path:            "/webhook/github",
				Plugin:          "github-handler",
				Command:         "handle",
				Secret:          secret,
				SignatureHeader: "X-Hub-Signature-256",
				MaxBodySize:     1048576,
			},
		},
	}

	mq := &mockQueue{
		enqueueFn: func(ctx context.Context, req queue.EnqueueRequest) (string, error) {
			// Verify job enqueued correctly
			if req.Plugin != "github-handler" {
				t.Errorf("Plugin = %v, want github-handler", req.Plugin)
			}
			if req.Command != "handle" {
				t.Errorf("Command = %v, want handle", req.Command)
			}
			if string(req.Payload) != string(body) {
				t.Errorf("Payload = %v, want %v", string(req.Payload), string(body))
			}
			return "job-123", nil
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	server := New(config, mq, logger)

	req := httptest.NewRequest("POST", "/webhook/github", bytes.NewReader(body))
	req.Header.Set("X-Hub-Signature-256", signature)
	rec := httptest.NewRecorder()

	server.handleWebhook(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusAccepted)
	}

	var resp TriggerResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.JobID != "job-123" {
		t.Errorf("JobID = %v, want job-123", resp.JobID)
	}
}

func TestHandleWebhook_InvalidSignature(t *testing.T) {
	secret := "test-secret"
	body := []byte(`{"event":"push"}`)
	wrongSignature := "sha256=0000000000000000000000000000000000000000000000000000000000000000"

	config := Config{
		Listen: "127.0.0.1:0",
		Endpoints: []EndpointConfig{
			{
				Path:            "/webhook/github",
				Plugin:          "github-handler",
				Command:         "handle",
				Secret:          secret,
				SignatureHeader: "X-Hub-Signature-256",
			},
		},
	}

	mq := &mockQueue{
		enqueueFn: func(ctx context.Context, req queue.EnqueueRequest) (string, error) {
			t.Fatal("Enqueue should not be called with invalid signature")
			return "", nil
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	server := New(config, mq, logger)

	req := httptest.NewRequest("POST", "/webhook/github", bytes.NewReader(body))
	req.Header.Set("X-Hub-Signature-256", wrongSignature)
	rec := httptest.NewRecorder()

	server.handleWebhook(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}

	var resp ErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Error should be generic (no details leaked)
	if resp.Error != "forbidden" {
		t.Errorf("Error = %v, want generic 'forbidden'", resp.Error)
	}
}

func TestHandleWebhook_MissingSignature(t *testing.T) {
	config := Config{
		Listen: "127.0.0.1:0",
		Endpoints: []EndpointConfig{
			{
				Path:            "/webhook/github",
				Plugin:          "github-handler",
				Secret:          "secret",
				SignatureHeader: "X-Hub-Signature-256",
			},
		},
	}

	mq := &mockQueue{
		enqueueFn: func(ctx context.Context, req queue.EnqueueRequest) (string, error) {
			t.Fatal("Enqueue should not be called without signature")
			return "", nil
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	server := New(config, mq, logger)

	req := httptest.NewRequest("POST", "/webhook/github", strings.NewReader(`{}`))
	// No signature header set
	rec := httptest.NewRecorder()

	server.handleWebhook(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestHandleWebhook_BodyTooLarge(t *testing.T) {
	secret := "test-secret"
	body := bytes.Repeat([]byte("a"), 2*1024*1024) // 2MB
	signature := formatGitHubSignature(computeExpectedSignature(body, secret))

	config := Config{
		Listen: "127.0.0.1:0",
		Endpoints: []EndpointConfig{
			{
				Path:            "/webhook/github",
				Plugin:          "github-handler",
				Secret:          secret,
				SignatureHeader: "X-Hub-Signature-256",
				MaxBodySize:     1048576, // 1MB limit
			},
		},
	}

	mq := &mockQueue{}
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	server := New(config, mq, logger)

	req := httptest.NewRequest("POST", "/webhook/github", bytes.NewReader(body))
	req.Header.Set("X-Hub-Signature-256", signature)
	rec := httptest.NewRecorder()

	server.handleWebhook(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusRequestEntityTooLarge)
	}
}

func TestHandleWebhook_UnknownPath(t *testing.T) {
	config := Config{
		Listen: "127.0.0.1:0",
		Endpoints: []EndpointConfig{
			{
				Path:   "/webhook/github",
				Plugin: "github-handler",
				Secret: "secret",
			},
		},
	}

	mq := &mockQueue{}
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	server := New(config, mq, logger)

	req := httptest.NewRequest("POST", "/webhook/unknown", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()

	server.handleWebhook(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestNew_AppliesDefaults(t *testing.T) {
	config := Config{
		Listen: "127.0.0.1:0",
		Endpoints: []EndpointConfig{
			{
				Path:   "/webhook/test",
				Plugin: "test-plugin",
				Secret: "secret",
				// MaxBodySize and Command not set - should get defaults
			},
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	server := New(config, &mockQueue{}, logger)

	ep := server.endpoints["/webhook/test"]
	if ep.MaxBodySize != DefaultMaxBodySize {
		t.Errorf("MaxBodySize = %d, want %d", ep.MaxBodySize, DefaultMaxBodySize)
	}
	if ep.Command != DefaultCommand {
		t.Errorf("Command = %v, want %v", ep.Command, DefaultCommand)
	}
}
