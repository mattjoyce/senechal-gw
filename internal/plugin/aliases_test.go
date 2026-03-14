package plugin

import (
	"testing"

	"github.com/mattjoyce/ductile/internal/config"
)

func TestApplyAliasesRegistersInstance(t *testing.T) {
	reg := NewRegistry()
	base := &Plugin{Name: "switch", Entrypoint: "/tmp/switch/run.py", Protocol: 2}
	if err := reg.Add(base); err != nil {
		t.Fatalf("add base: %v", err)
	}

	aliases, err := ApplyAliases(reg, map[string]config.PluginConf{
		"check_youtube": {Enabled: true, Uses: "switch"},
	})
	if err != nil {
		t.Fatalf("ApplyAliases: %v", err)
	}
	if len(aliases) != 1 {
		t.Fatalf("expected 1 alias, got %d", len(aliases))
	}
	alias, ok := reg.Get("check_youtube")
	if !ok {
		t.Fatalf("expected alias in registry")
	}
	if alias.Entrypoint != base.Entrypoint {
		t.Fatalf("expected alias entrypoint %q, got %q", base.Entrypoint, alias.Entrypoint)
	}
}

func TestApplyAliasesMissingBase(t *testing.T) {
	reg := NewRegistry()
	if _, err := ApplyAliases(reg, map[string]config.PluginConf{
		"check_youtube": {Enabled: true, Uses: "switch"},
	}); err == nil {
		t.Fatal("expected error for missing base plugin")
	}
}

func TestApplyAliasesConflictingName(t *testing.T) {
	reg := NewRegistry()
	if err := reg.Add(&Plugin{Name: "switch"}); err != nil {
		t.Fatalf("add base: %v", err)
	}
	if err := reg.Add(&Plugin{Name: "check_youtube"}); err != nil {
		t.Fatalf("add existing: %v", err)
	}
	if _, err := ApplyAliases(reg, map[string]config.PluginConf{
		"check_youtube": {Enabled: true, Uses: "switch"},
	}); err == nil {
		t.Fatal("expected error for alias name conflict")
	}
}
