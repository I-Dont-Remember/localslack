package server_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"localslack/internal/config"
	"localslack/internal/server"
)

const manifestJSON = `{
  "display_information": {"name": "Loaded App"},
  "features": {"bot_user": {"display_name": "loaded-bot"}},
  "settings": {"event_subscriptions": {"request_url": "http://127.0.0.1:9/slack/events", "bot_events": ["app_mention"]}}
}`

const manifestYAML = `
display_information:
  name: Yaml Loaded
settings:
  socket_mode_enabled: true
`

func writeFile(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return p
}

func pinnedNow() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }

func TestBuildRegistersManifestsIdempotently(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cfg := config.Config{
		Seed: "5", DBPath: filepath.Join(dir, "t.db"),
		AppManifest: []string{writeFile(t, dir, "a.json", manifestJSON), writeFile(t, dir, "b.yaml", manifestYAML)},
	}

	first, err := server.Build(t.Context(), discardLogger(), cfg, pinnedNow)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	apps := first.Service.Apps()
	if len(apps) != 2 {
		t.Fatalf("apps after first boot = %d, want 2", len(apps))
	}
	byName := map[string]string{}
	for _, a := range apps {
		byName[a.Name] = a.ID
	}
	if _, err := first.Service.SetAppDeliverTo(t.Context(), byName["Loaded App"], "http://localhost:7777"); err != nil {
		t.Fatalf("SetAppDeliverTo: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	second, err := server.Build(t.Context(), discardLogger(), cfg, pinnedNow)
	if err != nil {
		t.Fatalf("Build (restart): %v", err)
	}
	t.Cleanup(func() { _ = second.Close() })
	again := second.Service.Apps()
	if len(again) != 2 {
		t.Fatalf("apps after restart = %d, want 2 (idempotent upsert)", len(again))
	}
	for _, a := range again {
		if byName[a.Name] != a.ID {
			t.Errorf("app %q changed id across restart: %s -> %s", a.Name, byName[a.Name], a.ID)
		}
		if a.Name == "Loaded App" && a.DeliverTo != "http://localhost:7777" {
			t.Errorf("restart clobbered deliver_to: %q", a.DeliverTo)
		}
		if a.Name == "Yaml Loaded" && !a.SocketMode {
			t.Error("YAML manifest socket_mode_enabled not honored")
		}
	}
}

func TestBuildFailsOnBadManifest(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	tests := []struct {
		name string
		path string
		want string
	}{
		{"missing file", filepath.Join(dir, "nope.json"), "nope.json"},
		{"invalid manifest", writeFile(t, dir, "bad.json", `{"display_information":{}}`), "/display_information/name"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := config.Config{Seed: "5", DBPath: filepath.Join(t.TempDir(), "t.db"), AppManifest: []string{tt.path}}
			_, err := server.Build(t.Context(), discardLogger(), cfg, pinnedNow)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Build err = %v, want it to mention %q", err, tt.want)
			}
		})
	}
}
