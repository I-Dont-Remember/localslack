package server_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"localslack/internal/config"
	"localslack/internal/server"
)

func TestBuildWiresServiceAndStore(t *testing.T) {
	t.Parallel()

	now := func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }
	cfg := config.Config{Seed: "5", DBPath: filepath.Join(t.TempDir(), "t.db")}
	app, err := server.Build(t.Context(), discardLogger(), cfg, now)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if app.Store == nil || app.Service == nil {
		t.Fatalf("Build left Store=%v Service=%v, want both set", app.Store, app.Service)
	}

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/_localslack/health", nil)
	w := httptest.NewRecorder()
	app.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("health status = %d, want %d", w.Code, http.StatusOK)
	}
	_, _ = io.Copy(io.Discard, w.Result().Body)
	_ = w.Result().Body.Close()

	if err := app.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}
