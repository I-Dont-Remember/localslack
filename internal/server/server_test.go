package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"localslack/internal/server"
)

type healthResponse struct {
	OK bool `json:"ok"`
}

type errorResponse struct {
	OK    bool   `json:"ok"`
	Error string `json:"error"`
}

func TestNotImplementedHandler(t *testing.T) {
	t.Parallel()
	handler := server.NewRouter()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/chat.postMessage", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json")
	}
	want := errorResponse{OK: false, Error: "not_implemented"}
	var got errorResponse
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if got != want {
		t.Errorf("body = %+v, want %+v", got, want)
	}
}

func TestHealthEndpoint(t *testing.T) {
	t.Parallel()
	handler := server.NewRouter()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/_localslack/health", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json")
	}
	want := healthResponse{OK: true}
	var got healthResponse
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if got != want {
		t.Errorf("body = %+v, want %+v", got, want)
	}
}
