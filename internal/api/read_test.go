package api_test

import (
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"

	"localslack/internal/storetest"
)

func firstHuman(t *testing.T, app *storetest.App) map[string]any {
	t.Helper()
	_, body := call(t, app, "users.list", botToken, nil)
	for _, u := range items(t, body, "members") {
		if u["is_bot"] == false {
			return u
		}
	}
	t.Fatal("no human user")
	return nil
}

func stringList(t *testing.T, body map[string]any, key string) []string {
	t.Helper()
	raw, ok := body[key].([]any)
	if !ok {
		t.Fatalf("body[%q] not a list: %v", key, body)
	}
	out := make([]string, 0, len(raw))
	for _, r := range raw {
		out = append(out, r.(string))
	}
	return out
}

func wantErrBody(code string) map[string]any { return map[string]any{"ok": false, "error": code} }

func TestConversationsInfo(t *testing.T) {
	t.Parallel()

	app := storetest.NewApp(t)
	general := generalID(t, app)
	tests := []struct {
		name      string
		token     string
		params    map[string]string
		wantError string
		wantMembs bool
	}{
		{"ok", botToken, map[string]string{"channel": general}, "", false},
		{"with num_members", botToken, map[string]string{"channel": general, "include_num_members": "true"}, "", true},
		{"unknown", botToken, map[string]string{"channel": "C00000000"}, "channel_not_found", false},
		{"missing", botToken, nil, "channel_not_found", false},
		{"no token", "", map[string]string{"channel": general}, "not_authed", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, body := call(t, app, "conversations.info", tt.token, tt.params)
			if tt.wantError != "" {
				if diff := cmp.Diff(wantErrBody(tt.wantError), body); diff != "" {
					t.Errorf("body mismatch (-want +got):\n%s", diff)
				}
				return
			}
			ch, _ := body["channel"].(map[string]any)
			if body["ok"] != true || ch["id"] != general || ch["name"] != "general" || ch["is_channel"] != true || ch["is_private"] != false {
				t.Fatalf("body %v, want ok #general", body)
			}
			if _, got := ch["num_members"]; got != tt.wantMembs {
				t.Errorf("num_members present = %v, want %v", got, tt.wantMembs)
			}
		})
	}
}

func TestUsersInfo(t *testing.T) {
	t.Parallel()

	app := storetest.NewApp(t)
	human := firstHuman(t, app)
	tests := []struct {
		name      string
		params    map[string]string
		wantError string
	}{
		{"ok", map[string]string{"user": human["id"].(string)}, ""},
		{"unknown", map[string]string{"user": "U00000000"}, "user_not_found"},
		{"missing", nil, "user_not_found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, body := call(t, app, "users.info", botToken, tt.params)
			want := map[string]any{"ok": true, "user": human}
			if tt.wantError != "" {
				want = wantErrBody(tt.wantError)
			}
			if diff := cmp.Diff(want, body); diff != "" {
				t.Errorf("body mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestUsersInfoBotCarriesBotID(t *testing.T) {
	t.Parallel()

	app := storetest.NewApp(t)
	_, auth := call(t, app, "auth.test", botToken, nil)
	_, body := call(t, app, "users.info", botToken, map[string]string{"user": auth["user_id"].(string)})
	user, _ := body["user"].(map[string]any)
	profile, _ := user["profile"].(map[string]any)
	if user["is_bot"] != true || profile["bot_id"] != auth["bot_id"] {
		t.Errorf("user %v, want is_bot with profile.bot_id %v", user, auth["bot_id"])
	}
}

func TestConversationsMembers(t *testing.T) {
	t.Parallel()

	app := storetest.NewApp(t)
	general := generalID(t, app)
	_, all := call(t, app, "conversations.members", botToken, map[string]string{"channel": general, "limit": "1000"})
	ids := stringList(t, all, "members")
	if len(ids) < 3 {
		t.Fatalf("seeded #general has %d members, want >= 3 for pagination", len(ids))
	}

	t.Run("sorted full list", func(t *testing.T) {
		t.Parallel()
		if !slices.IsSorted(ids) || nextCursor(all) != "" {
			t.Errorf("members %v cursor %q, want sorted and no cursor", ids, nextCursor(all))
		}
	})
	t.Run("paginates without loss", func(t *testing.T) {
		t.Parallel()
		var got []string
		cursor := ""
		for range len(ids) + 1 {
			_, page := call(t, app, "conversations.members", botToken, map[string]string{"channel": general, "limit": "2", "cursor": cursor})
			got = append(got, stringList(t, page, "members")...)
			if cursor = nextCursor(page); cursor == "" {
				break
			}
		}
		if diff := cmp.Diff(ids, got); diff != "" {
			t.Errorf("paged members mismatch (-want +got):\n%s", diff)
		}
	})
	errs := []struct {
		name   string
		params map[string]string
		want   string
	}{
		{"unknown channel", map[string]string{"channel": "C00000000"}, "channel_not_found"},
		{"missing channel", nil, "channel_not_found"},
		{"bad cursor", map[string]string{"channel": general, "cursor": "junk"}, "invalid_cursor"},
	}
	for _, tt := range errs {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, body := call(t, app, "conversations.members", botToken, tt.params)
			if diff := cmp.Diff(wantErrBody(tt.want), body); diff != "" {
				t.Errorf("body mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestAPITest(t *testing.T) {
	t.Parallel()

	app := storetest.NewApp(t)
	foo := map[string]any{"foo": "bar"}
	tests := []struct {
		name   string
		token  string
		params map[string]string
		want   map[string]any
	}{
		{"no params", "", nil, map[string]any{"ok": true}},
		{"echoes args without a token", "", map[string]string{"foo": "bar"}, map[string]any{"ok": true, "args": foo}},
		{"token never echoed", botToken, map[string]string{"foo": "bar"}, map[string]any{"ok": true, "args": foo}},
		{
			"error", "",
			map[string]string{"error": "my_error", "foo": "bar"},
			map[string]any{"ok": false, "error": "my_error", "args": map[string]any{"error": "my_error", "foo": "bar"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			status, body := call(t, app, "api.test", tt.token, tt.params)
			if status != 200 {
				t.Errorf("status %d, want 200", status)
			}
			if diff := cmp.Diff(tt.want, body); diff != "" {
				t.Errorf("body mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
