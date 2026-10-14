package admin_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/go-cmp/cmp"

	"localslack/internal/config"
	"localslack/internal/events"
	"localslack/internal/server"
	"localslack/internal/storetest"
)

// droppingDispatcher reports a fixed drop count, like an Engine whose queue overflowed.
type droppingDispatcher struct{ events.Noop }

func (droppingDispatcher) Dropped() int64 { return 7 }

func TestHealthReportsDroppedEvents(t *testing.T) {
	t.Parallel()

	build := func(t *testing.T, opts ...server.Option) string {
		t.Helper()
		cfg := config.Config{DBPath: t.TempDir() + "/ls.db", Seed: storetest.PinnedSeed}
		built, err := server.Build(t.Context(), slog.New(slog.NewTextHandler(io.Discard, nil)), cfg, storetest.PinnedNow, opts...)
		if err != nil {
			t.Fatalf("Build: %v", err)
		}
		t.Cleanup(func() { _ = built.Close() })
		srv := httptest.NewServer(built.Handler)
		t.Cleanup(srv.Close)
		return srv.URL
	}
	tests := []struct {
		name string
		opts []server.Option
		want map[string]any
	}{
		{"dispatcher that counts drops", []server.Option{server.WithDispatcher(droppingDispatcher{})}, map[string]any{"status": "ok", "events_dropped": float64(7)}},
		{"dispatcher that does not count", []server.Option{server.WithDispatcher(events.Noop{})}, map[string]any{"status": "ok", "events_dropped": float64(0)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			url := build(t, tt.opts...)

			status, _, got := doReq(t, http.MethodGet, url+"/_localslack/health", "", nil)

			if status != http.StatusOK {
				t.Fatalf("status %d", status)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("health (-want +got):\n%s", diff)
			}
		})
	}
}
