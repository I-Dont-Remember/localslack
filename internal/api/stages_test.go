package api

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"localslack/internal/calllog"
	"localslack/internal/faults"
	"localslack/internal/service"
	"localslack/internal/store"
)

// TestEnforceAuth drives spec.auth through a custom registry over an unseeded store.
func TestEnforceAuth(t *testing.T) {
	t.Parallel()

	st, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	svc, err := service.New(t.Context(), st, time.Now, "x")
	if err != nil {
		t.Fatalf("service.New: %v", err)
	}
	svc.SetApps(service.App{ID: "A1", AppToken: "xapp-ok"})
	ok := func(context.Context, *call) (map[string]any, error) { return map[string]any{}, nil }
	a := &API{svc: svc, log: slog.New(slog.NewTextHandler(io.Discard, nil)), now: time.Now, rec: calllog.Noop{}, icpt: faults.Noop{}}
	a.methods = map[string]spec{
		"t.token": {fn: ok},
		"t.none":  {fn: ok, auth: authNone},
		"t.app":   {fn: ok, auth: authApp},
	}
	r := chi.NewRouter()
	r.Post("/api/{method}", a.serve)

	tests := []struct {
		method, token, want string
	}{
		{"t.none", "", `{"ok":true}`},
		{"t.none", "garbage", `{"ok":true}`},
		{"t.token", "", `{"error":"not_authed","ok":false}`},
		{"t.token", "xapp-ok", `{"error":"not_allowed_token_type","ok":false}`},
		{"t.app", "", `{"error":"not_authed","ok":false}`},
		{"t.app", "xapp-bad", `{"error":"invalid_auth","ok":false}`},
		{"t.app", "xapp-ok", `{"ok":true}`},
	}
	for _, tt := range tests {
		t.Run(tt.method+"/"+tt.token, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/"+tt.method, nil)
			if tt.token != "" {
				req.Header.Set("Authorization", "Bearer "+tt.token)
			}
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)
			if got := rec.Body.String(); got != tt.want+"\n" {
				t.Errorf("body = %q, want %q", got, tt.want)
			}
		})
	}
}
