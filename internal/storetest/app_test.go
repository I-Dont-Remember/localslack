package storetest_test

import (
	"net/http"
	"path/filepath"
	"testing"

	"localslack/internal/storetest"
)

func TestNewApp(t *testing.T) {
	t.Parallel()

	a := storetest.NewApp(t)
	b := storetest.NewApp(t)

	if a.URL == b.URL {
		t.Errorf("apps share URL %q, want isolated servers", a.URL)
	}
	if a.Config.DBPath == b.Config.DBPath {
		t.Errorf("apps share DBPath %q, want per-app temp DB", a.Config.DBPath)
	}
	if got, want := a.Config.Seed, storetest.PinnedSeed; got != want {
		t.Errorf("Seed = %q, want %q", got, want)
	}
	if got := filepath.Ext(a.Config.DBPath); got != ".db" {
		t.Errorf("DBPath ext = %q, want .db", got)
	}

	if empty, err := a.Store.IsEmpty(t.Context()); err != nil || empty {
		t.Errorf("Store.IsEmpty = %v, %v; want false, nil (seeded)", empty, err)
	}

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, a.URL+"/_localslack/health", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("health: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	if resp.StatusCode != http.StatusOK {
		t.Errorf("health status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}
