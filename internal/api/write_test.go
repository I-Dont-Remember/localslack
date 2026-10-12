package api_test

import (
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/google/go-cmp/cmp"

	"localslack/internal/constants"
	"localslack/internal/service"
	"localslack/internal/storetest"
)

func TestChatUpdateJSONBodyKeepsBlocksArray(t *testing.T) {
	t.Parallel()

	app := storetest.NewApp(t)
	general := generalID(t, app)
	_, posted := call(t, app, "chat.postMessage", botToken, map[string]string{"channel": general, "text": "v1"})
	body := `{"channel":"` + general + `","ts":"` + posted["ts"].(string) + `","text":"v2","blocks":[{"type":"divider"},{"type":"divider"}]}`

	_, got := do(t, http.MethodPost, app.URL+"/api/chat.update", botToken, constants.ContentTypeJSON, strings.NewReader(body))

	msg, _ := got["message"].(map[string]any)
	blocks, _ := msg["blocks"].([]any)
	if got["ok"] != true || len(blocks) != 2 {
		t.Errorf("chat.update JSON body = %v, want ok with 2 blocks", got)
	}
}

func TestChatPostEphemeral(t *testing.T) {
	t.Parallel()

	app := storetest.NewApp(t)
	general := generalID(t, app)
	human, _ := humanAndBot(t, app)
	var mu sync.Mutex
	var events []service.Event
	app.Service.Subscribe(func(e service.Event) { mu.Lock(); events = append(events, e); mu.Unlock() })

	tests := []struct {
		name   string
		params map[string]string
		want   string // error code, "" for success
	}{
		{"ok", map[string]string{"channel": general, "user": human.ID, "text": "psst"}, ""},
		{"blocks only", map[string]string{"channel": general, "user": human.ID, "blocks": `[{"type":"divider"}]`}, ""},
		{"user not in channel", map[string]string{"channel": general, "user": "UNOPE", "text": "x"}, "user_not_in_channel"},
		{"no channel", map[string]string{"channel": "CNOPE", "user": human.ID, "text": "x"}, "channel_not_found"},
		{"no content", map[string]string{"channel": general, "user": human.ID}, "no_text"},
	}
	for _, tt := range tests {
		_, b := call(t, app, "chat.postEphemeral", botToken, tt.params)
		if tt.want != "" {
			if b["ok"] != false || b["error"] != tt.want {
				t.Errorf("%s: body %v, want error %s", tt.name, b, tt.want)
			}
		} else if ts, _ := b["message_ts"].(string); b["ok"] != true || ts == "" {
			t.Errorf("%s: body %v, want ok with message_ts", tt.name, b)
		}
	}

	_, hist := call(t, app, "conversations.history", botToken, map[string]string{"channel": general, "limit": "100"})
	for _, m := range items(t, hist, "messages") {
		if m["text"] == "psst" {
			t.Errorf("ephemeral appeared in history: %v", m)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if len(events) != 0 || len(app.Service.Ephemerals()) != 2 {
		t.Errorf("events %v, ring %d; want no events and 2 remembered", events, len(app.Service.Ephemerals()))
	}
}

func TestChatDelete(t *testing.T) {
	t.Parallel()

	app := storetest.NewApp(t)
	general := generalID(t, app)
	human, _ := humanAndBot(t, app)
	post := func(params map[string]string) string {
		_, b := call(t, app, "chat.postMessage", botToken, params)
		return b["ts"].(string)
	}
	root := post(map[string]string{"channel": general, "text": "root"})
	reply := post(map[string]string{"channel": general, "text": "reply", "thread_ts": root})
	solo := post(map[string]string{"channel": general, "text": "solo"})

	tests := []struct {
		name   string
		token  string
		params map[string]string
		want   string // error code, "" for success
	}{
		{"not author", "xoxp-" + human.ID, map[string]string{"channel": general, "ts": solo}, "cant_delete_message"},
		{"no message", botToken, map[string]string{"channel": general, "ts": "1.000001"}, "message_not_found"},
		{"no channel", botToken, map[string]string{"channel": "CNOPE", "ts": solo}, "channel_not_found"},
		{"solo", botToken, map[string]string{"channel": general, "ts": solo}, ""},
		{"solo again", botToken, map[string]string{"channel": general, "ts": solo}, "message_not_found"},
		{"root with replies", botToken, map[string]string{"channel": general, "ts": root}, ""},
	}
	for _, tt := range tests {
		_, b := call(t, app, "chat.delete", tt.token, tt.params)
		if tt.want != "" {
			if b["ok"] != false || b["error"] != tt.want {
				t.Errorf("%s: body %v, want error %s", tt.name, b, tt.want)
			}
		} else if b["ok"] != true || b["channel"] != general || b["ts"] != tt.params["ts"] {
			t.Errorf("%s: body %v, want ok with channel and ts", tt.name, b)
		}
	}

	_, hist := call(t, app, "conversations.history", botToken, map[string]string{"channel": general, "limit": "1"})
	got := items(t, hist, "messages")[0]
	if got["ts"] != root || got["subtype"] != "tombstone" || got["text"] != "This message was deleted." || got["reply_count"] != float64(1) {
		t.Errorf("history head = %v, want tombstone root with 1 reply", got)
	}
	_, replies := call(t, app, "conversations.replies", botToken, map[string]string{"channel": general, "ts": root})
	if msgs := items(t, replies, "messages"); len(msgs) != 2 || msgs[1]["ts"] != reply {
		t.Errorf("replies = %v, want tombstone root then the reply", msgs)
	}
}

func TestChatUpdate(t *testing.T) {
	t.Parallel()

	app := storetest.NewApp(t)
	general := generalID(t, app)
	human, _ := humanAndBot(t, app)
	humanTok := "xoxp-" + human.ID
	_, posted := call(t, app, "chat.postMessage", botToken, map[string]string{"channel": general, "text": "v1"})
	ts, _ := posted["ts"].(string)

	tests := []struct {
		name   string
		token  string
		params map[string]string
		want   string // error code, "" for success
	}{
		{"ok", botToken, map[string]string{"channel": general, "ts": ts, "text": "v2"}, ""},
		{"blocks only", botToken, map[string]string{"channel": general, "ts": ts, "blocks": `[{"type":"divider"}]`}, ""},
		{"not author", humanTok, map[string]string{"channel": general, "ts": ts, "text": "x"}, "cant_update_message"},
		{"no message", botToken, map[string]string{"channel": general, "ts": "1.000001", "text": "x"}, "message_not_found"},
		{"no channel", botToken, map[string]string{"channel": "CNOPE", "ts": ts, "text": "x"}, "channel_not_found"},
		{"no content", botToken, map[string]string{"channel": general, "ts": ts}, "no_text"},
		{"bad blocks", botToken, map[string]string{"channel": general, "ts": ts, "blocks": `{nope`}, "invalid_blocks"},
		{"no token", "", map[string]string{"channel": general, "ts": ts, "text": "x"}, "not_authed"},
	}
	for _, tt := range tests {
		_, b := call(t, app, "chat.update", tt.token, tt.params)
		if tt.want != "" {
			if b["ok"] != false || b["error"] != tt.want {
				t.Errorf("%s: body %v, want error %s", tt.name, b, tt.want)
			}
			continue
		}
		if b["ok"] != true || b["channel"] != general || b["ts"] != ts {
			t.Errorf("%s: body %v, want ok with channel and ts", tt.name, b)
		}
	}

	_, hist := call(t, app, "conversations.history", botToken, map[string]string{"channel": general, "limit": "1"})
	got := items(t, hist, "messages")[0]
	edited, _ := got["edited"].(map[string]any)
	if got["text"] != "v2" || edited["user"] != posted["message"].(map[string]any)["user"] || edited["ts"] == "" {
		t.Errorf("history after update = %v, want text v2 and edited{user,ts}", got)
	}
	if diff := cmp.Diff([]any{map[string]any{"type": "divider"}}, got["blocks"]); diff != "" {
		t.Errorf("blocks (-want +got):\n%s", diff)
	}
}
