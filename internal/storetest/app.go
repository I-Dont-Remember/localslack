// Package storetest builds isolated, fully wired LocalSlack apps for tests.
package storetest

import (
	"io"
	"log/slog"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"localslack/internal/config"
	"localslack/internal/server"
)

// PinnedSeed makes generated data identical across test runs.
const PinnedSeed = "1"

// App is a running server backed by its own temp-file database.
type App struct {
	URL    string
	Config config.Config
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
	srv := httptest.NewServer(server.NewRouter(server.Deps{Log: log}))
	t.Cleanup(srv.Close)
	return &App{URL: srv.URL, Config: cfg}
}
