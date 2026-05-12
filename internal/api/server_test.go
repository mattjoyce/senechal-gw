package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCORSMiddlewarePreflight(t *testing.T) {
	handler := corsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("preflight should not call next handler")
	}))

	req := httptest.NewRequest(http.MethodOptions, "/plugin/echo/poll", nil)
	req.Header.Set("Origin", "https://example.test")
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)
	req.Header.Set("Access-Control-Request-Headers", "Authorization, Content-Type")
	resp := httptest.NewRecorder()

	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusNoContent)
	}
	assertHeader(t, resp, "Access-Control-Allow-Origin", "https://example.test")
	assertHeader(t, resp, "Access-Control-Allow-Credentials", "true")
	assertHeader(t, resp, "Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	assertHeader(t, resp, "Access-Control-Allow-Headers", "Accept, Authorization, Content-Type, X-CSRF-Token")
	assertHeader(t, resp, "Access-Control-Max-Age", "300")
}

func TestCORSMiddlewareActualRequest(t *testing.T) {
	called := false
	handler := corsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.Header().Set("Link", "</jobs>; rel=next")
		w.WriteHeader(http.StatusAccepted)
	}))

	req := httptest.NewRequest(http.MethodPost, "/pipeline/default", nil)
	req.Header.Set("Origin", "https://example.test")
	resp := httptest.NewRecorder()

	handler.ServeHTTP(resp, req)

	if !called {
		t.Fatal("actual request did not call next handler")
	}
	if resp.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusAccepted)
	}
	assertHeader(t, resp, "Access-Control-Allow-Origin", "https://example.test")
	assertHeader(t, resp, "Access-Control-Allow-Credentials", "true")
	assertHeader(t, resp, "Access-Control-Expose-Headers", "Link")
}

func TestCORSMiddlewareNoOrigin(t *testing.T) {
	handler := corsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if got := resp.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want empty", got)
	}
}

func assertHeader(t *testing.T, resp *httptest.ResponseRecorder, key, want string) {
	t.Helper()
	if got := resp.Header().Get(key); got != want {
		t.Fatalf("%s = %q, want %q", key, got, want)
	}
}
