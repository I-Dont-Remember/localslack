package api_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"localslack/internal/constants"
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
