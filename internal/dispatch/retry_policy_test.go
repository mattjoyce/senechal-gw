package dispatch

import (
	"testing"

	"github.com/mattjoyce/ductile/internal/config"
	"github.com/mattjoyce/ductile/internal/protocol"
	"github.com/mattjoyce/ductile/internal/queue"
)

func TestDecideRetryPolicy(t *testing.T) {
	retryTrue := true
	retryFalse := false

	tests := []struct {
		name          string
		resp          *protocol.Response
		compat        protocol.ResponseCompat
		exitCode      int
		job           *queue.Job
		defaultReason string
		wantRetryable bool
		wantReason    string
		wantExhausted bool
	}{
		{
			name: "default plugin error retries when attempts remain",
			resp: &protocol.Response{
				Status: "error",
				Error:  "temporary",
			},
			job:           &queue.Job{Attempt: 1, MaxAttempts: 3},
			defaultReason: retryReasonPluginError,
			wantRetryable: true,
			wantReason:    retryReasonPluginError,
		},
		{
			name: "v2 retry false remains non retryable compatibility signal",
			resp: &protocol.Response{
				Status: "error",
				Error:  "bad request",
			},
			compat:        protocol.ResponseCompat{Retry: &retryFalse},
			job:           &queue.Job{Attempt: 1, MaxAttempts: 3},
			defaultReason: retryReasonPluginError,
			wantRetryable: false,
			wantReason:    retryReasonPluginRetryFalse,
		},
		{
			name: "exit code 78 wins over plugin retry true",
			resp: &protocol.Response{
				Status: "error",
				Error:  "invalid config",
			},
			compat:        protocol.ResponseCompat{Retry: &retryTrue},
			exitCode:      nonRetryableExitCode,
			job:           &queue.Job{Attempt: 1, MaxAttempts: 3},
			defaultReason: retryReasonPluginError,
			wantRetryable: false,
			wantReason:    retryReasonExitCode78,
		},
		{
			name: "attempts exhausted remains visible on otherwise retryable error",
			resp: &protocol.Response{
				Status: "error",
				Error:  "temporary",
			},
			job:           &queue.Job{Attempt: 3, MaxAttempts: 3},
			defaultReason: retryReasonPluginError,
			wantRetryable: true,
			wantReason:    retryReasonPluginError,
			wantExhausted: true,
		},
		{
			name:          "spawn errors retry by default",
			job:           &queue.Job{Attempt: 1, MaxAttempts: 3},
			defaultReason: retryReasonSpawnError,
			wantRetryable: true,
			wantReason:    retryReasonSpawnError,
		},
		{
			name:          "timeout retries by default",
			job:           &queue.Job{Attempt: 1, MaxAttempts: 3},
			defaultReason: retryReasonTimeout,
			wantRetryable: true,
			wantReason:    retryReasonTimeout,
		},
		{
			name:          "config max attempts is fallback when job max attempts is absent",
			job:           &queue.Job{Attempt: 2},
			defaultReason: retryReasonPluginError,
			wantRetryable: true,
			wantReason:    retryReasonPluginError,
			wantExhausted: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := decideRetryPolicy(tt.resp, tt.compat, tt.exitCode, tt.job, config.PluginConf{
				Retry: &config.RetryConfig{MaxAttempts: 2},
			}, tt.defaultReason)
			if got.Retryable != tt.wantRetryable {
				t.Fatalf("Retryable = %v, want %v", got.Retryable, tt.wantRetryable)
			}
			if got.Reason != tt.wantReason {
				t.Fatalf("Reason = %q, want %q", got.Reason, tt.wantReason)
			}
			if got.AttemptsExhausted != tt.wantExhausted {
				t.Fatalf("AttemptsExhausted = %v, want %v", got.AttemptsExhausted, tt.wantExhausted)
			}
		})
	}
}
