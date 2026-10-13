package server_test

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"localslack/internal/config"
	"localslack/internal/server"
)

const startScenario = `{"users":[{"id":"UANN","name":"ann"}],"channels":[{"id":"CGEN","name":"general","members":["UANN"]}]}`

func TestOpenStoreWithScenario(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	file := filepath.Join(dir, "world.json")
	if err := os.WriteFile(file, []byte(startScenario), 0o600); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, nil))
	cfg := config.Config{Seed: "5", DBPath: filepath.Join(dir, "t.db"), Scenario: file}

	st, err := server.OpenStore(t.Context(), log, cfg, now)
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	users, _ := st.Users(t.Context())
	if len(users) != 1 || users[0].ID != "UANN" {
		t.Errorf("users = %+v, want the scenario's UANN only", users)
	}
	if !strings.Contains(buf.String(), "seeded database from scenario") {
		t.Errorf("log %q lacks the scenario seeding line", buf.String())
	}
	if err := st.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Second start: the database has data, so the scenario is ignored with a pointer to `reset --scenario`.
	buf.Reset()
	other := filepath.Join(dir, "other.json")
	if err := os.WriteFile(other, []byte(`{"users":[{"id":"UBOB","name":"bob"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg.Scenario = other
	again, err := server.OpenStore(t.Context(), log, cfg, now)
	if err != nil {
		t.Fatalf("second OpenStore: %v", err)
	}
	t.Cleanup(func() { _ = again.Close() })
	users, _ = again.Users(t.Context())
	if len(users) != 1 || users[0].ID != "UANN" {
		t.Errorf("users after second start = %+v, want untouched", users)
	}
	if out := buf.String(); !strings.Contains(out, "scenario ignored") || !strings.Contains(out, "localslack reset --scenario") {
		t.Errorf("log %q lacks the ignore warning with the reset command", out)
	}
}

func TestOpenStoreRejectsBadScenario(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(bad, []byte(`{"users":[{"id":"X1","name":"ann"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{DBPath: filepath.Join(dir, "t.db"), Scenario: bad}

	_, err := server.OpenStore(t.Context(), slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)), cfg, time.Now())

	if err == nil || !strings.Contains(err.Error(), "/users/0/id") {
		t.Errorf("OpenStore err = %v, want a pointer-bearing validation error", err)
	}
}
