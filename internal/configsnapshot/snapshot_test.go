package configsnapshot

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mattjoyce/ductile/internal/config"
	"github.com/mattjoyce/ductile/internal/storage"
	"gopkg.in/yaml.v3"
)

func TestBuildRedactsSecretsAndHashesSecretChanges(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "config.yaml")
	if err := os.WriteFile(configPath, []byte("service:\n  name: ductile\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg := config.Defaults()
	cfg.SourceFiles = map[string]*yaml.Node{configPath: {}}
	cfg.API.Enabled = true
	cfg.API.Auth.Tokens = []config.APIToken{{Token: "api-secret-one", Scopes: []string{"job:rw"}}}
	cfg.Tokens = []config.TokenEntry{{Name: "github_webhook_secret", Key: "webhook-secret-one"}}
	cfg.Webhooks = &config.WebhooksConfig{
		Endpoints: []config.WebhookEndpoint{{Name: "github", Path: "/github", Plugin: "echo", SecretRef: "github_webhook_secret"}},
	}
	cfg.Plugins["echo"] = config.PluginConf{
		Enabled: true,
		Config:  map[string]any{"api_key": "plugin-secret-one", "public": "visible"},
	}

	first, err := Build(BuildInput{
		Config:         cfg,
		ConfigPath:     configPath,
		ConfigSource:   "explicit",
		Reason:         ReasonStartup,
		DuctileVersion: "test-version",
		BinaryPath:     "/tmp/ductile",
		LoadedAt:       time.Date(2026, 4, 18, 1, 2, 3, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("Build(first): %v", err)
	}
	if strings.Contains(string(first.SanitizedConfig), "api-secret-one") ||
		strings.Contains(string(first.SanitizedConfig), "webhook-secret-one") ||
		strings.Contains(string(first.SanitizedConfig), "plugin-secret-one") {
		t.Fatalf("sanitized config leaked secret material: %s", first.SanitizedConfig)
	}
	if !strings.Contains(string(first.SanitizedConfig), "[redacted:api.auth.tokens[0].token]") {
		t.Fatalf("sanitized config did not redact API token: %s", first.SanitizedConfig)
	}
	if !strings.HasPrefix(first.ConfigHash, "blake3:") {
		t.Fatalf("ConfigHash = %q", first.ConfigHash)
	}
	if first.SourceHash == nil || !strings.HasPrefix(*first.SourceHash, "blake3:") {
		t.Fatalf("SourceHash = %v", first.SourceHash)
	}

	var uses []SecretUse
	if err := json.Unmarshal(first.SecretFingerprints, &uses); err != nil {
		t.Fatalf("unmarshal secret uses: %v", err)
	}
	if len(uses) < 3 {
		t.Fatalf("expected at least 3 secret uses, got %+v", uses)
	}

	cfg.API.Auth.Tokens[0].Token = "api-secret-two"
	second, err := Build(BuildInput{
		Config:       cfg,
		ConfigPath:   configPath,
		ConfigSource: "explicit",
		Reason:       ReasonStartup,
	})
	if err != nil {
		t.Fatalf("Build(second): %v", err)
	}
	if first.ConfigHash == second.ConfigHash {
		t.Fatal("config hash did not change after secret-only change")
	}
}

func TestBuildRecordsExplicitBaggageSemantics(t *testing.T) {
	t.Parallel()

	cfg := config.Defaults()
	cfg.Pipelines = []config.PipelineEntry{
		{
			Name: "wisdom",
			On:   "event.start",
			Steps: []config.StepEntry{
				{
					ID:   "summarize",
					Uses: "fabric",
					Baggage: map[string]string{
						"summary.text": "payload.result",
						"from":         "payload.metadata",
						"namespace":    "whisper",
					},
				},
			},
		},
	}

	first, err := Build(BuildInput{
		Config: cfg,
		Reason: ReasonStartup,
	})
	if err != nil {
		t.Fatalf("Build(first): %v", err)
	}

	var semantics map[string]string
	if err := json.Unmarshal(first.Semantics, &semantics); err != nil {
		t.Fatalf("unmarshal semantics: %v", err)
	}
	if semantics["baggage_durability"] != "author_explicit_claims" {
		t.Fatalf("baggage_durability = %q", semantics["baggage_durability"])
	}
	if semantics["baggage_immutability"] != "deep_accretion_immutable_paths" {
		t.Fatalf("baggage_immutability = %q", semantics["baggage_immutability"])
	}
	if semantics["baggage_transition"] != "legacy_payload_promotion_without_baggage" {
		t.Fatalf("baggage_transition = %q", semantics["baggage_transition"])
	}

	var sanitized map[string]any
	if err := json.Unmarshal(first.SanitizedConfig, &sanitized); err != nil {
		t.Fatalf("unmarshal sanitized config: %v", err)
	}
	pipelines := sanitized["pipelines"].([]any)
	steps := pipelines[0].(map[string]any)["steps"].([]any)
	baggage := steps[0].(map[string]any)["baggage"].(map[string]any)
	if baggage["summary.text"] != "payload.result" {
		t.Fatalf("summary.text baggage = %#v", baggage["summary.text"])
	}
	if baggage["namespace"] != "whisper" {
		t.Fatalf("namespace baggage = %#v", baggage["namespace"])
	}

	cfg.Pipelines[0].Steps[0].Baggage["summary.text"] = "payload.summary"
	second, err := Build(BuildInput{
		Config: cfg,
		Reason: ReasonStartup,
	})
	if err != nil {
		t.Fatalf("Build(second): %v", err)
	}
	if first.ConfigHash == second.ConfigHash {
		t.Fatal("config hash did not change after baggage-only change")
	}
}

func TestInsertAndGetRoundTrip(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "state.db")
	db, err := storage.OpenSQLite(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	cfg := config.Defaults()
	snapshot, err := Build(BuildInput{
		Config:         cfg,
		Reason:         ReasonStartup,
		DuctileVersion: "test-version",
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if err := Insert(context.Background(), db, snapshot); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	got, err := Get(context.Background(), db, snapshot.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != snapshot.ID || got.ConfigHash != snapshot.ConfigHash || got.Reason != ReasonStartup {
		t.Fatalf("round trip mismatch: got %+v want %+v", got, snapshot)
	}
}
