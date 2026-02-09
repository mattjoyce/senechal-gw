package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestValidateAPIKey(t *testing.T) {
	t.Parallel()

	if got := ValidateAPIKey("provided", "provided"); !got {
		t.Fatalf("expected true for matching keys")
	}
	if got := ValidateAPIKey("provided", "other"); got {
		t.Fatalf("expected false for mismatched keys")
	}
	if got := ValidateAPIKey("", "configured"); got {
		t.Fatalf("expected false for empty provided key")
	}
	if got := ValidateAPIKey("provided", ""); got {
		t.Fatalf("expected false for empty configured key")
	}
}

func TestExtractAPIKey(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "http://example.test", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	key, err := ExtractAPIKey(req)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if key != "test-key" {
		t.Fatalf("expected key %q, got %q", "test-key", key)
	}

	req2 := httptest.NewRequest(http.MethodGet, "http://example.test", nil)
	if _, err := ExtractAPIKey(req2); err == nil {
		t.Fatalf("expected error for missing header")
	}

	req3 := httptest.NewRequest(http.MethodGet, "http://example.test", nil)
	req3.Header.Set("Authorization", "Basic abc")
	if _, err := ExtractAPIKey(req3); err == nil {
		t.Fatalf("expected error for non-bearer header")
	}

	req4 := httptest.NewRequest(http.MethodGet, "http://example.test", nil)
	req4.Header.Set("Authorization", "Bearer   ")
	if _, err := ExtractAPIKey(req4); err == nil {
		t.Fatalf("expected error for empty bearer key")
	}
}
