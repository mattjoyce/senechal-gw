package main

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func newTestLogger(buf *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
}

func TestRunTCCPrewarm_NoopCases(t *testing.T) {
	cases := []struct {
		name  string
		paths []string
	}{
		{"nil paths", nil},
		{"empty paths", []string{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var buf bytes.Buffer
			runTCCPrewarm(c.paths, newTestLogger(&buf))
			if buf.Len() != 0 {
				t.Errorf("expected no log output, got: %s", buf.String())
			}
		})
	}
}

func TestRunTCCPrewarm_LogsAccessibleAndFailedPaths(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("TCC prewarm logging is darwin-only; non-darwin returns early before logging")
	}

	tmp := t.TempDir()
	existing := filepath.Join(tmp, "marker.txt")
	if err := os.WriteFile(existing, []byte{}, 0o644); err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	missing := filepath.Join(tmp, "does-not-exist")

	var buf bytes.Buffer
	runTCCPrewarm([]string{existing, missing}, newTestLogger(&buf))

	out := buf.String()
	if !strings.Contains(out, tccPrewarmAccessibleMsg) {
		t.Errorf("expected %q in log, got: %s", tccPrewarmAccessibleMsg, out)
	}
	if !strings.Contains(out, existing) {
		t.Errorf("expected accessible path %q in log, got: %s", existing, out)
	}
	if !strings.Contains(out, tccPrewarmFailedMsg) {
		t.Errorf("expected %q in log, got: %s", tccPrewarmFailedMsg, out)
	}
	if !strings.Contains(out, missing) {
		t.Errorf("expected missing path %q in log, got: %s", missing, out)
	}
}
