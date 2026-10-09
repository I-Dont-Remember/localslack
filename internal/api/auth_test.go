package api_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"localslack/internal/api"
	"localslack/internal/constants"
	"localslack/internal/service"
	"localslack/internal/storetest"
)

func humanAndBot(t *testing.T, app *storetest.App) (human, bot service.User) {
	t.Helper()
	us, err := app.Service.Users(t.Context())
	if err != nil {
		t.Fatalf("Users: %v", err)
	}
	for _, u := range us {
		if u.IsBot && bot.ID == "" {
			bot = u
		}
		if !u.IsBot && human.ID == "" {
			human = u
		}
	}
	return human, bot
}

func TestAuthTest(t *testing.T) {
	t.Parallel()

	app := storetest.NewApp(t)
	human, bot := humanAndBot(t, app)
	tests := []struct {
		name      string
		token     string
		wantError string
		wantUser  string
		wantBotID bool
	}{
		{"missing token", "", "not_authed", "", false},
		{"unknown user token", "xoxp-UNOPE", "invalid_auth", "", false},
		{"garbage token", "hunter2", "invalid_auth", "", false},
		{"bot token", "xoxb-test", "", bot.ID, true},
		{"user token", "xoxp-" + human.ID, "", human.ID, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			status, body := call(t, app, "auth.test", tt.token, nil)
			if status != http.StatusOK {
				t.Errorf("status = %d, want 200 (Slack errors are HTTP 200)", status)
			}
			if tt.wantError != "" {
				want := map[string]any{"ok": false, "error": tt.wantError}
				if diff := cmp.Diff(want, body); diff != "" {
					t.Errorf("body mismatch (-want +got):\n%s", diff)
				}
				return
			}
			if body["ok"] != true || body["user_id"] != tt.wantUser {
				t.Fatalf("body = %v, want ok and user_id %s", body, tt.wantUser)
			}
			if _, has := body["bot_id"]; has != tt.wantBotID {
				t.Errorf("bot_id present = %v, want %v", has, tt.wantBotID)
			}
			if id, _ := body["team_id"].(string); !strings.HasPrefix(id, "T") {
				t.Errorf("team_id = %q, want T...", id)
			}
			if u, _ := body["url"].(string); !strings.HasPrefix(u, "http") {
				t.Errorf("url = %q, want absolute url", u)
			}
		})
	}
}

func TestParamAndTokenTransports(t *testing.T) {
	t.Parallel()

	app := storetest.NewApp(t)
	tests := []struct {
		name        string
		verb        string
		path        string
		token       string
		contentType string
		body        string
	}{
		{"GET with query token", http.MethodGet, "/api/auth.test?token=xoxb-test", "", "", ""},
		{"POST json body token", http.MethodPost, "/api/auth.test", "", constants.ContentTypeJSON, `{"token":"xoxb-test"}`},
		{"POST form body token", http.MethodPost, "/api/auth.test", "", constants.ContentTypeForm, "token=xoxb-test"},
		{"POST bearer header", http.MethodPost, "/api/auth.test", "xoxb-test", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, body := do(t, tt.verb, app.URL+tt.path, tt.token, tt.contentType, strings.NewReader(tt.body))
			if body["ok"] != true {
				t.Errorf("body = %v, want ok", body)
			}
		})
	}
}

func TestUsersList(t *testing.T) {
	t.Parallel()

	app := storetest.NewApp(t)
	want, err := app.Service.Users(t.Context())
	if err != nil {
		t.Fatalf("Users: %v", err)
	}
	_, body := call(t, app, "users.list", "xoxb-test", nil)
	members, _ := body["members"].([]any)
	if len(members) != len(want) {
		t.Fatalf("members = %d, want %d", len(members), len(want))
	}
	got := map[string]bool{}
	for _, m := range members {
		got[m.(map[string]any)["id"].(string)] = m.(map[string]any)["is_bot"].(bool)
	}
	for _, u := range want {
		if isBot, ok := got[u.ID]; !ok || isBot != u.IsBot {
			t.Errorf("user %s: present=%v is_bot=%v, want is_bot=%v", u.ID, ok, isBot, u.IsBot)
		}
	}
	if _, has := body["response_metadata"]; !has {
		t.Error("missing response_metadata")
	}
}

func TestUnknownMethodFallsThrough(t *testing.T) {
	t.Parallel()

	app := storetest.NewApp(t)
	for _, token := range []string{"", "xoxb-test"} {
		status, body := call(t, app, "no.such.method", token, nil)
		if status != http.StatusNotFound || body["error"] != "not_implemented" {
			t.Errorf("token %q: status %d body %v, want 404 not_implemented", token, status, body)
		}
	}
}

func TestEveryServiceErrorHasASlackCode(t *testing.T) {
	t.Parallel()

	for _, err := range service.AllErrors() {
		if code, ok := api.ErrorCode(err); !ok || code == "" {
			t.Errorf("service error %q has no Slack error code mapping", err)
		}
	}
}
