package api_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"localslack/internal/storetest"
)

const botToken = "xoxb-test"

func items(t *testing.T, body map[string]any, key string) []map[string]any {
	t.Helper()
	raw, ok := body[key].([]any)
	if !ok {
		t.Fatalf("body[%q] is not a list: %v", key, body)
	}
	out := make([]map[string]any, 0, len(raw))
	for _, r := range raw {
		out = append(out, r.(map[string]any))
	}
	return out
}

func nextCursor(body map[string]any) string {
	md, _ := body["response_metadata"].(map[string]any)
	c, _ := md["next_cursor"].(string)
	return c
}

func generalID(t *testing.T, app *storetest.App) string {
	t.Helper()
	_, body := call(t, app, "conversations.list", botToken, map[string]string{"limit": "1000"})
	for _, c := range items(t, body, "channels") {
		if c["name"] == "general" {
			return c["id"].(string)
		}
	}
	t.Fatal("no #general")
	return ""
}

func TestConversationsCreate(t *testing.T) {
	t.Parallel()

	app := storetest.NewApp(t)
	tests := []struct {
		name      string
		token     string
		params    map[string]string
		wantError string
	}{
		{"ok", botToken, map[string]string{"name": "project-x"}, ""},
		{"duplicate", botToken, map[string]string{"name": "project-x"}, "name_taken"},
		{"duplicate of seeded general", botToken, map[string]string{"name": "general"}, "name_taken"},
		{"bad name", botToken, map[string]string{"name": "Has Spaces"}, "invalid_name_specials"},
		{"missing name", botToken, nil, "invalid_name_required"},
		{"no token", "", map[string]string{"name": "zzz"}, "not_authed"},
	}
	for _, tt := range tests { // sequential: duplicate depends on ok
		status, body := call(t, app, "conversations.create", tt.token, tt.params)
		if status != 200 {
			t.Errorf("%s: status %d, want 200", tt.name, status)
		}
		if tt.wantError != "" {
			if body["ok"] != false || body["error"] != tt.wantError {
				t.Errorf("%s: body %v, want error %s", tt.name, body, tt.wantError)
			}
			continue
		}
		ch, _ := body["channel"].(map[string]any)
		id, _ := ch["id"].(string)
		if body["ok"] != true || !strings.HasPrefix(id, "C") || ch["name"] != "project-x" || ch["is_channel"] != true || ch["is_member"] != true {
			t.Errorf("%s: body %v, want ok channel C... project-x", tt.name, body)
		}
	}
}

func TestConversationsListPagination(t *testing.T) {
	t.Parallel()

	app := storetest.NewApp(t)
	for i := range 4 {
		call(t, app, "conversations.create", botToken, map[string]string{"name": "pg-" + strconv.Itoa(i)})
	}
	_, full := call(t, app, "conversations.list", botToken, map[string]string{"types": "public_channel", "limit": "1000"})
	var want []string
	for _, c := range items(t, full, "channels") {
		want = append(want, c["id"].(string))
	}

	var got []string
	cursor := ""
	for range 100 {
		_, page := call(t, app, "conversations.list", botToken, map[string]string{"limit": "3", "cursor": cursor})
		for _, c := range items(t, page, "channels") {
			got = append(got, c["id"].(string))
		}
		if cursor = nextCursor(page); cursor == "" {
			break
		}
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("paged != full (-want +got):\n%s", diff)
	}

	_, bad := call(t, app, "conversations.list", botToken, map[string]string{"cursor": "bogus"})
	if bad["error"] != "invalid_cursor" {
		t.Errorf("bad cursor body = %v", bad)
	}
	_, badTypes := call(t, app, "conversations.list", botToken, map[string]string{"types": "nope"})
	if badTypes["error"] != "invalid_types" {
		t.Errorf("bad types body = %v", badTypes)
	}
}

func TestConversationsOpenAndListIMs(t *testing.T) {
	t.Parallel()

	app := storetest.NewApp(t)
	human, bot := humanAndBot(t, app)

	_, body := call(t, app, "conversations.open", botToken, map[string]string{"users": human.ID})
	ch, _ := body["channel"].(map[string]any)
	id, _ := ch["id"].(string)
	if body["ok"] != true || !strings.HasPrefix(id, "D") || ch["is_im"] != true || ch["user"] != human.ID {
		t.Fatalf("open body = %v, want D... im with user %s", body, human.ID)
	}
	_, again := call(t, app, "conversations.open", botToken, map[string]string{"users": human.ID})
	if again["channel"].(map[string]any)["id"] != id {
		t.Errorf("reopen gave a different id: %v", again)
	}

	_, ims := call(t, app, "conversations.list", botToken, map[string]string{"types": "im"})
	if got := items(t, ims, "channels"); len(got) != 1 || got[0]["id"] != id {
		t.Errorf("bot im list = %v, want just %s", got, id)
	}
	_, theirs := call(t, app, "conversations.list", "xoxp-"+human.ID, map[string]string{"types": "im"})
	if got := items(t, theirs, "channels"); len(got) != 1 || got[0]["user"] != bot.ID {
		t.Errorf("human im list = %v, want the bot as peer", got)
	}
	_, other := call(t, app, "conversations.list", "xoxp-"+humanOther(t, app, human.ID), map[string]string{"types": "im"})
	if got := items(t, other, "channels"); len(got) != 0 {
		t.Errorf("unrelated user sees %d ims, want 0", len(got))
	}

	for _, tt := range []struct {
		params map[string]string
		want   string
	}{
		{map[string]string{"users": "UNOPE"}, "user_not_found"},
		{nil, "users_list_not_supplied"},
	} {
		if _, b := call(t, app, "conversations.open", botToken, tt.params); b["error"] != tt.want {
			t.Errorf("open %v = %v, want %s", tt.params, b, tt.want)
		}
	}
}

