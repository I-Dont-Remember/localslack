// Package storetest builds isolated, fully wired LocalSlack apps for tests.
package storetest

import (
	"io"
	"log/slog"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"localslack/internal/config"
	"localslack/internal/server"
	"localslack/internal/store"
)

// PinnedSeed makes generated data identical across test runs.
const PinnedSeed = "1"

// PinnedNow is the clock used for seeding, so timestamps are stable.
func PinnedNow() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }

// App is a running server backed by its own temp-file database.
type App struct {
	URL    string
	Config config.Config
	Store  *store.Store
}

// NewApp starts an isolated app (temp DB, pinned seed) and stops it on cleanup.
func NewApp(t *testing.T) *App {
	t.Helper()

	cfg := config.Config{
		Addr:   "127.0.0.1:0",
		DBPath: filepath.Join(t.TempDir(), "localslack.db"),
		Seed:   PinnedSeed,
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	st, err := server.OpenStore(t.Context(), log, cfg, PinnedNow())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() {
		if err := st.Close(); err != nil {
			t.Errorf("close store: %v", err)
		}
	})
	srv := httptest.NewServer(server.NewRouter(server.Deps{Log: log}))
	t.Cleanup(srv.Close)
	return &App{URL: srv.URL, Config: cfg, Store: st}
}

// NewStore opens a seeded (pinned seed and clock) temp-file store, closed on cleanup.
func NewStore(t *testing.T) *store.Store {
	t.Helper()

	cfg := config.Config{DBPath: filepath.Join(t.TempDir(), "localslack.db"), Seed: PinnedSeed}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	st, err := server.OpenStore(t.Context(), log, cfg, PinnedNow())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() {
		if err := st.Close(); err != nil {
			t.Errorf("close store: %v", err)
		}
	})
	return st
}
