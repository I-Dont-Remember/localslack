package store

import (
	"encoding/json"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestMessageDataRoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		msg  Message
	}{
		{"plain", Message{ChannelID: "C1", TS: "1.000001", UserID: "U1", Text: "hi"}},
		{"bot", Message{ChannelID: "C1", TS: "1.000002", UserID: "U1", Text: "hi", BotID: "B1", Subtype: "bot_message"}},
		{"rich", Message{
			ChannelID: "C1", TS: "1.000003", UserID: "U1", Text: "hi",
			Blocks:      json.RawMessage(`[{"type":"section"}]`),
			Attachments: json.RawMessage(`[{"text":"a"}]`),
			Metadata:    json.RawMessage(`{"event_type":"x","event_payload":{"k":1}}`),
			Edited:      &Edit{User: "U1", TS: "2.000001"},
		}},
	}
	s := openTemp(t)
	fixtureTx(t, s)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := s.Write(t.Context(), func(tx *Tx) error { return tx.InsertMessage(t.Context(), tt.msg) })
			if err != nil {
				t.Fatalf("InsertMessage: %v", err)
			}
			got, err := s.Message(t.Context(), tt.msg.ChannelID, tt.msg.TS)
			if err != nil {
				t.Fatalf("Message: %v", err)
			}
			if diff := cmp.Diff(tt.msg, got); diff != "" {
				t.Errorf("round trip (-want +got):\n%s", diff)
			}
		})
	}
}

func TestMessageDataIsJSONColumn(t *testing.T) {
	t.Parallel()

	s := openTemp(t)
	fixtureTx(t, s)
	m := Message{ChannelID: "C1", TS: "9.000001", UserID: "U1", BotID: "B1"}
	if err := s.Write(t.Context(), func(tx *Tx) error { return tx.InsertMessage(t.Context(), m) }); err != nil {
		t.Fatalf("InsertMessage: %v", err)
	}
	var got string
	if err := s.reader.QueryRowContext(t.Context(), `SELECT data FROM messages WHERE ts = '9.000001'`).Scan(&got); err != nil {
		t.Fatalf("select data: %v", err)
	}
	if want := `{"bot_id":"B1"}`; got != want {
		t.Errorf("data = %s, want %s", got, want)
	}
}
