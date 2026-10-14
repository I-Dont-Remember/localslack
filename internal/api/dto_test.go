package api

import (
	"encoding/json"
	"testing"

	"github.com/google/go-cmp/cmp"

	"localslack/internal/service"
)

func TestMessageDTO(t *testing.T) {
	t.Parallel()

	base := map[string]any{"type": "message", "user": "U1", "text": "hi", "ts": "1.000001"}
	with := func(kv map[string]any) map[string]any {
		out := map[string]any{}
		for k, v := range base {
			out[k] = v
		}
		for k, v := range kv {
			out[k] = v
		}
		return out
	}
	tests := []struct {
		name string
		msg  service.Message
		want map[string]any
	}{
		{"plain", service.Message{UserID: "U1", Text: "hi", TS: "1.000001"}, base},
		{"bot", service.Message{UserID: "U1", Text: "hi", TS: "1.000001", BotID: "B1"}, with(map[string]any{"bot_id": "B1"})},
		{"subtype", service.Message{UserID: "U1", Text: "hi", TS: "1.000001", Subtype: "me_message"}, with(map[string]any{"subtype": "me_message"})},
		{"content", service.Message{
			UserID: "U1", Text: "hi", TS: "1.000001",
			Blocks:      json.RawMessage(`[{"type":"divider"}]`),
			Attachments: json.RawMessage(`[{"text":"a"}]`),
			Metadata:    json.RawMessage(`{"event_type":"e"}`),
		}, with(map[string]any{
			"blocks":      []any{map[string]any{"type": "divider"}},
			"attachments": []any{map[string]any{"text": "a"}},
			"metadata":    map[string]any{"event_type": "e"},
		})},
		{
			"edited",
			service.Message{UserID: "U1", Text: "hi", TS: "1.000001", Edited: &service.Edit{User: "U2", TS: "2.000001"}},
			with(map[string]any{"edited": map[string]any{"user": "U2", "ts": "2.000001"}}),
		},
		{
			"reactions",
			service.Message{UserID: "U1", Text: "hi", TS: "1.000001", Reactions: []service.Reaction{{Name: "eyes", Users: []string{"U2"}, Count: 1}}},
			with(map[string]any{"reactions": []any{map[string]any{"name": "eyes", "users": []any{"U2"}, "count": float64(1)}}}),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b, err := json.Marshal(messageDTO(tt.msg))
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			var got map[string]any
			if err := json.Unmarshal(b, &got); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("messageDTO (-want +got):\n%s", diff)
			}
		})
	}
}
