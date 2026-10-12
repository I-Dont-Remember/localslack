package api_test

import (
	"encoding/json"
	"maps"
	"net/http"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"localslack/internal/constants"
	"localslack/internal/storetest"
)

func contentChannel(t *testing.T, app *storetest.App, name string) string {
	t.Helper()
	_, b := call(t, app, "conversations.create", botToken, map[string]string{"name": name})
	ch, _ := b["channel"].(map[string]any)
	id, _ := ch["id"].(string)
	if id == "" {
		t.Fatalf("create %s: %v", name, b)
	}
	return id
}

func enc(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func TestPostContent(t *testing.T) {
	t.Parallel()

	app := storetest.NewApp(t)
	general := generalID(t, app)
	blocks := []any{map[string]any{"type": "section", "text": map[string]any{"type": "mrkdwn", "text": "hi *there*"}}}
	attachments := []any{map[string]any{"text": "att", "color": "good"}}
	meta := map[string]any{"event_type": "task_created", "event_payload": map[string]any{"id": "T1"}}

	t.Run("form strings, blocks only", func(t *testing.T) {
		t.Parallel()
		_, b := call(t, app, "chat.postMessage", botToken, map[string]string{"channel": general, "blocks": enc(blocks)})
		if b["ok"] != true {
			t.Fatalf("body = %v", b)
		}
		msg, _ := b["message"].(map[string]any)
		if diff := cmp.Diff(blocks, msg["blocks"]); diff != "" {
			t.Errorf("message.blocks (-want +got):\n%s", diff)
		}
	})

	t.Run("json values round trip", func(t *testing.T) {
		t.Parallel()
		ch := contentChannel(t, app, "json-content")
		body := enc(map[string]any{"channel": ch, "text": "t", "blocks": blocks, "attachments": attachments, "metadata": meta})
		status, b := do(t, http.MethodPost, app.URL+"/api/chat.postMessage", botToken, constants.ContentTypeJSON, strings.NewReader(body))
		if status != http.StatusOK || b["ok"] != true {
			t.Fatalf("status %d body = %v", status, b)
		}
		_, hist := call(t, app, "conversations.history", botToken, map[string]string{"channel": ch})
		got := items(t, hist, "messages")[0]
		want := map[string]any{"blocks": blocks, "attachments": attachments, "metadata": nil}
		if diff := cmp.Diff(want, map[string]any{"blocks": got["blocks"], "attachments": got["attachments"], "metadata": got["metadata"]}); diff != "" {
			t.Errorf("history content (-want +got):\n%s", diff)
		}
		_, hist = call(t, app, "conversations.history", botToken, map[string]string{"channel": ch, "include_all_metadata": "true"})
		if diff := cmp.Diff(meta, items(t, hist, "messages")[0]["metadata"]); diff != "" {
			t.Errorf("history metadata (-want +got):\n%s", diff)
		}
	})

	t.Run("replies", func(t *testing.T) {
		t.Parallel()
		ch := contentChannel(t, app, "reply-content")
		_, root := call(t, app, "chat.postMessage", botToken, map[string]string{"channel": ch, "text": "root"})
		ts, _ := root["ts"].(string)
		call(t, app, "chat.postMessage", botToken, map[string]string{
			"channel": ch, "thread_ts": ts, "blocks": enc(blocks), "metadata": enc(meta),
		})
		_, rep := call(t, app, "conversations.replies", botToken, map[string]string{"channel": ch, "ts": ts})
		got := items(t, rep, "messages")[1]
		if diff := cmp.Diff(blocks, got["blocks"]); diff != "" || got["metadata"] != nil {
			t.Errorf("reply blocks diff %q metadata %v", diff, got["metadata"])
		}
		_, rep = call(t, app, "conversations.replies", botToken, map[string]string{"channel": ch, "ts": ts, "include_all_metadata": "true"})
		if diff := cmp.Diff(meta, items(t, rep, "messages")[1]["metadata"]); diff != "" {
			t.Errorf("reply metadata (-want +got):\n%s", diff)
		}
	})

	for _, tt := range []struct {
		name   string
		params map[string]string
		want   string
	}{
		{"blocks not json", map[string]string{"blocks": "nope"}, "invalid_blocks"},
		{"block without type", map[string]string{"blocks": `[{"x":1}]`}, "invalid_blocks"},
		{"attachments object", map[string]string{"text": "x", "attachments": `{"a":1}`}, "invalid_attachments"},
		{"metadata missing payload", map[string]string{"text": "x", "metadata": `{"event_type":"t"}`}, "invalid_metadata_format"},
		{"too long", map[string]string{"text": strings.Repeat("a", 40001)}, "msg_too_long"},
		{"empty blocks only", map[string]string{"blocks": "[]"}, "no_text"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			params := map[string]string{"channel": general}
			maps.Copy(params, tt.params)
			if _, b := call(t, app, "chat.postMessage", botToken, params); b["error"] != tt.want {
				t.Errorf("body %v, want error %s", b, tt.want)
			}
		})
	}
}
