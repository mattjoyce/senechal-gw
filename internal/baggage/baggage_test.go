package baggage

import (
	"strings"
	"testing"

	"github.com/mattjoyce/ductile/internal/router/dsl"
)

func TestApplyClaimsMapsAndBulkImports(t *testing.T) {
	payload := map[string]any{
		"message":  "hello",
		"language": "en",
		"metadata": map[string]any{
			"duration": float64(12),
		},
	}
	ctx := map[string]any{
		"origin": map[string]any{"channel": "chan-1"},
	}
	spec := &dsl.BaggageSpec{
		Mappings: map[string]string{
			"summary.text":    "payload.message",
			"summary.lang":    "payload.language",
			"origin.channel":  "context.origin.channel",
			"whisper.message": "payload.message",
		},
		Bulk: &dsl.BaggageBulkSpec{
			From:      "payload.metadata",
			Namespace: "whisper.metadata",
		},
	}

	got, err := ApplyClaims(payload, spec, ctx)
	if err != nil {
		t.Fatalf("ApplyClaims() error = %v", err)
	}

	summary := got["summary"].(map[string]any)
	if summary["text"] != "hello" || summary["lang"] != "en" {
		t.Fatalf("summary = %+v, want mapped text/lang", summary)
	}
	origin := got["origin"].(map[string]any)
	if origin["channel"] != "chan-1" {
		t.Fatalf("origin.channel = %v, want chan-1", origin["channel"])
	}
	whisper := got["whisper"].(map[string]any)
	if whisper["message"] != "hello" {
		t.Fatalf("whisper.message = %v, want hello", whisper["message"])
	}
	metadata := whisper["metadata"].(map[string]any)
	if metadata["duration"] != float64(12) {
		t.Fatalf("whisper.metadata.duration = %v, want 12", metadata["duration"])
	}
	if _, exists := got["message"]; exists {
		t.Fatalf("legacy payload message was promoted into explicit baggage: %+v", got)
	}
}

func TestApplyClaimsRejectsMissingSource(t *testing.T) {
	_, err := ApplyClaims(
		map[string]any{"message": "hello"},
		&dsl.BaggageSpec{Mappings: map[string]string{"summary.text": "payload.missing"}},
		nil,
	)
	if err == nil {
		t.Fatalf("expected missing source error")
	}
	if !strings.Contains(err.Error(), "path not found") {
		t.Fatalf("error = %v, want path not found", err)
	}
}

func TestApplyClaimsRequiresNamespaceForBulkImport(t *testing.T) {
	_, err := ApplyClaims(
		map[string]any{"metadata": map[string]any{"duration": 12}},
		&dsl.BaggageSpec{Bulk: &dsl.BaggageBulkSpec{From: "payload.metadata"}},
		nil,
	)
	if err == nil {
		t.Fatalf("expected missing namespace error")
	}
	if !strings.Contains(err.Error(), "namespace is required") {
		t.Fatalf("error = %v, want namespace is required", err)
	}
}
