package api_test

import (
	"strings"
	"testing"

	"localslack/internal/storetest"
)

func TestChatPostMessage(t *testing.T) {
	t.Parallel()

	app := storetest.NewApp(t)
	general := generalID(t, app)

	_, body := call(t, app, "chat.postMessage", botToken, map[string]string{"channel": general, "text": "hello there"})
	msg, _ := body["message"].(map[string]any)
	ts, _ := body["ts"].(string)
	if body["ok"] != true || body["channel"] != general || ts == "" || msg["text"] != "hello there" || msg["ts"] != ts || msg["type"] != "message" {
		t.Fatalf("post body = %v", body)
	}

	_, hist := call(t, app, "conversations.history", botToken, map[string]string{"channel": general, "limit": "1"})
	if got := items(t, hist, "messages"); len(got) != 1 || got[0]["ts"] != ts || got[0]["text"] != "hello there" {
		t.Errorf("history after post = %v, want the new message first", got)
	}

	_, reply := call(t, app, "chat.postMessage", botToken, map[string]string{"channel": general, "text": "in thread", "thread_ts": ts})
	if m, _ := reply["message"].(map[string]any); reply["ok"] != true || m["thread_ts"] != ts {
		t.Errorf("reply body = %v, want message.thread_ts %s", reply, ts)
	}
	_, hist = call(t, app, "conversations.history", botToken, map[string]string{"channel": general, "limit": "1"})
	if got := items(t, hist, "messages"); got[0]["reply_count"] != float64(1) || got[0]["thread_ts"] != ts {
		t.Errorf("parent after reply = %v, want reply_count 1 thread_ts %s", got[0], ts)
	}

	for _, tt := range []struct {
		name   string
		token  string
		params map[string]string
		want   string
	}{
		{"missing channel", botToken, map[string]string{"channel": "CNOPE", "text": "x"}, "channel_not_found"},
		{"no channel param", botToken, map[string]string{"text": "x"}, "channel_not_found"},
		{"no text", botToken, map[string]string{"channel": general}, "no_text"},
		{"bad thread", botToken, map[string]string{"channel": general, "text": "x", "thread_ts": "1.000001"}, "thread_not_found"},
		{"no token", "", map[string]string{"channel": general, "text": "x"}, "not_authed"},
	} {
		if _, b := call(t, app, "chat.postMessage", tt.token, tt.params); b["error"] != tt.want {
			t.Errorf("%s: body %v, want error %s", tt.name, b, tt.want)
		}
	}
}

func TestPostAsUserToken(t *testing.T) {
	t.Parallel()

	app := storetest.NewApp(t)
	human, _ := humanAndBot(t, app)
	general := generalID(t, app)
	_, body := call(t, app, "chat.postMessage", "xoxp-"+human.ID, map[string]string{"channel": general, "text": "as human"})
	if msg, _ := body["message"].(map[string]any); msg["user"] != human.ID {
		t.Errorf("message user = %v, want %s", msg["user"], human.ID)
	}
}

func TestConversationsArchive(t *testing.T) {
	t.Parallel()

	app := storetest.NewApp(t)
	_, created := call(t, app, "conversations.create", botToken, map[string]string{"name": "to-archive"})
	id := created["channel"].(map[string]any)["id"].(string)

	if _, b := call(t, app, "conversations.archive", botToken, map[string]string{"channel": id}); b["ok"] != true {
		t.Fatalf("archive body = %v", b)
	}
	if _, b := call(t, app, "chat.postMessage", botToken, map[string]string{"channel": id, "text": "x"}); b["error"] != "is_archived" {
		t.Errorf("post to archived = %v, want is_archived", b)
	}
	_, all := call(t, app, "conversations.list", botToken, map[string]string{"limit": "1000"})
	_, live := call(t, app, "conversations.list", botToken, map[string]string{"limit": "1000", "exclude_archived": "true"})
	has := func(body map[string]any) bool {
		for _, c := range items(t, body, "channels") {
			if strings.EqualFold(c["id"].(string), id) {
				return true
			}
		}
		return false
	}
	if !has(all) || has(live) {
		t.Errorf("archived channel in full list=%v, in exclude_archived list=%v; want true, false", has(all), has(live))
	}
	if _, b := call(t, app, "conversations.archive", botToken, map[string]string{"channel": generalID(t, app)}); b["error"] != "cant_archive_general" {
		t.Errorf("archive general = %v, want cant_archive_general", b)
	}
}
