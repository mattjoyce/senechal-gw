//go:build darwin || linux || freebsd || openbsd || netbsd

package dispatch

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/mattjoyce/ductile/internal/protocol"
)

// gracefulPluginEntry runs from init() when this test binary is re-execed as a
// plugin (see writeGracefulPlugin). Installing the SIGTERM ignore in init()
// — before the testing framework parses flags — closes the startup race where
// SIGTERM could arrive before the handler is in place. It models a well-behaved
// plugin doing a clean shutdown: ignore SIGTERM, finish work within the grace
// period, emit a valid response, exit 0. A Go helper is used because a /bin/sh
// trap does not reliably survive SIGTERM on macOS.
func init() {
	if os.Getenv("DUCTILE_GRACEFUL_PLUGIN_HELPER") != "1" {
		return
	}
	signal.Ignore(syscall.SIGTERM)
	// Sleep past the parent's timeout (3s) but well inside the 5s grace
	// period so the process completes its work after the timeout fired.
	time.Sleep(3 * time.Second)
	fmt.Println(`{"status":"ok","result":"slow-but-done"}`)
	os.Exit(0)
}

// writeGracefulPlugin writes a shell shim that re-execs the test binary into
// TestSubprocessExecutorGracefulPluginHelper, yielding an OS-independent plugin
// that genuinely ignores SIGTERM.
func writeGracefulPlugin(t *testing.T) string {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatalf("locate test binary: %v", err)
	}
	path := filepath.Join(t.TempDir(), "graceful-plugin.sh")
	shim := fmt.Sprintf(
		"#!/bin/sh\nexec env DUCTILE_GRACEFUL_PLUGIN_HELPER=1 %q -test.run=^$ -test.v=false\n",
		self)
	if err := os.WriteFile(path, []byte(shim), 0o700); err != nil {
		t.Fatalf("write plugin shim: %v", err)
	}
	return path
}

// TestSubprocessExecutorPrefersCompletedResultOverTimeout reproduces C-FRO-5:
// a plugin that finishes its work successfully *within the SIGTERM grace
// period* must have its response delivered, not discarded as a timeout. The
// OPS-001 shape: completed work classified only after the timeout branch has
// already won.
func TestSubprocessExecutorPrefersCompletedResultOverTimeout(t *testing.T) {
	// Not parallel: re-execs this test binary as a child, whose startup
	// latency under -race must stay below the parent timeout. Suite-wide
	// parallel contention inflates that latency.

	// A well-behaved plugin: ignores SIGTERM, finishes its work within the
	// grace period, emits a valid response, exits 0. Its work completed —
	// the timeout branch must deliver that result, not discard it.
	scriptPath := writeGracefulPlugin(t)

	executor := newSubprocessExecutor(nil)
	req := &protocol.Request{Protocol: 2, JobID: "job-slow-done", Command: "poll"}
	resp, _, _, _, _, exitCode, err := executor.execute(
		context.Background(), "slow-done", scriptPath, req, 3*time.Second, slog.Default())
	if err != nil {
		t.Fatalf("execute error = %v, want nil (process completed successfully)", err)
	}
	if resp == nil {
		t.Fatal("response = nil, want completed response (result was discarded as timeout)")
	}
	if resp.Result != "slow-but-done" {
		t.Fatalf("response result = %q, want %q", resp.Result, "slow-but-done")
	}
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0", exitCode)
	}
}

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
