package service_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"

	"localslack/internal/service"
)

func TestUpdateMessage(t *testing.T) {
	t.Parallel()

	svc, _ := newSvc(t)
	human, bot := usersOf(t, svc)
	ch := generalID(t, svc, human)
	post := func(text string) service.Message {
		t.Helper()
		m, err := svc.PostMessage(t.Context(), human, service.PostOpts{Channel: ch, Content: service.Content{Text: text}})
		if err != nil {
			t.Fatalf("PostMessage: %v", err)
		}
		return m
	}
	blocks := json.RawMessage(`[{"type":"divider"}]`)
	tests := []struct {
		name    string
		actor   service.User
		channel string
		ts      func(service.Message) string
		content service.Content
		want    func(service.Message) service.Message
		wantErr error
	}{
		{
			name: "text", actor: human, channel: ch, ts: func(m service.Message) string { return m.TS },
			content: service.Content{Text: "new"},
			want:    func(m service.Message) service.Message { m.Text = "new"; return m },
		},
		{
			name: "blocks only keeps text", actor: human, channel: ch, ts: func(m service.Message) string { return m.TS },
			content: service.Content{Blocks: blocks},
			want:    func(m service.Message) service.Message { m.Blocks = blocks; return m },
		},
		{name: "not author", actor: bot, channel: ch, ts: func(m service.Message) string { return m.TS }, content: service.Content{Text: "x"}, wantErr: service.ErrCantUpdateMessage},
		{name: "no message", actor: human, channel: ch, ts: func(service.Message) string { return "1.000001" }, content: service.Content{Text: "x"}, wantErr: service.ErrMessageNotFound},
		{name: "no channel", actor: human, channel: "CNOPE", ts: func(m service.Message) string { return m.TS }, content: service.Content{Text: "x"}, wantErr: service.ErrChannelNotFound},
		{name: "empty", actor: human, channel: ch, ts: func(m service.Message) string { return m.TS }, wantErr: service.ErrNoText},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			orig := post("orig " + tt.name)
			var c collector
			unsub := svc.Subscribe(c.handle)
			defer unsub()

			got, err := svc.UpdateMessage(t.Context(), tt.actor, service.UpdateOpts{Channel: tt.channel, TS: tt.ts(orig), Content: tt.content})

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			changed := changedEvents(c.got(), orig.TS)
			if tt.wantErr != nil {
				if len(changed) != 0 {
					t.Errorf("failed update emitted %+v", changed)
				}
				return
			}
			if got.Edited == nil || got.Edited.User != human.ID || got.Edited.TS <= orig.TS {
				t.Errorf("Edited = %+v, want user %s and ts after %s", got.Edited, human.ID, orig.TS)
			}
			got.Edited = nil
			if diff := cmp.Diff(tt.want(orig), got); diff != "" {
				t.Errorf("updated message (-want +got):\n%s", diff)
			}
			stored, err := svc.History(t.Context(), human, ch, service.PageOpts{Limit: 1000, Oldest: orig.TS, Inclusive: true, Latest: orig.TS})
			if err != nil || len(stored.Items) != 1 || stored.Items[0].Text != got.Text || stored.Items[0].Edited == nil {
				t.Errorf("history = %+v, %v; want the edited message", stored.Items, err)
			}
			if len(changed) != 1 || changed[0].Prev.Text != orig.Text || changed[0].Msg.TS != orig.TS || changed[0].Msg.Edited == nil {
				t.Errorf("MessageChanged events = %+v, want one with Prev %q", changed, orig.Text)
			}
		})
	}
}

func changedEvents(evs []service.Event, ts string) []service.MessageChanged {
	var out []service.MessageChanged
	for _, e := range evs {
		if ev, ok := e.(service.MessageChanged); ok && ev.Msg.TS == ts {
			out = append(out, ev)
		}
	}
	return out
}