// humanOther returns a user that is neither the bot nor excluded; skips the test if the seed lacks one.
func humanOther(t *testing.T, app *storetest.App, exclude string) string {
	t.Helper()
	us, err := app.Service.Users(t.Context())
	if err != nil {
		t.Fatalf("Users: %v", err)
	}
	for _, u := range us {
		if !u.IsBot && u.ID != exclude {
			return u.ID
		}
	}
	t.Fatalf("pinned seed has no second human; change storetest.PinnedSeed")
	return ""
}

func TestHistoryAndReplies(t *testing.T) {
	t.Parallel()

	app := storetest.NewApp(t)
	general := generalID(t, app)

	_, hist := call(t, app, "conversations.history", botToken, map[string]string{"channel": general, "limit": "1000"})
	msgs := items(t, hist, "messages")
	if hist["ok"] != true || len(msgs) == 0 {
		t.Fatalf("history = %v", hist)
	}
	var root map[string]any
	for i, m := range msgs {
		if i > 0 && m["ts"].(string) >= msgs[i-1]["ts"].(string) {
			t.Errorf("history not newest-first at %d", i)
		}
		if m["type"] != "message" || m["user"] == nil || m["text"] == nil {
			t.Errorf("message missing fields: %v", m)
		}
		if rc, _ := m["reply_count"].(float64); rc > 0 {
			root = m
		}
		if _, isReply := m["parent_user_id"]; isReply {
			t.Errorf("history must not contain replies: %v", m)
		}
	}
	if root == nil || root["thread_ts"] != root["ts"] {
		t.Fatalf("seeded thread root = %v, want thread_ts == ts", root)
	}

	_, th := call(t, app, "conversations.replies", botToken, map[string]string{"channel": general, "ts": root["ts"].(string)})
	replies := items(t, th, "messages")
	if len(replies) != int(root["reply_count"].(float64))+1 || replies[0]["ts"] != root["ts"] {
		t.Fatalf("replies = %v, want root first then %v replies", replies, root["reply_count"])
	}
	for _, r := range replies[1:] {
		if r["thread_ts"] != root["ts"] || r["parent_user_id"] != root["user"] {
			t.Errorf("reply = %v, want thread_ts %v parent_user_id %v", r, root["ts"], root["user"])
		}
	}

	for _, tt := range []struct {
		method string
		params map[string]string
		want   string
	}{
		{"conversations.history", map[string]string{"channel": "CNOPE"}, "channel_not_found"},
		{"conversations.history", nil, "channel_not_found"},
		{"conversations.replies", map[string]string{"channel": general, "ts": "1.000001"}, "thread_not_found"},
		{"conversations.replies", map[string]string{"channel": "CNOPE", "ts": "1.000001"}, "channel_not_found"},
	} {
		if _, b := call(t, app, tt.method, botToken, tt.params); b["error"] != tt.want {
			t.Errorf("%s %v = %v, want %s", tt.method, tt.params, b, tt.want)
		}
	}
}

func TestHistoryPaginationWalk(t *testing.T) {
	t.Parallel()

	app := storetest.NewApp(t)
	general := generalID(t, app)
	_, full := call(t, app, "conversations.history", botToken, map[string]string{"channel": general, "limit": "1000"})
	var want []string
	for _, m := range items(t, full, "messages") {
		want = append(want, m["ts"].(string))
	}

	var got []string
	cursor := ""
	for range 100 {
		_, page := call(t, app, "conversations.history", botToken, map[string]string{"channel": general, "limit": "2", "cursor": cursor})
		for _, m := range items(t, page, "messages") {
			got = append(got, m["ts"].(string))
		}
		if more, _ := page["has_more"].(bool); more != (nextCursor(page) != "") {
			t.Errorf("has_more=%v but next_cursor=%q", more, nextCursor(page))
		}
		if cursor = nextCursor(page); cursor == "" {
			break
		}
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("walk != full (-want +got):\n%s", diff)
	}
}
