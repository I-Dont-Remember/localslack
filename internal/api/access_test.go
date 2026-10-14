package api_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"localslack/internal/storetest"
)

func TestPrivateChannelAccess(t *testing.T) {
	t.Parallel()
	app := storetest.NewApp(t)
	_, users := call(t, app, "users.list", botToken, nil)
	var human string
	for _, u := range items(t, users, "members") {
		if u["is_bot"] == false {
			human = u["id"].(string)
			break
		}
	}
	userTok := "xoxp-" + human

	_, created := call(t, app, "conversations.create", userTok, map[string]string{"name": "secret", "is_private": "true"})
	ch, _ := created["channel"].(map[string]any)
	if created["ok"] != true || ch["is_private"] != true || ch["is_group"] != true || ch["is_channel"] != false || ch["is_member"] != true {
		t.Fatalf("create private = %v", created)
	}
	id := ch["id"].(string)

	tests := []struct {
		name   string
		method string
		token  string
		params map[string]string
		want   string // error, "" = ok
	}{
		{"creator reads history", "conversations.history", userTok, map[string]string{"channel": id}, ""},
		{"bot non-member history", "conversations.history", botToken, map[string]string{"channel": id}, "channel_not_found"},
		{"bot non-member info", "conversations.info", botToken, map[string]string{"channel": id}, "channel_not_found"},
		{"bot non-member members", "conversations.members", botToken, map[string]string{"channel": id}, "channel_not_found"},
		{"bot non-member post", "chat.postMessage", botToken, map[string]string{"channel": id, "text": "x"}, "channel_not_found"},
		{"creator posts", "chat.postMessage", userTok, map[string]string{"channel": id, "text": "x"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, body := call(t, app, tt.method, tt.token, tt.params)
			got := ""
			if body["ok"] != true {
				got, _ = body["error"].(string)
			}
			if got != tt.want {
				t.Errorf("error = %q, want %q (%v)", got, tt.want, body)
			}
		})
	}

	names := func(token, types string) []string {
		_, body := call(t, app, "conversations.list", token, map[string]string{"types": types, "limit": "1000"})
		var out []string
		for _, c := range items(t, body, "channels") {
			out = append(out, c["name"].(string))
		}
		return out
	}
	if diff := cmp.Diff([]string{"secret"}, names(userTok, "private_channel")); diff != "" {
		t.Errorf("member private list (-want +got):\n%s", diff)
	}
	if got := names(botToken, "private_channel"); len(got) != 0 {
		t.Errorf("non-member private list = %v, want none", got)
	}
}
