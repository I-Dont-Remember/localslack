package api_test

import (
	"net/url"
	"strings"
	"testing"

	"localslack/internal/appmanifest"
	"localslack/internal/service"
	"localslack/internal/storetest"
)

func registerSocketApp(t *testing.T, app *storetest.App) service.App {
	t.Helper()
	parsed, err := appmanifest.Parse([]byte(`{"display_information":{"name":"Sock"},"settings":{"socket_mode_enabled":true}}`))
	if err != nil {
		t.Fatalf("parse manifest: %v", err)
	}
	a, err := app.Service.RegisterApp(t.Context(), service.RegisterInput{Manifest: parsed, AppToken: "xapp-1-test-socket"})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	return a
}

func TestAppsConnectionsOpen(t *testing.T) {
	t.Parallel()
	app := storetest.NewApp(t)
	reg := registerSocketApp(t, app)

	status, got := call(t, app, "apps.connections.open", reg.AppToken, nil)
	if status != 200 || got["ok"] != true {
		t.Fatalf("open = %d %v, want ok", status, got)
	}
	raw, _ := got["url"].(string)
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("url %q: %v", raw, err)
	}
	host := strings.TrimPrefix(app.URL, "http://")
	if u.Scheme != "ws" || u.Host != host || u.Path != "/_localslack/socket" {
		t.Errorf("url = %q, want ws://%s/_localslack/socket?...", raw, host)
	}
	q := u.Query()
	if q.Get("app_id") != reg.ID || q.Get("ticket") == "" {
		t.Errorf("query = %v, want ticket and app_id=%s", q, reg.ID)
	}

	_, again := call(t, app, "apps.connections.open", reg.AppToken, nil)
	if again["url"] == got["url"] {
		t.Error("two opens returned the same url, want a fresh one-time ticket each")
	}
}

func TestAppsConnectionsOpenTokenTypes(t *testing.T) {
	t.Parallel()
	app := storetest.NewApp(t)
	reg := registerSocketApp(t, app)
	human, _ := humanAndBot(t, app)

	tests := []struct {
		name, method, token, wantErr string
	}{
		{"no token", "apps.connections.open", "", "not_authed"},
		{"unknown app token", "apps.connections.open", "xapp-1-nope", "invalid_auth"},
		{"bot token", "apps.connections.open", reg.BotToken, "not_allowed_token_type"},
		{"user token", "apps.connections.open", "xoxp-" + human.ID, "not_allowed_token_type"},
		{"app token on other method", "auth.test", reg.AppToken, "not_allowed_token_type"},
		{"app token posting", "chat.postMessage", reg.AppToken, "not_allowed_token_type"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, got := call(t, app, tt.method, tt.token, map[string]string{"channel": "C1", "text": "x"})
			if got["ok"] != false || got["error"] != tt.wantErr {
				t.Errorf("%s = %v, want error %q", tt.method, got, tt.wantErr)
			}
		})
	}
}
