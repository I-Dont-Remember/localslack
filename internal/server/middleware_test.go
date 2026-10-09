package server

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRecovererReturnsJSON500(t *testing.T) {
	t.Parallel()
	var logs bytes.Buffer
	log := slog.New(slog.NewTextHandler(&logs, nil))
	h := recoverer(log)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	}))
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	resp := w.Result()
	t.Cleanup(func() { _ = resp.Body.Close() })
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusInternalServerError)
	}
	var got map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	want := map[string]any{"ok": false, "error": "internal_error"}
	if got["ok"] != want["ok"] || got["error"] != want["error"] {
		t.Errorf("body = %v, want %v", got, want)
	}
	if !strings.Contains(logs.String(), "boom") {
		t.Errorf("panic not logged: %q", logs.String())
	}
}

func TestRequestLoggerLogsStatus(t *testing.T) {
	t.Parallel()
	var logs bytes.Buffer
	log := slog.New(slog.NewTextHandler(&logs, nil))
	h := requestLogger(log)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/x", nil)

	h.ServeHTTP(httptest.NewRecorder(), req)

	for _, want := range []string{"method=GET", "path=/x", "status=418"} {
		if !strings.Contains(logs.String(), want) {
			t.Errorf("log %q missing %q", logs.String(), want)
		}
	}
}
