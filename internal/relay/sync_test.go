package relay

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mattjoyce/ductile/internal/config"
	"github.com/mattjoyce/ductile/internal/queue"
	"github.com/mattjoyce/ductile/internal/state"
)

// fakeWaiter stands in for the dispatcher. It records calls, can block until
// released (to exercise the relay-dedicated concurrency budget), and can
// simulate a wait timeout.
type fakeWaiter struct {
	mu      sync.Mutex
	calls   int
	entered chan string // receives rootJobID when a wait begins
	release chan struct{}
	timeout bool
	result  []*queue.JobResult
}

func (f *fakeWaiter) WaitForJobTree(ctx context.Context, rootJobID string, timeout time.Duration) ([]*queue.JobResult, error) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	if f.entered != nil {
		f.entered <- rootJobID
	}
	if f.timeout {
		return nil, fmt.Errorf("timeout waiting for job tree completion")
	}
	if f.release != nil {
		select {
		case <-f.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(timeout):
			return nil, fmt.Errorf("timeout waiting for job tree completion")
		}
	}
	out := make([]*queue.JobResult, 0, len(f.result))
	for _, r := range f.result {
		cp := *r
		if cp.JobID == "" {
			cp.JobID = rootJobID
		}
		out = append(out, &cp)
	}
	return out, nil
}

func syncReceiverConfig(allowSync, syncEnabled bool, maxConcurrent int) *config.Config {
	return &config.Config{
		Service: config.ServiceConfig{Name: "lab"},
		RemoteIngress: &config.RemoteIngressConfig{
			ListenPath:       "/ingest/peer",
			AllowedClockSkew: 5 * time.Minute,
			TrustedPeers: []config.RelayPeerConfig{
				{Name: "home-primary", Enabled: true, SecretRef: "relay-lab-v1", Accept: []string{"backup.ready"}, AllowSync: allowSync},
			},
			Sync: &config.RelayIngressSyncConfig{Enabled: syncEnabled, MaxTimeout: 2 * time.Second, MaxConcurrent: maxConcurrent},
		},
		Tokens: []config.TokenEntry{{Name: "relay-lab-v1", Key: "shared-secret"}},
	}
}

func sendSyncRelay(t *testing.T, serverURL string, env Envelope) *http.Response {
	t.Helper()
	body, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	timestamp := time.Now().UTC().Format(time.RFC3339Nano)
	signature, err := sign(http.MethodPost, "/ingest/peer/home-primary", timestamp, body, "shared-secret")
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, serverURL+"/ingest/peer/home-primary", strings.NewReader(string(body)))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(HeaderPeer, "home-primary")
	req.Header.Set(HeaderTimestamp, timestamp)
	req.Header.Set(HeaderSignature, signature)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	return resp
}

func syncEnvelope(timeout string) Envelope {
	return Envelope{
		Event:  EnvelopeEvent{Type: "backup.ready", Payload: map[string]any{"path": "/srv/x"}, DedupeKey: "backup.ready:sync-1"},
		Origin: EnvelopeOrigin{Instance: "home-primary", EventID: "evt-sync-1"},
		Reply:  &EnvelopeReply{Mode: SyncReplyMode, Timeout: timeout},
	}
}

func newSyncReceiver(t *testing.T, cfg *config.Config, waiter TreeWaiter) *Receiver {
	t.Helper()
	db := setupRelayDB(t)
	q := queue.New(db)
	contexts := state.NewContextStore(db)
	engine := setupRelayRouter(t, `
pipelines:
  - name: process-offsite-backup
    on: backup.ready
    steps:
      - id: verify-backup
        uses: echo
`)
	receiver, err := NewReceiver(cfg, q, engine, contexts, waiter, slog.Default())
	if err != nil {
		t.Fatalf("NewReceiver: %v", err)
	}
	return receiver
}

