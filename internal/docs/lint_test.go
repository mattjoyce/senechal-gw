// Package docs holds documentation lint tests. The package itself has no
// runtime callers; it exists so `go test ./...` exercises the doc-smoke
// regression check that protects the post-Sprint-14 facts-first language.
package docs

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// forbiddenPhrases catches state-first language that Sprint 14 removed from
// docs, agent guidance, and the operator skill. Every phrase here is the
// exact wording (case-insensitive substring) that previously appeared in the
// canonical surfaces. Lines inside ```historical-note ... ``` blocks are
// exempt so prior context can be preserved deliberately.
var forbiddenPhrases = []string{
	"plugin_state is the primary",
	"durable state lives in plugin_state",
	"plugin state is the primary",
	"shallow-merged into the plugin's persistent state",
	"single json blob per plugin",
}

// scanRoots is the set of doc-bearing paths Sprint 14 reframed. These are
// resolved relative to the repo root.
var scanRoots = []string{
	"docs",
	"CLAUDE.md",
	"skills/ductile",
}

func TestNoForbiddenStateFirstPhrases(t *testing.T) {
	repoRoot := repoRoot(t)

	for _, root := range scanRoots {
		root := filepath.Join(repoRoot, root)
		walk(t, root)
	}
}

func walk(t *testing.T, root string) {
	t.Helper()
	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		t.Fatalf("stat %s: %v", root, err)
	}
	if !info.IsDir() {
		checkFile(t, root)
		return
	}
	if err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		switch filepath.Ext(path) {
		case ".md", ".html", ".txt":
			checkFile(t, path)
		}
		return nil
	}); err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
}

func checkFile(t *testing.T, path string) {
	t.Helper()
	raw, err := os.ReadFile(path) // #nosec G304 -- test scans only repo files we control
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	lines := strings.Split(string(raw), "\n")
	inHistorical := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```historical-note") {
			inHistorical = true
			continue
		}
		if inHistorical && trimmed == "```" {
			inHistorical = false
			continue
		}
		if inHistorical {
			continue
		}
		lower := strings.ToLower(line)
		for _, phrase := range forbiddenPhrases {
			if strings.Contains(lower, phrase) {
				t.Errorf("%s:%d forbidden phrase %q found: %s", path, i+1, phrase, line)
			}
		}
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime.Caller failed")
	}
	// internal/docs/lint_test.go -> ../../
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}
