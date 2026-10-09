package server_test

import (
	"bytes"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"localslack/internal/config"
	"localslack/internal/server"
)

func TestOpenStoreSeeding(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name      string
		cfg       config.Config
		wantEmpty bool
		wantLog   string
	}{
		{"pinned seed", config.Config{Seed: "5"}, false, ""},
		{"no seed flag", config.Config{Seed: "5", NoSeed: true}, true, ""},
		{"random seed is logged", config.Config{}, false, "seed="},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var buf bytes.Buffer
			log := slog.New(slog.NewTextHandler(&buf, nil))
			cfg := tt.cfg
			cfg.DBPath = filepath.Join(t.TempDir(), "t.db")

			st, err := server.OpenStore(t.Context(), log, cfg, now)
			if err != nil {
				t.Fatalf("OpenStore: %v", err)
			}
			t.Cleanup(func() { _ = st.Close() })

			got, err := st.IsEmpty(t.Context())
			if err != nil {
				t.Fatalf("IsEmpty: %v", err)
			}
			if got != tt.wantEmpty {
				t.Errorf("IsEmpty = %v, want %v", got, tt.wantEmpty)
			}
			if !strings.Contains(buf.String(), tt.wantLog) {
				t.Errorf("log %q missing %q", buf.String(), tt.wantLog)
			}
		})
	}
}

func TestOpenStoreDoesNotReseed(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, nil))
	cfg := config.Config{Seed: "5", DBPath: filepath.Join(t.TempDir(), "t.db")}
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	first, err := server.OpenStore(t.Context(), log, cfg, now)
	if err != nil {
		t.Fatalf("first OpenStore: %v", err)
	}
	want, err := first.Users(t.Context())
	if err != nil {
		t.Fatalf("Users: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	cfg.Seed = "6" // would produce different data if it reseeded
	second, err := server.OpenStore(t.Context(), log, cfg, now)
	if err != nil {
		t.Fatalf("second OpenStore: %v", err)
	}
	t.Cleanup(func() { _ = second.Close() })
	got, err := second.Users(t.Context())
	if err != nil {
		t.Fatalf("Users: %v", err)
	}
	if len(got) != len(want) || got[0] != want[0] {
		t.Errorf("users changed across restart: got %v, want %v", got, want)
	}
}
