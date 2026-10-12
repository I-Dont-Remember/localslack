package api_test

import (
	"sync"
	"testing"

	"github.com/google/go-cmp/cmp"

	"localslack/internal/service"
	"localslack/internal/storetest"
)

func TestReactionsAdd(t *testing.T) {
	t.Parallel()

	app := storetest.NewApp(t)
	general := generalID(t, app)
	human, bot := humanAndBot(t, app)
	_, posted := call(t, app, "chat.postMessage", "xoxp-"+human.ID, map[string]string{"channel": general, "text": "react to me"})
	ts, _ := posted["ts"].(string)
	var mu sync.Mutex
	var events []service.ReactionAdded
	app.Service.Subscribe(func(e service.Event) {
		if ev, ok := e.(service.ReactionAdded); ok {
			mu.Lock()
			events = append(events, ev)
			mu.Unlock()
		}
	})

	tests := []struct {
		name   string
		token  string
		params map[string]string
		want   string // error code, "" for success
	}{
		{"ok", botToken, map[string]string{"channel": general, "timestamp": ts, "name": "eyes"}, ""},
		{"second user", "xoxp-" + human.ID, map[string]string{"channel": general, "timestamp": ts, "name": "eyes"}, ""},
		{"already reacted", botToken, map[string]string{"channel": general, "timestamp": ts, "name": "eyes"}, "already_reacted"},
		{"invalid name", botToken, map[string]string{"channel": general, "timestamp": ts, "name": ":eyes:"}, "invalid_name"},
		{"no name", botToken, map[string]string{"channel": general, "timestamp": ts}, "invalid_name"},
		{"no message", botToken, map[string]string{"channel": general, "timestamp": "1.000001", "name": "eyes"}, "message_not_found"},
		{"no channel", botToken, map[string]string{"channel": "CNOPE", "timestamp": ts, "name": "eyes"}, "channel_not_found"},
		{"no token", "", map[string]string{"channel": general, "timestamp": ts, "name": "eyes"}, "not_authed"},
	}
	for _, tt := range tests {
		_, b := call(t, app, "reactions.add", tt.token, tt.params)
		if tt.want != "" {
			if b["ok"] != false || b["error"] != tt.want {
				t.Errorf("%s: body %v, want error %s", tt.name, b, tt.want)
			}
		} else if b["ok"] != true {
			t.Errorf("%s: body %v, want ok", tt.name, b)
		}
	}

	_, hist := call(t, app, "conversations.history", botToken, map[string]string{"channel": general, "limit": "1"})
	got := items(t, hist, "messages")[0]["reactions"]
	want := []any{map[string]any{"name": "eyes", "users": []any{bot.ID, human.ID}, "count": float64(2)}}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("history reactions (-want +got):\n%s", diff)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(events) != 2 || events[0].User != bot.ID || events[0].Msg.UserID != human.ID || events[0].Reaction != "eyes" {
		t.Errorf("ReactionAdded events = %+v, want 2; first by bot on the human's message", events)
	}
}
