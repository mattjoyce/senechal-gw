package main

import (
	"log/slog"
	"os"
	"runtime"
)

// Log message constants for tcc-prewarm. Stable strings — referenced by tests
// and may be keyed off by external log scrapers.
const (
	tccPrewarmAccessibleMsg = "tcc-prewarm: path accessible"
	tccPrewarmFailedMsg     = "tcc-prewarm: path access failed"
)

// runTCCPrewarm stat()s each path to surface macOS TCC permission popups
// synchronously at gateway cold-start.
//
// macOS indexes Files-and-Folders TCC grants by designated code requirement.
// An adhoc-signed binary's cdhash changes on every rebuild, which invalidates
// existing grants for protected categories (Documents, NetworkVolumes, etc.).
// The first child-process access of a protected path on the new binary
// triggers a TCC popup that blocks the calling subprocess.
//
// By stat()-ing each configured path here, the popup appears at start-up
// while the operator is at the keyboard for the deploy, rather than at an
// arbitrary later moment when an unattended job hits the protected path
// and times out waiting for a click.
//
// Each path is stat()-ed sequentially — parallelising would stack multiple
// TCC dialogs at once, which is confusing for the operator. Configure only
// local-volume paths; an unreachable network mount blocks os.Stat for the
// filesystem-level timeout and delays gateway readiness.
//
// No-op on non-darwin and when paths is empty. Never returns an error: a
// path that fails to stat is logged at Warn and skipped. Intended to be
// called only on cold start (after PID lock acquired); SIGHUP reloads do
// not change the binary's cdhash and therefore do not invalidate existing
// grants.
func runTCCPrewarm(paths []string, logger *slog.Logger) {
	if runtime.GOOS != "darwin" {
		return
	}
	for _, p := range paths {
		if _, err := os.Stat(p); err != nil {
			logger.Warn(tccPrewarmFailedMsg, "path", p, slog.Any("error", err))
			continue
		}
		logger.Info(tccPrewarmAccessibleMsg, "path", p)
	}
}