func TestSyncReplySuccessReturnsAggregatedResult(t *testing.T) {
	waiter := &fakeWaiter{result: []*queue.JobResult{
		{StepID: "verify-backup", Status: queue.StatusSucceeded, Plugin: "echo", Command: "handle", Result: json.RawMessage(`{"verified":true}`)},
	}}
	receiver := newSyncReceiver(t, syncReceiverConfig(true, true, 4), waiter)
	server := setupRelayServer(t, receiver)
	defer server.Close()

	resp := sendSyncRelay(t, server.URL, syncEnvelope("1s"))
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 200; body=%s", resp.StatusCode, raw)
	}
	var got AcceptanceResponse
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got.Status != string(queue.StatusSucceeded) {
		t.Fatalf("Status = %q, want succeeded", got.Status)
	}
	if string(got.Result) != `{"verified":true}` {
		t.Fatalf("Result = %s, want terminal step result", got.Result)
	}
	if len(got.Tree) != 1 || got.Tree[0].Plugin != "echo" {
		t.Fatalf("Tree = %+v, want one echo node", got.Tree)
	}
	if waiter.calls != 1 {
		t.Fatalf("waiter.calls = %d, want 1", waiter.calls)
	}
}

func TestSyncReplyTimeoutReturns202StillRunning(t *testing.T) {
	receiver := newSyncReceiver(t, syncReceiverConfig(true, true, 4), &fakeWaiter{timeout: true})
	server := setupRelayServer(t, receiver)
	defer server.Close()

	resp := sendSyncRelay(t, server.URL, syncEnvelope("1s"))
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", resp.StatusCode)
	}
	var got AcceptanceResponse
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if !got.TimedOut || got.Status != "running" {
		t.Fatalf("got %+v, want TimedOut running", got)
	}
	if got.JobID == "" {
		t.Fatal("expected job id on timeout so the sender can reconcile")
	}
}

func TestSyncReplyRefusedWhenPolicyForbids(t *testing.T) {
	cases := []struct {
		name        string
		allowSync   bool
		syncEnabled bool
	}{
		{"peer-not-allowed", false, true},
		{"sync-disabled", true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			receiver := newSyncReceiver(t, syncReceiverConfig(tc.allowSync, tc.syncEnabled, 4), &fakeWaiter{})
			server := setupRelayServer(t, receiver)
			defer server.Close()

			resp := sendSyncRelay(t, server.URL, syncEnvelope("1s"))
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d, want 422", resp.StatusCode)
			}
		})
	}
}

func TestSyncReplyUnavailableWithoutWaiter(t *testing.T) {
	receiver := newSyncReceiver(t, syncReceiverConfig(true, true, 4), nil)
	server := setupRelayServer(t, receiver)
	defer server.Close()

	resp := sendSyncRelay(t, server.URL, syncEnvelope("1s"))
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", resp.StatusCode)
	}
}

// A slow remote peer must not be able to consume more than the relay-dedicated
// budget. With MaxConcurrent=1 a second concurrent sync request is rejected
// while the first is still waiting — local API sync callers are unaffected
// because that semaphore is entirely separate.
func TestSyncReplyConcurrencyIsolation(t *testing.T) {
	waiter := &fakeWaiter{
		entered: make(chan string, 1),
		release: make(chan struct{}),
		result:  []*queue.JobResult{{StepID: "verify-backup", Status: queue.StatusSucceeded, Plugin: "echo"}},
	}
	receiver := newSyncReceiver(t, syncReceiverConfig(true, true, 1), waiter)
	server := setupRelayServer(t, receiver)
	defer server.Close()

	firstDone := make(chan int, 1)
	go func() {
		resp := sendSyncRelay(t, server.URL, syncEnvelope("2s"))
		firstDone <- resp.StatusCode
		_ = resp.Body.Close()
	}()

	select {
	case <-waiter.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("first sync request never entered the waiter")
	}

	resp := sendSyncRelay(t, server.URL, syncEnvelope("2s"))
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("second request status = %d, want 503 (budget exhausted)", resp.StatusCode)
	}
	_ = resp.Body.Close()

	close(waiter.release)
	if code := <-firstDone; code != http.StatusOK {
		t.Fatalf("first request status = %d, want 200", code)
	}
}
