//go:build darwin || linux || freebsd || openbsd || netbsd

package dispatch

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/mattjoyce/ductile/internal/protocol"
)

func TestSubprocessExecutorReturnsMalformedProtocolResponse(t *testing.T) {
	t.Parallel()

	scriptPath := writeDispatchTestScript(t, `#!/bin/sh
echo '{not-json'
`)

	executor := newSubprocessExecutor(nil)
	req := &protocol.Request{Protocol: 2, JobID: "job-malformed", Command: "poll"}
	resp, _, rawResp, rawStdout, _, _, err := executor.execute(context.Background(), "malformed", scriptPath, req, 5*time.Second, slog.Default())
	if err == nil {
		t.Fatal("execute error = nil, want decode response error")
	}
	if !strings.Contains(err.Error(), "decode response") {
		t.Fatalf("execute error = %v, want decode response error", err)
	}
	if resp != nil {
		t.Fatalf("response = %#v, want nil", resp)
	}
	if got := string(rawResp); got != "{not-json\n" {
		t.Fatalf("raw response = %q, want malformed stdout", got)
	}
	if got := string(rawStdout); got != "{not-json\n" {
		t.Fatalf("raw stdout = %q, want malformed stdout", got)
	}
}
